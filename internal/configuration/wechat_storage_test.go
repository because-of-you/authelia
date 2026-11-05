// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

package configuration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/authelia/authelia/v4/internal/configuration/schema"
	"github.com/authelia/authelia/v4/internal/configuration/validator"
)

func TestWeChatStorageConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "configuration.yml")
	require.NoError(t, os.WriteFile(path, []byte("authentication_backend:\n  wechat_storage:\n    enabled: true\nwechat:\n  auto_register: true\n"), 0600))
	v := schema.NewStructValidator()
	keys, config, err := Load(v, NewFileSource(path))
	require.NoError(t, err)
	validator.ValidateKeys(keys, nil, DefaultEnvPrefix, v)
	require.Empty(t, v.Errors())
	require.NotNil(t, config.AuthenticationBackend.WeChatStorage)
	require.True(t, config.AuthenticationBackend.WeChatStorage.Enabled)
	require.NotNil(t, config.WeChat)
	require.True(t, config.WeChat.AutoRegister)
	validator.ValidateAuthenticationBackend(&config.AuthenticationBackend, v)
	require.Empty(t, v.Errors())
	require.True(t, config.AuthenticationBackend.PasswordReset.Disable)
}
