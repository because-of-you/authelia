// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/authelia/authelia/v4/internal/model"
	"github.com/authelia/authelia/v4/internal/wechat"
)

func TestWeChatAuthenticationHistoryIsScopedAndLimited(t *testing.T) {
	p := newTestSQLiteProviderWithEncryption(t)
	t.Cleanup(func() { require.NoError(t, p.Close()) })
	ctx := context.Background()
	require.NoError(t, p.SchemaMigrate(ctx, true, SchemaLatest))
	s := p.WeChatStore()
	require.NoError(t, s.RegisterIdentity(ctx, "wx-test", "alice"))
	require.NoError(t, s.RegisterIdentity(ctx, "wx-test", "bob"))
	alice, _, err := s.LookupMiniUser(ctx, "wx-test", "alice")
	require.NoError(t, err)
	bob, _, err := s.LookupMiniUser(ctx, "wx-test", "bob")
	require.NoError(t, err)
	now := time.Now().UTC().Truncate(time.Second)
	for i := range 51 {
		require.NoError(t, p.AppendAuthenticationLog(ctx, model.AuthenticationAttempt{Username: alice.Username, Time: now.Add(time.Duration(i) * time.Second), Successful: i%2 == 0, Type: "1FA", RemoteIP: model.NewNullIPFromString("192.0.2.1"), RequestURI: "https://user:secret@app.example.com/api?token=secret#fragment", RequestMethod: "GET"}))
	}
	require.NoError(t, p.AppendAuthenticationLog(ctx, model.AuthenticationAttempt{Username: bob.Username, Time: now.Add(time.Hour), Successful: true, Type: "TOTP"}))
	session := wechat.MiniSession{Username: alice.Username, AppID: "wx-test", OpenID: "alice"}
	page, err := s.AuthenticationHistory(ctx, session, 0)
	require.NoError(t, err)
	require.Len(t, page.Records, 10)
	require.Equal(t, "1FA", page.Records[0].Type)
	require.True(t, page.Records[0].Successful)
	require.Equal(t, "192.0.2.1", page.Records[0].RemoteIP)
	require.Equal(t, "https://app.example.com/api", page.Records[0].RequestURI)
	require.Equal(t, "GET", page.Records[0].RequestMethod)
	require.True(t, page.Records[0].Time.Equal(now.Add(50*time.Second)))
	_, err = p.db.ExecContext(ctx, `UPDATE authentication_logs SET request_uri = NULL, remote_ip = NULL WHERE id = ?`, page.Records[0].ID)
	require.NoError(t, err)
	withNulls, err := s.AuthenticationHistory(ctx, session, 0)
	require.NoError(t, err)
	require.Empty(t, withNulls.Records[0].RequestURI)
	require.Empty(t, withNulls.Records[0].RemoteIP)
	require.Positive(t, page.NextCursor)
	second, err := s.AuthenticationHistory(ctx, session, page.NextCursor)
	require.NoError(t, err)
	require.Len(t, second.Records, 10)
	require.True(t, second.Records[0].Time.Equal(now.Add(40*time.Second)))
	session.OpenID = "bob"
	page, err = s.AuthenticationHistory(ctx, session, 0)
	require.NoError(t, err)
	require.Empty(t, page.Records)
}
