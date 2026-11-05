// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"testing"

	"github.com/authelia/authelia/v4/internal/configuration/schema"
	"github.com/authelia/authelia/v4/internal/middlewares"
)

func TestWeChatServerRejectsIncompleteConfiguration(t *testing.T) {
	_, err := handlerMain(&schema.Configuration{AuthenticationBackend: schema.AuthenticationBackend{WeChatStorage: &schema.AuthenticationBackendWeChatStorage{Enabled: true}}}, middlewares.Providers{})
	if err == nil {
		t.Fatal("wechat_storage without wechat configuration was accepted")
	}

	for _, cfg := range []*schema.WeChat{
		{},
		{AppID: "wx0123456789abcdef"},
		{AppID: "wx0123456789abcdef", AppSecret: "secret"},
		{AppID: "wx0123456789abcdef", AppSecret: "secret", QREnvVersion: "invalid"},
	} {
		_, err := handlerMain(&schema.Configuration{WeChat: cfg}, middlewares.Providers{})
		if err == nil {
			t.Fatalf("incomplete WeChat configuration accepted: %+v", cfg)
		}
	}
}
