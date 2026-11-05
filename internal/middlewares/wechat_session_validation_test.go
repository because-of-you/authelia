// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

package middlewares

import (
	"errors"
	"testing"

	"github.com/authelia/authelia/v4/internal/authentication"
	"github.com/authelia/authelia/v4/internal/configuration/schema"
	"github.com/authelia/authelia/v4/internal/session"
)

type wechatSessionResolver struct {
	authentication.UserProvider
	details *authentication.UserDetails
	err     error
}

func (p wechatSessionResolver) ResolveWeChatUser(_, _ string) (*authentication.UserDetails, error) {
	return p.details, p.err
}

func TestWeChatSessionValidationFailsClosed(t *testing.T) {
	s := session.UserSession{Username: "Alice", WeChatAppID: "wx-app", WeChatOpenID: "openid"}
	cfg := &schema.WeChat{AppID: "wx-app"}
	for _, tc := range []struct {
		name     string
		provider authentication.UserProvider
		config   *schema.WeChat
		want     bool
	}{
		{"valid", wechatSessionResolver{details: &authentication.UserDetails{Username: "Alice"}}, cfg, true},
		{"revoked", wechatSessionResolver{err: errors.New("not found")}, cfg, false},
		{"database error", wechatSessionResolver{err: errors.New("database unavailable")}, cfg, false},
		{"different account case", wechatSessionResolver{details: &authentication.UserDetails{Username: "alice"}}, cfg, false},
		{"missing configuration", wechatSessionResolver{details: &authentication.UserDetails{Username: "Alice"}}, nil, false},
		{"different app", wechatSessionResolver{details: &authentication.UserDetails{Username: "Alice"}}, &schema.WeChat{AppID: "other"}, false},
		{"missing resolver", nil, cfg, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := validWeChatSession(tc.provider, tc.config, s); got != tc.want {
				t.Fatalf("validWeChatSession() = %v, want %v", got, tc.want)
			}
		})
	}
}
