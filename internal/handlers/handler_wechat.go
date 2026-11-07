// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/valyala/fasthttp"

	"github.com/authelia/authelia/v4/internal/authentication"
	"github.com/authelia/authelia/v4/internal/middlewares"
	"github.com/authelia/authelia/v4/internal/regulation"
	"github.com/authelia/authelia/v4/internal/wechat"
)

type wechatUserProvider interface {
	authentication.UserProvider
	ResolveWeChatUser(appID, openID string) (*authentication.UserDetails, error)
}

// Profile reads only the currently authenticated user's user-supplied profile.
func (h *WeChatHandlers) Profile(ctx *middlewares.AutheliaCtx) {
	auth, ok := h.authenticated(ctx)
	if !ok {
		return
	}
	profile, err := h.store.Profile(ctx, auth.Session)
	if errors.Is(err, wechat.ErrNotFound) {
		if err = ctx.SetJSONBody(map[string]any{"nickname": "", "avatar": "", "email": ""}); err != nil {
			ctx.Logger.Error("Failed to write profile response")
		}
		return
	}
	if err != nil {
		wechatError(ctx, fasthttp.StatusServiceUnavailable, "资料暂不可用")
		return
	}
	email := ""
	if profile.Email != nil {
		email = *profile.Email
	}
	if err = ctx.SetJSONBody(map[string]any{"nickname": profile.Nickname, "avatar": profile.Avatar, "email": email}); err != nil {
		ctx.Logger.Error("Failed to write profile response")
	}
}

// UpdateProfile accepts a small JPEG or PNG and a display name for the current user.
func (h *WeChatHandlers) UpdateProfile(ctx *middlewares.AutheliaCtx) {
	auth, ok := h.authenticated(ctx)
	if !ok {
		return
	}
	var body struct {
		Nickname string  `json:"nickname"`
		Avatar   []byte  `json:"avatar"`
		Email    *string `json:"email"`
	}
	contentType := strings.TrimSpace(strings.SplitN(string(ctx.Request.Header.ContentType()), ";", 2)[0])
	if !strings.EqualFold(contentType, "application/json") || len(ctx.PostBody()) == 0 || len(ctx.PostBody()) > 400000 || json.Unmarshal(ctx.PostBody(), &body) != nil {
		wechatError(ctx, fasthttp.StatusBadRequest, "资料格式无效")
		return
	}
	body.Nickname = strings.TrimSpace(body.Nickname)
	if body.Email != nil {
		value := strings.ToLower(strings.TrimSpace(*body.Email))
		if value != "" {
			parsed, parseErr := mail.ParseAddress(value)
			if parseErr != nil || parsed.Address != value || parsed.Name != "" || len(value) > 320 || !strings.Contains(value, "@") || !isASCII(value) {
				wechatError(ctx, fasthttp.StatusBadRequest, "邮箱格式无效")
				return
			}
		}
		body.Email = &value
	}
	imageConfig, format, err := image.DecodeConfig(bytes.NewReader(body.Avatar))
	if body.Nickname == "" || !utf8.ValidString(body.Nickname) || utf8.RuneCountInString(body.Nickname) > 30 || len(body.Avatar) == 0 || len(body.Avatar) > 256*1024 || err != nil || (format != "png" && format != "jpeg") || imageConfig.Width <= 0 || imageConfig.Height <= 0 || imageConfig.Width > 2048 || imageConfig.Height > 2048 {
		wechatError(ctx, fasthttp.StatusBadRequest, "请选择不超过256KB的PNG或JPEG头像并填写昵称")
		return
	}
	if _, _, err = image.Decode(bytes.NewReader(body.Avatar)); err != nil {
		wechatError(ctx, fasthttp.StatusBadRequest, "头像图片无效")
		return
	}
	if err = h.store.SaveProfile(ctx, auth.Session, wechat.Profile{Nickname: body.Nickname, Avatar: body.Avatar, Email: body.Email}); err != nil {
		if errors.Is(err, wechat.ErrNotFound) {
			wechatError(ctx, fasthttp.StatusUnauthorized, "账户不可用")
		} else if errors.Is(err, wechat.ErrConflict) {
			wechatError(ctx, fasthttp.StatusConflict, "邮箱已被其他账户使用")
		} else {
			wechatError(ctx, fasthttp.StatusServiceUnavailable, "资料保存失败")
		}
		return
	}
	ctx.ReplyOK()
}

