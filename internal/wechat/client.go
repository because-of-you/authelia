// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

// Package wechat implements the server-side WeChat mini program integration.
package wechat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var ErrWeChat = errors.New("wechat service request failed")

// ImageContentType recognizes the image formats returned by the WeChat QR API.
func ImageContentType(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}):
		return "image/png"
	case bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}):
		return "image/jpeg"
	default:
		return ""
	}
}

// Client sends requests only to the configured WeChat API endpoint.
type Client struct {
	AppID, AppSecret, QREnvVersion string
	baseURL                        string
	httpClient                     *http.Client
	mu                             sync.Mutex
	accessToken                    string
	tokenExpires                   time.Time
}

// NewClient creates a WeChat API client with bounded request timeouts.
func NewClient(appID, appSecret, qrEnvVersion string) *Client {
	if qrEnvVersion == "" {
		qrEnvVersion = "release"
	}
	return &Client{AppID: appID, AppSecret: appSecret, QREnvVersion: qrEnvVersion, baseURL: "https://api.weixin.qq.com", httpClient: &http.Client{Timeout: 10 * time.Second}}
}

// Valid returns whether the WeChat identity configuration is usable.
func (c *Client) Valid() bool {
	return c != nil && strings.HasPrefix(c.AppID, "wx") && len(c.AppID) == 18 && c.AppSecret != "" && (c.QREnvVersion == "release" || c.QREnvVersion == "trial" || c.QREnvVersion == "develop")
}

// ExchangeCode verifies a temporary wx.login code and returns the server-verified OpenID.
func (c *Client) ExchangeCode(ctx context.Context, code string) (openID string, err error) {
	if !c.Valid() || code == "" || len(code) > 256 {
		return "", ErrWeChat
	}
	values := url.Values{"appid": {c.AppID}, "secret": {c.AppSecret}, "js_code": {code}, "grant_type": {"authorization_code"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/sns/jscode2session?"+values.Encode(), nil)
	if err != nil {
		return "", ErrWeChat
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", ErrWeChat
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", ErrWeChat
	}
	var result struct {
		OpenID  string `json:"openid"`
		ErrCode int    `json:"errcode"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil || result.ErrCode != 0 || result.OpenID == "" || len(result.OpenID) > 128 {
		return "", ErrWeChat
	}
	return result.OpenID, nil
}

func (c *Client) token(ctx context.Context) (token string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.accessToken != "" && time.Now().Before(c.tokenExpires) {
		return c.accessToken, nil
	}
	values := url.Values{"grant_type": {"client_credential"}, "appid": {c.AppID}, "secret": {c.AppSecret}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/cgi-bin/token?"+values.Encode(), nil)
	if err != nil {
		return "", ErrWeChat
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", ErrWeChat
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", ErrWeChat
	}
	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		ErrCode     int    `json:"errcode"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil || result.ErrCode != 0 || result.AccessToken == "" || result.ExpiresIn <= 60 {
		return "", ErrWeChat
	}
	c.accessToken, c.tokenExpires = result.AccessToken, time.Now().Add(time.Duration(result.ExpiresIn-60)*time.Second)
	return c.accessToken, nil
}

// QRCode requests a mini program QR code whose scene is a one-time transaction.
func (c *Client) QRCode(ctx context.Context, scene string) ([]byte, error) {
	if !c.Valid() || scene == "" || len(scene) > 32 {
		return nil, ErrWeChat
	}
	token, err := c.token(ctx)
	if err != nil {
		return nil, ErrWeChat
	}
	body, err := json.Marshal(map[string]any{"scene": scene, "page": "pages/authorize/index", "check_path": false, "env_version": c.QREnvVersion})
	if err != nil {
		return nil, ErrWeChat
	}
	values := url.Values{"access_token": {token}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/wxa/getwxacodeunlimit?"+values.Encode(), bytes.NewReader(body))
	if err != nil {
		return nil, ErrWeChat
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, ErrWeChat
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, ErrWeChat
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil || ImageContentType(data) == "" {
		return nil, fmt.Errorf("%w: invalid QR image", ErrWeChat)
	}
	return data, nil
}
