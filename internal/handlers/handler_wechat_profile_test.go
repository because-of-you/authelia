// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/authelia/authelia/v4/internal/authentication"
	"github.com/authelia/authelia/v4/internal/mocks"
	"github.com/authelia/authelia/v4/internal/model"
	"github.com/authelia/authelia/v4/internal/session"
	"github.com/authelia/authelia/v4/internal/wechat"
	"github.com/stretchr/testify/require"
)

type testProfileStore struct {
	wechat.Storage
	session wechat.MiniSession
	profile wechat.Profile
	saves   int
}

func (s *testProfileStore) Session(context.Context, string, time.Time) (wechat.MiniSession, error) {
	return s.session, nil
}
func (s *testProfileStore) SaveProfile(_ context.Context, session wechat.MiniSession, profile wechat.Profile) error {
	s.session, s.profile = session, profile
	s.saves++
	return nil
}
func (s *testProfileStore) Profile(_ context.Context, session wechat.MiniSession) (wechat.Profile, error) {
	s.session = session
	return s.profile, nil
}
func (s *testProfileStore) LookupMiniUser(_ context.Context, appID, openID string) (*authentication.UserDetails, bool, error) {
	if appID != s.session.AppID || openID != s.session.OpenID {
		return nil, false, authentication.ErrUserNotFound
	}
	return &authentication.UserDetails{Username: s.session.Username}, false, nil
}

type testProfileProvider struct {
	*mocks.MockStorage
	store wechat.Storage
}

func (p *testProfileProvider) WeChatStore() wechat.Storage { return p.store }

func TestBrowserWeChatProfileUsesSessionIdentity(t *testing.T) {
	mock := mocks.NewMockAutheliaCtx(t)
	defer mock.Close()
	store := &testProfileStore{profile: wechat.Profile{Nickname: "微信昵称", Avatar: []byte{0xff, 0xd8, 0xff, 0xe0}}}
	mock.Ctx.Providers.StorageProvider = &testProfileProvider{MockStorage: mock.StorageMock, store: store}
	userSession := session.UserSession{Username: "Alice", WeChatAppID: "wx-test", WeChatOpenID: "open-id"}
	info := model.UserInfo{DisplayName: "Original"}
	applyWeChatProfile(mock.Ctx, userSession, &info)
	require.Equal(t, "Original", info.DisplayName)
	require.False(t, info.WeChatAvatar)
	userSession.AuthenticationMethodRefs.WeChat = true
	applyWeChatProfile(mock.Ctx, userSession, &info)
	require.Equal(t, "微信昵称", info.DisplayName)
	require.True(t, info.WeChatAvatar)
	require.Equal(t, wechat.MiniSession{Username: "Alice", AppID: "wx-test", OpenID: "open-id"}, store.session)
}

func TestWeChatProfileRequiresSessionAndValidImage(t *testing.T) {
	mock := mocks.NewMockAutheliaCtx(t)
	defer mock.Close()
	store := &testProfileStore{session: wechat.MiniSession{Username: "Alice", AppID: "wx0123456789abcdef", OpenID: "open-id"}}
	provider := &wechatRefreshProvider{UserProvider: mock.UserProviderMock, details: &authentication.UserDetails{Username: "Alice"}}
	h := &WeChatHandlers{client: wechat.NewClient("wx0123456789abcdef", "secret", "develop"), store: store, provider: provider}
	h.UpdateProfile(mock.Ctx)
	require.Equal(t, 401, mock.Ctx.Response.StatusCode())
	require.Zero(t, store.saves)
	mock.Ctx.Request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 64))
	mock.Ctx.Request.Header.SetContentType("application/json")
	mock.Ctx.Request.SetBodyString(`{"nickname":"Alice","avatar":"bm90IGFuIGltYWdl"}`)
	h.UpdateProfile(mock.Ctx)
	require.Equal(t, 400, mock.Ctx.Response.StatusCode())
	require.Zero(t, store.saves)
	var avatar bytes.Buffer
	require.NoError(t, png.Encode(&avatar, image.NewRGBA(image.Rect(0, 0, 1, 1))))
	body, err := json.Marshal(map[string]any{"nickname": "Alice", "avatar": avatar.Bytes()})
	require.NoError(t, err)
	mock.Ctx.Request.SetBody(body)
	h.UpdateProfile(mock.Ctx)
	require.Equal(t, 1, store.saves)
	require.Equal(t, "Alice", store.profile.Nickname)
	require.Equal(t, "open-id", store.session.OpenID)
}

func TestDisabledWeChatUserCannotApproveChallenge(t *testing.T) {
	mock := mocks.NewMockAutheliaCtx(t)
	defer mock.Close()
	store := &testProfileStore{session: wechat.MiniSession{Username: "Alice", AppID: "wx0123456789abcdef", OpenID: "open-id"}}
	h := &WeChatHandlers{client: wechat.NewClient("wx0123456789abcdef", "secret", "develop"), store: store}
	mock.Ctx.Request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 64))
	mock.Ctx.Request.Header.SetContentType("application/json")
	mock.Ctx.Request.SetBodyString(`{"transaction":"0123456789abcdef0123456789abcdef"}`)
	h.Complete(mock.Ctx)
	require.Equal(t, 403, mock.Ctx.Response.StatusCode())
	require.Contains(t, string(mock.Ctx.Response.Body()), "账户尚未启用")
}