func isASCII(value string) bool {
	for _, r := range value {
		if r > 127 {
			return false
		}
	}
	return true
}

// WeChatEnabled reports whether the configured provider and WeChat client can be used.
func WeChatEnabled(ctx *middlewares.AutheliaCtx) bool {
	if ctx.Configuration.WeChat == nil {
		return false
	}
	cfg := ctx.Configuration.WeChat
	client := wechat.NewClient(cfg.AppID, cfg.AppSecret, cfg.QREnvVersion)
	_, ok := ctx.Providers.UserProvider.(wechatUserProvider)
	return client.Valid() && ok && wechatStore(ctx.Providers.StorageProvider) != nil
}

func wechatStore(storageProvider any) wechat.Storage {
	if p, ok := storageProvider.(interface{ WeChatStore() wechat.Storage }); ok {
		return p.WeChatStore()
	}
	return nil
}

// WeChatHandlers owns the client and persistent store for WeChat authentication.
type WeChatHandlers struct {
	client   *wechat.Client
	store    wechat.Storage
	provider wechatUserProvider
}

// NewWeChatHandlers returns nil unless the integration has been explicitly configured.
func NewWeChatHandlers(appID, appSecret, env string, provider authentication.UserProvider, storageProvider any) *WeChatHandlers {
	p, ok := provider.(wechatUserProvider)
	client := wechat.NewClient(appID, appSecret, env)
	store := wechatStore(storageProvider)
	if !ok || !client.Valid() || store == nil {
		return nil
	}
	return &WeChatHandlers{client: client, store: store, provider: p}
}

func wechatError(ctx *middlewares.AutheliaCtx, status int, message string) {
	if err := ctx.ReplyJSON(middlewares.ErrorResponse{Status: "KO", Message: message}, status); err != nil {
		ctx.Logger.WithError(err).Error("Failed to write WeChat response")
	}
}

func wechatBody(ctx *middlewares.AutheliaCtx, v any) bool {
	contentType := strings.TrimSpace(strings.SplitN(string(ctx.Request.Header.ContentType()), ";", 2)[0])
	return strings.EqualFold(contentType, "application/json") && len(ctx.PostBody()) > 0 && len(ctx.PostBody()) <= 8192 && json.Unmarshal(ctx.PostBody(), v) == nil
}

