// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	"go.uber.org/mock/gomock"

	"github.com/authelia/authelia/v4/internal/authentication"
	"github.com/authelia/authelia/v4/internal/configuration/schema"
	"github.com/authelia/authelia/v4/internal/mocks"
	"github.com/authelia/authelia/v4/internal/model"
	"github.com/authelia/authelia/v4/internal/session"
)

func TestWeChatBearerRejectsMalformedCredentials(t *testing.T) {
	mock := mocks.NewMockAutheliaCtx(t)
	defer mock.Close()
	valid := strings.Repeat("a", 64)
	for _, value := range []string{"", "Basic " + valid, "Bearer short", "Bearer " + valid + " extra", "Bearer " + strings.Repeat("z", 64)} {
		mock.Ctx.Request.Header.Set("Authorization", value)
		if token := wechatBearer(mock.Ctx); token != "" {
			t.Fatalf("malformed bearer accepted: %q", value)
		}
	}
	mock.Ctx.Request.Header.Set("Authorization", "Bearer "+valid)
	if token := wechatBearer(mock.Ctx); token != valid {
		t.Fatal("valid bearer rejected")
	}
}

func TestWeChatConsumeRequiresSameOrigin(t *testing.T) {
	for _, tc := range []struct {
		name, origin, referer, site string
		want                        bool
	}{
		{"same origin", "https://login.example.com:8080", "", "same-origin", true},
		{"same origin referer fallback", "", "https://login.example.com:8080/path", "", true},
		{"wrong origin", "https://evil.example.com:8080", "", "same-site", false},
		{"cross site", "https://login.example.com:8080", "", "cross-site", false},
		{"missing origin and referer", "", "", "", false},
		{"null origin", "null", "", "", false},
		{"origin path", "https://login.example.com:8080/path", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := mocks.NewMockAutheliaCtx(t)
			defer mock.Close()
			if tc.origin != "" {
				mock.Ctx.Request.Header.Set("Origin", tc.origin)
			}
			if tc.referer != "" {
				mock.Ctx.Request.Header.Set("Referer", tc.referer)
			}
			if tc.site != "" {
				mock.Ctx.Request.Header.Set("Sec-Fetch-Site", tc.site)
			}
			if got := wechatSameOrigin(mock.Ctx); got != tc.want {
				t.Fatalf("same origin=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestWeChatBanCheckFailsClosedAndAudits(t *testing.T) {
	mock := mocks.NewMockAutheliaCtx(t)
	defer mock.Close()
	mock.StorageMock.EXPECT().LoadBannedIP(gomock.Any(), gomock.Any()).Return([]model.BannedIP{{}}, nil)
	mock.StorageMock.EXPECT().AppendAuthenticationLog(gomock.Any(), gomock.Any()).Return(nil)
	if wechatCheckBan(mock.Ctx, "alice") {
		t.Fatal("banned user was accepted")
	}
	if mock.Ctx.Response.StatusCode() != fasthttp.StatusUnauthorized {
		t.Fatalf("unexpected status: %d", mock.Ctx.Response.StatusCode())
	}
}

func TestWeChatDisabledWhenConfigurationIsMissing(t *testing.T) {
	mock := mocks.NewMockAutheliaCtx(t)
	defer mock.Close()
	mock.Ctx.Configuration.WeChat = nil
	if WeChatEnabled(mock.Ctx) {
		t.Fatal("WeChat unexpectedly enabled without configuration")
	}
	mock.Ctx.Configuration.WeChat = &schema.WeChat{AppID: "wx0123456789abcdef"}
	if WeChatEnabled(mock.Ctx) {
		t.Fatal("WeChat unexpectedly enabled without secret or directory")
	}
}

func TestWeChatRejectsInvalidTargetBeforeIssuingQR(t *testing.T) {
	mock := mocks.NewMockAutheliaCtx(t)
	defer mock.Close()
	mock.Ctx.Request.SetBodyString(`{"targetURL":"https://attacker.invalid/","requestMethod":"GET"}`)
	mock.Ctx.Request.Header.SetContentType("application/json")
	mock.Ctx.Request.Header.Set("Origin", "https://login.example.com:8080")
	(&WeChatHandlers{}).Start(mock.Ctx)
	if mock.Ctx.Response.StatusCode() != fasthttp.StatusBadRequest {
		t.Fatalf("unexpected status: %d", mock.Ctx.Response.StatusCode())
	}
}

type wechatRefreshProvider struct {
	authentication.UserProvider
	details *authentication.UserDetails
	err     error
	calls   int
}

func (p *wechatRefreshProvider) ResolveWeChatUser(appID, openID string) (*authentication.UserDetails, error) {
	p.calls++
	if appID != "wx0123456789abcdef" || openID != "open-id" {
		return nil, authentication.ErrUserNotFound
	}
	return p.details, p.err
}

func TestWeChatBrowserSessionRevalidatesBindingEveryTime(t *testing.T) {
	for _, tc := range []struct {
		name        string
		details     *authentication.UserDetails
		err         error
		wantInvalid bool
		wantGroup   string
	}{
		{"groups refresh", &authentication.UserDetails{Username: "Alice", Groups: []string{"current"}}, nil, false, "current"},
		{"identity unbound or account disabled", nil, authentication.ErrUserNotFound, true, ""},
		{"directory unavailable", nil, errors.New("database unavailable"), true, ""},
		{"different account by case", &authentication.UserDetails{Username: "alice", Groups: []string{"admin"}}, nil, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := mocks.NewMockAutheliaCtx(t)
			defer mock.Close()
			p := &wechatRefreshProvider{UserProvider: mock.UserProviderMock, details: tc.details, err: tc.err}
			mock.Ctx.Providers.UserProvider = p
			mock.Ctx.Configuration.WeChat = &schema.WeChat{AppID: "wx0123456789abcdef"}
			s := session.NewDefaultUserSession()
			s.SetOneFactorWeChat(time.Now(), &authentication.UserDetails{Username: "Alice", Groups: []string{"old"}})
			s.WeChatAppID, s.WeChatOpenID = "wx0123456789abcdef", "open-id"
			s.RefreshTTL = time.Now().Add(time.Hour)
			_, invalid := handleSessionValidateRefresh(mock.Ctx, &s, schema.NewRefreshIntervalDurationNever())
			if invalid != tc.wantInvalid {
				t.Fatalf("invalid=%v, want %v", invalid, tc.wantInvalid)
			}
			if p.calls != 1 {
				t.Fatalf("expected a fresh identity lookup, got %d", p.calls)
			}
			if tc.wantGroup != "" && (len(s.Groups) != 1 || s.Groups[0] != tc.wantGroup) {
				t.Fatalf("groups not refreshed: %v", s.Groups)
			}
		})
	}
}

func TestWeChatBrowserSessionRejectsChangedAppID(t *testing.T) {
	mock := mocks.NewMockAutheliaCtx(t)
	defer mock.Close()
	p := &wechatRefreshProvider{UserProvider: mock.UserProviderMock, details: &authentication.UserDetails{Username: "Alice"}}
	mock.Ctx.Providers.UserProvider = p
	mock.Ctx.Configuration.WeChat = &schema.WeChat{AppID: "wxproductionappid"}
	s := session.NewDefaultUserSession()
	s.SetOneFactorWeChat(time.Now(), &authentication.UserDetails{Username: "Alice"})
	s.WeChatAppID, s.WeChatOpenID = "wx0123456789abcdef", "open-id"
	_, invalid := handleSessionValidateRefresh(mock.Ctx, &s, schema.NewRefreshIntervalDurationNever())
	if !invalid || p.calls != 0 {
		t.Fatalf("changed AppID was accepted: invalid=%v, lookups=%d", invalid, p.calls)
	}
}

func TestWeChatUserGroupsAreAlwaysAnArray(t *testing.T) {
	data, err := json.Marshal(wechatUserData(&authentication.UserDetails{Username: "alice"}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"groups":[]`) {
		t.Fatalf("groups not represented as an array: %s", data)
	}
}
