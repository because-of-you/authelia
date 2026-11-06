// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/authelia/authelia/v4/internal/authentication"
	"github.com/authelia/authelia/v4/internal/mocks"
	"github.com/authelia/authelia/v4/internal/wechat"
)

type groupHandlerStore struct {
	wechat.Storage
	enabled bool
	groups  []string
	changes int
}

func (s *groupHandlerStore) Session(context.Context, string, time.Time) (wechat.MiniSession, error) {
	return wechat.MiniSession{Username: "alice", AppID: "wx0123456789abcdef", OpenID: "openid"}, nil
}
func (s *groupHandlerStore) LookupMiniUser(context.Context, string, string) (*authentication.UserDetails, bool, error) {
	return &authentication.UserDetails{Username: "alice", Groups: s.groups}, s.enabled, nil
}
func (s *groupHandlerStore) ChangeGroup(context.Context, wechat.MiniSession, string, string, string) error {
	s.changes++
	return nil
}

func TestWeChatGroupManagementRejectsNonAdminAndDisabledAdmin(t *testing.T) {
	for _, tc := range []struct {
		name    string
		enabled bool
		groups  []string
		want    int
	}{
		{"ordinary user", true, []string{"users"}, 403},
		{"disabled admin", false, []string{"admin"}, 403},
		{"enabled admin", true, []string{"admin"}, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := mocks.NewMockAutheliaCtx(t)
			defer mock.Close()
			store := &groupHandlerStore{enabled: tc.enabled, groups: tc.groups}
			h := &WeChatHandlers{client: wechat.NewClient("wx0123456789abcdef", "secret", "develop"), store: store}
			mock.Ctx.Request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 64))
			mock.Ctx.Request.Header.SetContentType("application/json")
			mock.Ctx.Request.SetBodyString(`{"action":"create","group":"engineering"}`)
			h.ChangeGroups(mock.Ctx)
			require.Equal(t, tc.want, mock.Ctx.Response.StatusCode())
			if tc.want == 200 {
				require.Equal(t, 1, store.changes)
			} else {
				require.Zero(t, store.changes)
			}
		})
	}
}
