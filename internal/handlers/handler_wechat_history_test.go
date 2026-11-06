// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/authelia/authelia/v4/internal/mocks"
	"github.com/authelia/authelia/v4/internal/wechat"
)

type historyHandlerStore struct {
	groupHandlerStore
	actor  wechat.MiniSession
	before int64
}

func (s *historyHandlerStore) AuthenticationHistory(_ context.Context, actor wechat.MiniSession, before int64) (wechat.AuthenticationPage, error) {
	s.actor, s.before = actor, before
	return wechat.AuthenticationPage{Records: []wechat.AuthenticationRecord{}}, nil
}

func TestWeChatAuthenticationHistoryRequiresSessionAndUsesVerifiedIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, token, query string
		want               int
	}{
		{"missing token", "", "", 401},
		{"invalid cursor", strings.Repeat("a", 64), "before=-1", 400},
		{"verified account", strings.Repeat("a", 64), "before=42&username=bob", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := mocks.NewMockAutheliaCtx(t)
			defer mock.Close()
			store := &historyHandlerStore{}
			h := &WeChatHandlers{client: wechat.NewClient("wx0123456789abcdef", "secret", "develop"), store: store}
			if tc.token != "" {
				mock.Ctx.Request.Header.Set("Authorization", "Bearer "+tc.token)
			}
			mock.Ctx.Request.SetRequestURI("/api/wechat/authentication-history?" + tc.query)
			h.AuthenticationHistory(mock.Ctx)
			require.Equal(t, tc.want, mock.Ctx.Response.StatusCode())
			if tc.want == 200 {
				require.Equal(t, "alice", store.actor.Username)
				require.EqualValues(t, 42, store.before)
			}
		})
	}
}
