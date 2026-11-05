// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

package wechat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testClient(fn roundTripFunc) *Client {
	c := NewClient("wx0123456789abcdef", "secret-value", "develop")
	c.httpClient = &http.Client{Transport: fn}
	return c
}

func response(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func TestExchangeCode(t *testing.T) {
	c := testClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/sns/jscode2session" || r.URL.Query().Get("appid") != "wx0123456789abcdef" || r.URL.Query().Get("js_code") != "one-time-code" {
			t.Errorf("unexpected exchange request: %s", r.URL.Path)
		}
		return response(`{"openid":"server-verified-openid","session_key":"never-return"}`), nil
	})
	got, err := c.ExchangeCode(context.Background(), "one-time-code")
	if err != nil || got != "server-verified-openid" {
		t.Fatalf("unexpected exchange result: %q %v", got, err)
	}
}

func TestExchangeCodeRejectsWeChatErrorsAndDoesNotExposeSecret(t *testing.T) {
	c := testClient(func(r *http.Request) (*http.Response, error) {
		return response(`{"errcode":40029,"errmsg":"invalid code"}`), nil
	})
	if _, err := c.ExchangeCode(context.Background(), "bad"); !errors.Is(err, ErrWeChat) {
		t.Fatalf("expected WeChat failure, got %v", err)
	}
	c.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, errors.New("secret-value") })}
	if _, err := c.ExchangeCode(context.Background(), "bad"); err == nil || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("secret leaked in error: %v", err)
	}
}

func TestQRCodeUsesFixedPageAndCachesAccessToken(t *testing.T) {
	tokenCalls, imageCalls := 0, 0
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 1, 2}
	c := testClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			tokenCalls++
			return response(`{"access_token":"token","expires_in":7200}`), nil
		case "/wxa/getwxacodeunlimit":
			imageCalls++
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["scene"] != "0123456789abcdef0123456789abcdef" || body["page"] != "pages/authorize/index" || body["env_version"] != "develop" {
				t.Errorf("unexpected QR request: %v", body)
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(png)), Header: make(http.Header)}, nil
		default:
			t.Fatalf("unexpected API path: %s", r.URL.Path)
			return nil, nil
		}
	})
	for i := 0; i < 2; i++ {
		got, err := c.QRCode(context.Background(), "0123456789abcdef0123456789abcdef")
		if err != nil || !bytes.Equal(got, png) {
			t.Fatalf("unexpected QR result: %v", err)
		}
	}
	if tokenCalls != 1 || imageCalls != 2 {
		t.Fatalf("token requests=%d, QR requests=%d", tokenCalls, imageCalls)
	}
}

func TestQRCodeRejectsWeChatErrorBody(t *testing.T) {
	c := testClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/cgi-bin/token" {
			return response(`{"access_token":"token","expires_in":7200}`), nil
		}
		return response(`{"errcode":40001,"errmsg":"credential error"}`), nil
	})
	if _, err := c.QRCode(context.Background(), "0123456789abcdef0123456789abcdef"); !errors.Is(err, ErrWeChat) {
		t.Fatalf("expected error response, got %v", err)
	}
}

func TestImageContentType(t *testing.T) {
	cases := []struct {
		data []byte
		want string
	}{
		{[]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, "image/png"},
		{[]byte{0xff, 0xd8, 0xff, 0xe0}, "image/jpeg"},
		{[]byte(`{"errcode":40001}`), ""},
		{[]byte{0xff, 0xd8}, ""},
	}
	for _, tc := range cases {
		if got := ImageContentType(tc.data); got != tc.want {
			t.Fatalf("format %x: got %q, want %q", tc.data, got, tc.want)
		}
	}
}
