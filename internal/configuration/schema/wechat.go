// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

package schema

// WeChat configures the WeChat mini program identity and QR authorization flow.
// An absent or incomplete configuration leaves the endpoints disabled.
type WeChat struct {
	AppID        string `koanf:"app_id" yaml:"app_id,omitempty" toml:"app_id,omitempty" json:"app_id,omitempty"`
	AppSecret    string `koanf:"app_secret" yaml:"app_secret,omitempty" toml:"app_secret,omitempty" json:"-"`
	QREnvVersion string `koanf:"qr_env_version" yaml:"qr_env_version,omitempty" toml:"qr_env_version,omitempty" json:"qr_env_version,omitempty"`
	AutoRegister bool   `koanf:"auto_register" yaml:"auto_register,omitempty" toml:"auto_register,omitempty" json:"auto_register,omitempty"`
}
