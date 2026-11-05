// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/authelia/authelia/v4/internal/authentication"
	"github.com/authelia/authelia/v4/internal/wechat"
)

func TestWeChatConfiguredSQLiteStorage(t *testing.T) {
	p := newTestSQLiteProviderWithEncryption(t)
	t.Cleanup(func() { require.NoError(t, p.Close()) })
	ctx := context.Background()
	require.NoError(t, p.SchemaMigrate(ctx, true, SchemaLatest))

	_, err := p.db.ExecContext(ctx, `INSERT INTO wechat_users (id, username, display_name, enabled) VALUES (1, 'Alice', 'Alice', 1)`)
	require.NoError(t, err)
	_, err = p.db.ExecContext(ctx, `INSERT INTO wechat_identities (user_id, appid, openid) VALUES (1, 'wx-test', 'open-id')`)
	require.NoError(t, err)
	details, err := p.LookupWeChatUser(ctx, "wx-test", "open-id", "")
	require.NoError(t, err)
	require.Equal(t, "Alice", details.Username)
	_, err = p.LookupWeChatUser(ctx, "wx-test", "OPEN-ID", "")
	require.ErrorIs(t, err, authentication.ErrUserNotFound)

	now := time.Now().UTC().Truncate(time.Second)
	s := p.WeChatStore()
	session := wechat.MiniSession{Username: "Alice", AppID: "wx-test", OpenID: "open-id", ExpiresAt: now.Add(time.Minute)}
	require.NoError(t, s.CreateSession(ctx, "session-token", session))
	got, err := s.Session(ctx, "session-token", now)
	require.NoError(t, err)
	require.Equal(t, session, got)
	challenge := wechat.Challenge{TargetDomain: "app.example.com", ExpiresAt: now.Add(time.Minute)}
	require.NoError(t, s.CreateChallenge(ctx, "challenge", "browser", challenge, []byte("image")))
	require.NoError(t, s.Decide(ctx, "challenge", session, "approved", now))
	require.ErrorIs(t, s.Consume(ctx, "challenge", "wrong-browser", now), wechat.ErrNotFound)
	require.NoError(t, s.Consume(ctx, "challenge", "browser", now))
	require.ErrorIs(t, s.Consume(ctx, "challenge", "browser", now), wechat.ErrNotFound)

	require.NoError(t, s.CreateChallenge(ctx, "concurrent", "browser", challenge, []byte("image")))
	require.NoError(t, s.Decide(ctx, "concurrent", session, "approved", now))
	var successes atomic.Int32
	var wait sync.WaitGroup
	unexpected := make(chan error, 8)
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			err := s.Consume(ctx, "concurrent", "browser", now)
			if err == nil {
				successes.Add(1)
			} else if !errors.Is(err, wechat.ErrNotFound) {
				unexpected <- err
			}
		}()
	}
	wait.Wait()
	close(unexpected)
	for err := range unexpected {
		require.NoError(t, err)
	}
	require.EqualValues(t, 1, successes.Load())

	_, err = p.db.ExecContext(ctx, `UPDATE wechat_users SET enabled = 0 WHERE id = 1`)
	require.NoError(t, err)
	_, err = p.LookupWeChatUser(ctx, "wx-test", "open-id", "")
	require.ErrorIs(t, err, authentication.ErrUserNotFound)

	// The feature migration supports the same downgrade and upgrade mechanism as the existing schema.
	require.NoError(t, p.SchemaMigrate(ctx, false, 30))
	var count int
	err = p.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM wechat_users`)
	require.Error(t, err)
	require.NoError(t, p.SchemaMigrate(ctx, true, SchemaLatest))
	require.NoError(t, p.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM wechat_users`))
	require.Zero(t, count)
}

func TestWeChatAutoRegistration(t *testing.T) {
	p := newTestSQLiteProviderWithEncryption(t)
	t.Cleanup(func() { require.NoError(t, p.Close()) })
	ctx := context.Background()
	require.NoError(t, p.SchemaMigrate(ctx, true, SchemaLatest))
	s := p.WeChatStore()
	require.NoError(t, s.RegisterIdentity(ctx, "wx-test", "verified-openid"))
	details, err := p.LookupWeChatUser(ctx, "wx-test", "verified-openid", "")
	require.NoError(t, err)
	require.NotEmpty(t, details.Username)
	require.Empty(t, details.Groups)
	require.ErrorIs(t, s.RegisterIdentity(ctx, "wx-test", "verified-openid"), wechat.ErrNotFound)
	_, err = p.db.ExecContext(ctx, `UPDATE wechat_users SET enabled = 0 WHERE username = ?`, details.Username)
	require.NoError(t, err)
	require.ErrorIs(t, s.RegisterIdentity(ctx, "wx-test", "verified-openid"), wechat.ErrNotFound)
	_, err = p.LookupWeChatUser(ctx, "wx-test", "verified-openid", "")
	require.ErrorIs(t, err, authentication.ErrUserNotFound)
	var count int
	require.NoError(t, p.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM wechat_users`))
	require.Equal(t, 1, count)
}

func TestWeChatProfileUsesAuthenticatedIdentity(t *testing.T) {
	p := newTestSQLiteProviderWithEncryption(t)
	t.Cleanup(func() { require.NoError(t, p.Close()) })
	ctx := context.Background()
	require.NoError(t, p.SchemaMigrate(ctx, true, SchemaLatest))
	s := p.WeChatStore()
	require.NoError(t, s.RegisterIdentity(ctx, "wx-test", "alice-openid"))
	require.NoError(t, s.RegisterIdentity(ctx, "wx-test", "bob-openid"))
	alice, err := p.LookupWeChatUser(ctx, "wx-test", "alice-openid", "")
	require.NoError(t, err)
	bob, err := p.LookupWeChatUser(ctx, "wx-test", "bob-openid", "")
	require.NoError(t, err)
	a := wechat.MiniSession{Username: alice.Username, AppID: "wx-test", OpenID: "alice-openid"}
	b := wechat.MiniSession{Username: bob.Username, AppID: "wx-test", OpenID: "bob-openid"}
	profile := wechat.Profile{Nickname: "Alice", Avatar: []byte("image")}
	require.NoError(t, s.SaveProfile(ctx, a, profile))
	got, err := s.Profile(ctx, a)
	require.NoError(t, err)
	require.Equal(t, profile, got)
	_, err = s.Profile(ctx, b)
	require.ErrorIs(t, err, wechat.ErrNotFound)
	b.Username = a.Username
	require.ErrorIs(t, s.SaveProfile(ctx, b, profile), wechat.ErrNotFound)
	_, err = p.db.ExecContext(ctx, `UPDATE wechat_users SET enabled = 0 WHERE username = ?`, a.Username)
	require.NoError(t, err)
	require.ErrorIs(t, s.SaveProfile(ctx, a, profile), wechat.ErrNotFound)
}
