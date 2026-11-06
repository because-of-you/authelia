// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/authelia/authelia/v4/internal/wechat"
)

func TestWeChatGroupManagementAuthorizationAndMembership(t *testing.T) {
	p := newTestSQLiteProviderWithEncryption(t)
	t.Cleanup(func() { require.NoError(t, p.Close()) })
	ctx := context.Background()
	require.NoError(t, p.SchemaMigrate(ctx, true, SchemaLatest))
	s := p.WeChatStore()
	require.NoError(t, s.RegisterIdentity(ctx, "wx-test", "admin-openid"))
	require.NoError(t, s.RegisterIdentity(ctx, "wx-test", "user-openid"))
	admin, _, err := s.LookupMiniUser(ctx, "wx-test", "admin-openid")
	require.NoError(t, err)
	user, _, err := s.LookupMiniUser(ctx, "wx-test", "user-openid")
	require.NoError(t, err)
	adminSession := wechat.MiniSession{Username: admin.Username, AppID: "wx-test", OpenID: "admin-openid"}
	userSession := wechat.MiniSession{Username: user.Username, AppID: "wx-test", OpenID: "user-openid"}
	_, err = p.db.ExecContext(ctx, `INSERT INTO wechat_groups (name) VALUES ('admin')`)
	require.NoError(t, err)
	_, err = p.db.ExecContext(ctx, `INSERT INTO wechat_user_groups (user_id, group_id) SELECT u.id, g.id FROM wechat_users u, wechat_groups g WHERE u.username = ? AND g.name = 'admin'`, admin.Username)
	require.NoError(t, err)

	// A disabled admin and a regular user cannot modify or enumerate the directory.
	require.ErrorIs(t, s.ChangeGroup(ctx, adminSession, "create", "engineering", ""), wechat.ErrForbidden)
	_, err = s.ListGroupManagement(ctx, userSession)
	require.ErrorIs(t, err, wechat.ErrForbidden)
	_, err = p.db.ExecContext(ctx, `UPDATE wechat_users SET enabled = ? WHERE username = ?`, true, admin.Username)
	require.NoError(t, err)
	require.NoError(t, s.ChangeGroup(ctx, adminSession, "create", "engineering", ""))
	require.ErrorIs(t, s.ChangeGroup(ctx, adminSession, "create", "engineering", ""), wechat.ErrConflict)
	require.NoError(t, s.ChangeGroup(ctx, adminSession, "add", "engineering", user.Username))
	data, err := s.ListGroupManagement(ctx, adminSession)
	require.NoError(t, err)
	require.Contains(t, data.Groups, "engineering")
	_, enabled, err := s.LookupMiniUser(ctx, "wx-test", "user-openid")
	require.NoError(t, err)
	require.False(t, enabled)
	_, err = p.db.ExecContext(ctx, `UPDATE wechat_users SET enabled = ? WHERE username = ?`, true, user.Username)
	require.NoError(t, err)
	webUser, err := p.LookupWeChatUser(ctx, "wx-test", "user-openid", "")
	require.NoError(t, err)
	require.Contains(t, webUser.Groups, "engineering")
	require.NoError(t, s.ChangeGroup(ctx, adminSession, "remove", "engineering", user.Username))
	require.NoError(t, s.ChangeGroup(ctx, adminSession, "delete", "engineering", ""))
	require.ErrorIs(t, s.ChangeGroup(ctx, adminSession, "delete", "admin", ""), wechat.ErrForbidden)
	// Revoking the admin membership takes effect on the very next request.
	require.NoError(t, s.ChangeGroup(ctx, adminSession, "remove", "admin", admin.Username))
	require.ErrorIs(t, s.ChangeGroup(ctx, adminSession, "create", "finance", ""), wechat.ErrForbidden)
}