func wechatRandom(size int) (string, error) {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func wechatValidHex(value string, bytes int) bool {
	if len(value) != bytes*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func wechatBearer(ctx *middlewares.AutheliaCtx) string {
	parts := strings.Fields(string(ctx.Request.Header.Peek("Authorization")))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || !wechatValidHex(parts[1], 32) {
		return ""
	}
	return parts[1]
}

type wechatUser struct {
	Username    string   `json:"username"`
	DisplayName string   `json:"display_name"`
	Groups      []string `json:"groups"`
	Enabled     bool     `json:"enabled"`
}

func wechatUserData(details *authentication.UserDetails, enabled bool) wechatUser {
	groups := details.Groups
	if groups == nil {
		groups = []string{}
	}
	return wechatUser{Username: details.Username, DisplayName: details.DisplayName, Groups: groups, Enabled: enabled}
}

type wechatAuthenticatedSession struct {
	Details *authentication.UserDetails
	Enabled bool
	Session wechat.MiniSession
	Token   string
}

func (h *WeChatHandlers) authenticated(ctx *middlewares.AutheliaCtx) (*wechatAuthenticatedSession, bool) {
	token := wechatBearer(ctx)
	if token == "" {
		wechatError(ctx, fasthttp.StatusUnauthorized, "请先登录")
		return nil, false
	}
	stored, err := h.store.Session(ctx, token, time.Now())
	if err != nil {
		if errors.Is(err, wechat.ErrNotFound) {
			wechatError(ctx, fasthttp.StatusUnauthorized, "登录已过期，请重新登录")
		} else {
			wechatError(ctx, fasthttp.StatusServiceUnavailable, "认证服务暂不可用")
		}
		return nil, false
	}
	if stored.AppID != h.client.AppID || stored.OpenID == "" {
		wechatError(ctx, fasthttp.StatusUnauthorized, "账户不可用")
		return nil, false
	}
	details, enabled, err := h.store.LookupMiniUser(ctx, stored.AppID, stored.OpenID)
	if err != nil || details == nil || details.Username == "" || details.Username != stored.Username {
		wechatError(ctx, fasthttp.StatusUnauthorized, "账户不可用")
		return nil, false
	}
	return &wechatAuthenticatedSession{Details: details, Enabled: enabled, Session: stored, Token: token}, true
}

func wechatCheckBan(ctx *middlewares.AutheliaCtx, username string) bool {
	if ctx.Providers.Regulator == nil {
		wechatError(ctx, fasthttp.StatusServiceUnavailable, "认证服务暂不可用")
		return false
	}
	ban, _, expires, err := ctx.Providers.Regulator.BanCheck(ctx, username)
	if err != nil {
		doMarkAuthenticationAttempt(ctx, false, regulation.NewBan(ban, username, expires), regulation.AuthType1FA, err)
		wechatError(ctx, fasthttp.StatusUnauthorized, "认证失败")
		return false
	}
	return true
}

// Login exchanges a wx.login code and issues an opaque mini program session.
func (h *WeChatHandlers) Login(ctx *middlewares.AutheliaCtx) {
	var body struct {
		Code string `json:"code"`
	}
	if !wechatBody(ctx, &body) || body.Code == "" || len(body.Code) > 256 {
		wechatError(ctx, fasthttp.StatusBadRequest, "登录请求无效")
		return
	}
	openID, err := h.client.ExchangeCode(ctx, body.Code)
	if err != nil {
		if ctx.Providers.Regulator != nil {
			doMarkAuthenticationAttempt(ctx, false, regulation.NewBan(regulation.BanTypeUnknown, "", nil), regulation.AuthType1FA, err)
		}
		wechatError(ctx, fasthttp.StatusUnauthorized, "微信登录失败，请重试")
		return
	}
	details, enabled, err := h.store.LookupMiniUser(ctx, h.client.AppID, openID)
	if errors.Is(err, authentication.ErrUserNotFound) {
		registerErr := h.store.RegisterIdentity(ctx, h.client.AppID, openID)
		// A concurrent registration may have completed first; always look up the identity again.
		details, enabled, err = h.store.LookupMiniUser(ctx, h.client.AppID, openID)
		if registerErr != nil && err != nil && !errors.Is(registerErr, wechat.ErrNotFound) {
			ctx.Logger.Error("Failed to register WeChat account in storage")
			wechatError(ctx, fasthttp.StatusServiceUnavailable, "认证服务暂不可用")
			return
		}
	}
	if err != nil || details == nil || details.Username == "" {
		if ctx.Providers.Regulator != nil {
			doMarkAuthenticationAttempt(ctx, false, regulation.NewBan(regulation.BanTypeUnknown, "", nil), regulation.AuthType1FA, err)
		}
		wechatError(ctx, fasthttp.StatusUnauthorized, "账户不可用")
		return
	}
	if !wechatCheckBan(ctx, details.Username) {
		return
	}
	token, err := wechatRandom(32)
	if err != nil {
		wechatError(ctx, fasthttp.StatusServiceUnavailable, "认证服务暂不可用")
		return
	}
	expires := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	if err = h.store.CreateSession(ctx, token, wechat.MiniSession{Username: details.Username, AppID: h.client.AppID, OpenID: openID, ExpiresAt: expires}); err != nil {
		wechatError(ctx, fasthttp.StatusServiceUnavailable, "认证服务暂不可用")
		return
	}
	doMarkAuthenticationAttempt(ctx, true, regulation.NewBan(regulation.BanTypeNone, details.Username, nil), regulation.AuthType1FA, nil)
	if err = ctx.SetJSONBody(map[string]any{"access_token": token, "expires_at": expires, "user": wechatUserData(details, enabled)}); err != nil {
		ctx.Logger.WithError(err).Error("Failed to write WeChat login response")
	}
}

// User returns details for a currently valid mini program session.
func (h *WeChatHandlers) User(ctx *middlewares.AutheliaCtx) {
	auth, ok := h.authenticated(ctx)
	if !ok {
		return
	}
	if err := ctx.SetJSONBody(map[string]any{"expires_at": auth.Session.ExpiresAt, "user": wechatUserData(auth.Details, auth.Enabled)}); err != nil {
		ctx.Logger.WithError(err).Error("Failed to write WeChat user response")
	}
}

// Logout revokes the mini program session without affecting browser sessions.
func (h *WeChatHandlers) Logout(ctx *middlewares.AutheliaCtx) {
	auth, ok := h.authenticated(ctx)
	if !ok {
		return
	}
	if err := h.store.DeleteSession(ctx, auth.Token); err != nil {
		wechatError(ctx, fasthttp.StatusServiceUnavailable, "认证服务暂不可用")
		return
	}
	ctx.ReplyOK()
}

type wechatStartRequest struct {
	TargetURL     string `json:"targetURL"`
	RequestMethod string `json:"requestMethod"`
	FlowID        string `json:"flowID"`
	Flow          string `json:"flow"`
	Subflow       string `json:"subflow"`
	UserCode      string `json:"userCode"`
}

func wechatChallengeInput(ctx *middlewares.AutheliaCtx) string {
	return string(ctx.QueryArgs().Peek("transaction"))
}

// Start creates a QR challenge bound to the requesting browser's anonymous session.
func (h *WeChatHandlers) Start(ctx *middlewares.AutheliaCtx) {
	if !wechatSameOrigin(ctx) {
		wechatError(ctx, fasthttp.StatusForbidden, "授权请求来源无效")
		return
	}
	var body wechatStartRequest
	if !wechatBody(ctx, &body) || len(body.TargetURL) > 2048 || len(body.FlowID) > 128 || len(body.Flow) > 128 || len(body.Subflow) > 128 || len(body.UserCode) > 128 {
		wechatError(ctx, fasthttp.StatusBadRequest, "授权请求无效")
		return
	}
	if body.RequestMethod == "" {
		body.RequestMethod = http.MethodGet
	}
	body.RequestMethod = strings.ToUpper(body.RequestMethod)
	if len(body.RequestMethod) > 16 || strings.IndexFunc(body.RequestMethod, func(r rune) bool { return r < 'A' || r > 'Z' }) >= 0 {
		wechatError(ctx, fasthttp.StatusBadRequest, "授权请求无效")
		return
	}
	var target *url.URL
	var err error
	if body.TargetURL != "" {
		if target, err = url.ParseRequestURI(body.TargetURL); err != nil || target.User != nil || target.Hostname() == "" || !ctx.IsSafeRedirectionTargetURI(target) {
			wechatError(ctx, fasthttp.StatusBadRequest, "访问目标无效")
			return
		}
	} else {
		if target = ctx.GetDefaultRedirectionURL(); target == nil {
			target, err = ctx.IssuerURL()
		}
		if err != nil || target == nil || target.Hostname() == "" {
			wechatError(ctx, fasthttp.StatusBadRequest, "访问目标无效")
			return
		}
	}
	provider, err := ctx.GetSessionProvider()
	if err != nil {
		wechatError(ctx, fasthttp.StatusServiceUnavailable, "认证服务暂不可用")
		return
	}
	userSession, err := provider.GetSession(ctx.RequestCtx)
	if err != nil {
		wechatError(ctx, fasthttp.StatusServiceUnavailable, "认证服务暂不可用")
		return
	}
	if userSession.WeChatBrowserBinding == "" {
		if userSession.WeChatBrowserBinding, err = wechatRandom(32); err != nil {
			wechatError(ctx, fasthttp.StatusServiceUnavailable, "认证服务暂不可用")
			return
		}
		if err = provider.SaveSession(ctx.RequestCtx, userSession); err != nil {
			wechatError(ctx, fasthttp.StatusServiceUnavailable, "认证服务暂不可用")
			return
		}
	}
	transaction, err := wechatRandom(16)
	if err != nil {
		wechatError(ctx, fasthttp.StatusServiceUnavailable, "认证服务暂不可用")
		return
	}
	image, err := h.client.QRCode(ctx, transaction)
	if err != nil {
		wechatError(ctx, fasthttp.StatusServiceUnavailable, "无法生成微信小程序码")
		return
	}
	expires := time.Now().UTC().Add(5 * time.Minute).Truncate(time.Second)
	challenge := wechat.Challenge{TargetURL: body.TargetURL, TargetDomain: target.Hostname(), RequestMethod: body.RequestMethod, FlowID: body.FlowID, Flow: body.Flow, Subflow: body.Subflow, UserCode: body.UserCode, ExpiresAt: expires}
	if err = h.store.CreateChallenge(ctx, transaction, userSession.WeChatBrowserBinding, challenge, image); err != nil {
		wechatError(ctx, fasthttp.StatusServiceUnavailable, "认证服务暂不可用")
		return
	}
	imageURL := strings.TrimSuffix(ctx.Configuration.Server.Address.RouterPath(), "/") + "/api/qr-login/image?transaction=" + transaction
	if err = ctx.SetJSONBody(map[string]any{"transaction": transaction, "qr_code": imageURL, "expires_at": expires}); err != nil {
		ctx.Logger.WithError(err).Error("Failed to write WeChat challenge response")
	}
}

// Image serves a QR only to the browser which created the challenge.
func (h *WeChatHandlers) Image(ctx *middlewares.AutheliaCtx) {
	transaction := wechatChallengeInput(ctx)
	if !wechatValidHex(transaction, 16) {
		wechatError(ctx, fasthttp.StatusBadRequest, "授权请求无效")
		return
	}
	provider, err := ctx.GetSessionProvider()
	if err != nil {
		wechatError(ctx, fasthttp.StatusServiceUnavailable, "认证服务暂不可用")
		return
	}
	userSession, err := provider.GetSession(ctx.RequestCtx)
	if err != nil || userSession.WeChatBrowserBinding == "" {
		wechatError(ctx, fasthttp.StatusUnauthorized, "浏览器会话无效")
		return
	}
	image, err := h.store.Image(ctx, transaction, userSession.WeChatBrowserBinding, time.Now())
	if err != nil {
		if errors.Is(err, wechat.ErrNotFound) {
			wechatError(ctx, fasthttp.StatusNotFound, "授权请求不存在")
		} else {
			wechatError(ctx, fasthttp.StatusServiceUnavailable, "认证服务暂不可用")
		}
		return
	}
	contentType := wechat.ImageContentType(image)
	if contentType == "" {
		wechatError(ctx, fasthttp.StatusServiceUnavailable, "二维码不可用")
		return
	}
	ctx.SetContentType(contentType)
	ctx.SetBody(image)
}

// Scan records that a QR code was opened; possession alone never authorizes login.
func (h *WeChatHandlers) Scan(ctx *middlewares.AutheliaCtx) {
	var body struct {
		Transaction string `json:"transaction"`
	}
	if !wechatBody(ctx, &body) || !wechatValidHex(body.Transaction, 16) {
		wechatError(ctx, fasthttp.StatusBadRequest, "授权请求无效")
		return
	}
	if err := h.store.MarkScanned(ctx, body.Transaction, time.Now()); err != nil {
		if errors.Is(err, wechat.ErrNotFound) {
			wechatError(ctx, fasthttp.StatusConflict, "授权请求已失效")
		} else {
			wechatError(ctx, fasthttp.StatusServiceUnavailable, "认证服务暂不可用")
		}
		return
	}
	ctx.ReplyOK()
}

// Challenge returns server-derived target and expiry information for the confirmation screen.
func (h *WeChatHandlers) Challenge(ctx *middlewares.AutheliaCtx) {
	if _, ok := h.authenticated(ctx); !ok {
		return
	}
	transaction := wechatChallengeInput(ctx)
	if !wechatValidHex(transaction, 16) {
		wechatError(ctx, fasthttp.StatusBadRequest, "授权请求无效")
		return
	}
	c, err := h.store.Challenge(ctx, transaction, "")
	if err != nil {
		if errors.Is(err, wechat.ErrNotFound) {
			wechatError(ctx, fasthttp.StatusNotFound, "授权请求不存在")
		} else {
			wechatError(ctx, fasthttp.StatusServiceUnavailable, "认证服务暂不可用")
		}
		return
	}
	if (c.Status == "pending" || c.Status == "scanned" || c.Status == "approved") && !c.ExpiresAt.After(time.Now()) {
		c.Status = "expired"
	}
	if err = ctx.SetJSONBody(map[string]any{"target_domain": c.TargetDomain, "expires_at": c.ExpiresAt, "status": c.Status}); err != nil {
		ctx.Logger.WithError(err).Error("Failed to write WeChat challenge response")
	}
}

func (h *WeChatHandlers) decide(ctx *middlewares.AutheliaCtx, decision string) {
	auth, ok := h.authenticated(ctx)
	if !ok {
		return
	}
	if decision == "approved" && !auth.Enabled {
		wechatError(ctx, fasthttp.StatusForbidden, "账户尚未启用，无法授权登录")
		return
	}
	var body struct {
		Transaction string `json:"transaction"`
	}
	if !wechatBody(ctx, &body) || !wechatValidHex(body.Transaction, 16) {
		wechatError(ctx, fasthttp.StatusBadRequest, "授权请求无效")
		return
	}
	if err := h.store.Decide(ctx, body.Transaction, auth.Session, decision, time.Now()); err != nil {
		if errors.Is(err, wechat.ErrNotFound) {
			wechatError(ctx, fasthttp.StatusConflict, "授权请求已失效")
		} else {
			wechatError(ctx, fasthttp.StatusServiceUnavailable, "认证服务暂不可用")
		}
		return
	}
	if err := ctx.SetJSONBody(map[string]any{"status": decision}); err != nil {
		ctx.Logger.WithError(err).Error("Failed to write WeChat decision response")
	}
}

func (h *WeChatHandlers) Complete(ctx *middlewares.AutheliaCtx) { h.decide(ctx, "approved") }
func (h *WeChatHandlers) Reject(ctx *middlewares.AutheliaCtx)   { h.decide(ctx, "rejected") }

// Status is read-only and only reveals the challenge to its originating browser.
func (h *WeChatHandlers) Status(ctx *middlewares.AutheliaCtx) {
	transaction := wechatChallengeInput(ctx)
	if !wechatValidHex(transaction, 16) {
		wechatError(ctx, fasthttp.StatusBadRequest, "授权请求无效")
		return
	}
	provider, err := ctx.GetSessionProvider()
	if err != nil {
		wechatError(ctx, fasthttp.StatusServiceUnavailable, "认证服务暂不可用")
		return
	}
	userSession, err := provider.GetSession(ctx.RequestCtx)
	if err != nil || userSession.WeChatBrowserBinding == "" {
		wechatError(ctx, fasthttp.StatusUnauthorized, "浏览器会话无效")
		return
	}
	c, err := h.store.Challenge(ctx, transaction, userSession.WeChatBrowserBinding)
	if err != nil {
		if errors.Is(err, wechat.ErrNotFound) {
			wechatError(ctx, fasthttp.StatusNotFound, "授权请求不存在")
		} else {
			wechatError(ctx, fasthttp.StatusServiceUnavailable, "认证服务暂不可用")
		}
		return
	}
	if (c.Status == "pending" || c.Status == "scanned" || c.Status == "approved") && !c.ExpiresAt.After(time.Now()) {
		c.Status = "expired"
	}
	if err = ctx.SetJSONBody(map[string]any{"status": c.Status}); err != nil {
		ctx.Logger.WithError(err).Error("Failed to write WeChat status response")
	}
}

func wechatSameOrigin(ctx *middlewares.AutheliaCtx) bool {
	if site := string(ctx.Request.Header.Peek("Sec-Fetch-Site")); site != "" && site != "same-origin" && site != "none" {
		return false
	}
	issuer, err := ctx.IssuerURL()
	if err != nil {
		return false
	}
	value := string(ctx.Request.Header.Peek("Origin"))
	fromOrigin := value != ""
	if value == "" {
		value = string(ctx.Request.Header.Referer())
	}
	if value == "" || value == "null" {
		return false
	}
	origin, err := url.Parse(value)
	if err != nil || origin.User != nil || origin.Scheme == "" || origin.Hostname() == "" {
		return false
	}
	if fromOrigin && (origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "") {
		return false
	}
	port := func(u *url.URL) string {
		if u.Port() != "" {
			return u.Port()
		}
		if strings.EqualFold(u.Scheme, "https") {
			return "443"
		}
		if strings.EqualFold(u.Scheme, "http") {
			return "80"
		}
		return ""
	}
	return strings.EqualFold(origin.Scheme, issuer.Scheme) && strings.EqualFold(origin.Hostname(), issuer.Hostname()) && port(origin) == port(issuer)
}

// Consume changes the browser's authentication state only on a same-origin POST.
func (h *WeChatHandlers) Consume(ctx *middlewares.AutheliaCtx) {
	if !wechatSameOrigin(ctx) {
		wechatError(ctx, fasthttp.StatusForbidden, "授权请求来源无效")
		return
	}
	var body struct {
		Transaction string `json:"transaction"`
	}
	if !wechatBody(ctx, &body) || !wechatValidHex(body.Transaction, 16) {
		wechatError(ctx, fasthttp.StatusBadRequest, "授权请求无效")
		return
	}
	provider, err := ctx.GetSessionProvider()
	if err != nil {
		wechatError(ctx, fasthttp.StatusServiceUnavailable, "认证服务暂不可用")
		return
	}
	userSession, err := provider.GetSession(ctx.RequestCtx)
	if err != nil || userSession.WeChatBrowserBinding == "" {
		wechatError(ctx, fasthttp.StatusUnauthorized, "浏览器会话无效")
		return
	}
	c, err := h.store.Challenge(ctx, body.Transaction, userSession.WeChatBrowserBinding)
	if err != nil {
		if errors.Is(err, wechat.ErrNotFound) {
			wechatError(ctx, fasthttp.StatusNotFound, "授权请求不存在")
		} else {
			wechatError(ctx, fasthttp.StatusServiceUnavailable, "认证服务暂不可用")
		}
		return
	}
	if c.Status != "approved" || !c.ExpiresAt.After(time.Now()) {
		wechatError(ctx, fasthttp.StatusConflict, "授权请求已失效")
		return
	}
	if c.AppID != h.client.AppID || c.OpenID == "" || c.Username == "" {
		wechatError(ctx, fasthttp.StatusUnauthorized, "账户不可用")
		return
	}
	details, err := h.provider.ResolveWeChatUser(c.AppID, c.OpenID)
	if err != nil || details == nil || details.Username == "" || details.Username != c.Username {
		if ctx.Providers.Regulator != nil {
			doMarkAuthenticationAttempt(ctx, false, regulation.NewBan(regulation.BanTypeUnknown, "", nil), regulation.AuthType1FA, err)
		}
		wechatError(ctx, fasthttp.StatusUnauthorized, "账户不可用")
		return
	}
	if !wechatCheckBan(ctx, details.Username) {
		return
	}
	if err = h.store.Consume(ctx, body.Transaction, userSession.WeChatBrowserBinding, time.Now()); err != nil {
		if errors.Is(err, wechat.ErrNotFound) {
			wechatError(ctx, fasthttp.StatusConflict, "授权请求已失效")
		} else {
			wechatError(ctx, fasthttp.StatusServiceUnavailable, "认证服务暂不可用")
		}
		return
	}
	userSession = provider.NewDefaultUserSession()
	if err = provider.SaveSession(ctx.RequestCtx, userSession); err != nil {
		wechatError(ctx, fasthttp.StatusServiceUnavailable, "认证服务暂不可用")
		return
	}
	if err = provider.RegenerateSession(ctx.RequestCtx); err != nil {
		wechatError(ctx, fasthttp.StatusServiceUnavailable, "认证服务暂不可用")
		return
	}
	userSession.SetOneFactorWeChat(ctx.GetClock().Now(), details)
	userSession.WeChatAppID, userSession.WeChatOpenID = c.AppID, c.OpenID
	if ctx.Configuration.AuthenticationBackend.RefreshInterval.Update() {
		userSession.RefreshTTL = ctx.GetClock().Now().Add(ctx.Configuration.AuthenticationBackend.RefreshInterval.Value())
	}
	if err = provider.SaveSession(ctx.RequestCtx, userSession); err != nil {
		wechatError(ctx, fasthttp.StatusServiceUnavailable, "认证服务暂不可用")
		return
	}
	doMarkAuthenticationAttempt(ctx, true, regulation.NewBan(regulation.BanTypeNone, details.Username, nil), regulation.AuthType1FA, nil)
	if c.Flow != "" {
		handleFlowResponse(ctx, &userSession, c.FlowID, c.Flow, c.Subflow, c.UserCode)
	} else {
		Handle1FAResponse(ctx, c.TargetURL, c.RequestMethod, details.Username, details.Groups)
	}
	var response struct {
		Status string         `json:"status"`
		Data   map[string]any `json:"data"`
	}
	if err = json.Unmarshal(ctx.Response.Body(), &response); err != nil || response.Status != "OK" {
		return
	}
	if response.Data == nil {
		response.Data = make(map[string]any)
	}
	response.Data["status"] = "consumed"
	if err = ctx.SetJSONBody(response.Data); err != nil {
		ctx.Logger.WithError(err).Error("Failed to write WeChat status response")
	}
}
