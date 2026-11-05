// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

package authentication

import (
	"context"
	"errors"
	"time"
)

// ErrWeChatPasswordDisabled indicates WeChat-only accounts cannot use passwords.
var ErrWeChatPasswordDisabled = errors.New("password authentication is disabled for the WeChat directory")

// WeChatDirectory is the subset of configured storage required by the user provider.
type WeChatDirectory interface {
	LookupWeChatUser(ctx context.Context, appID, openID, username string) (*UserDetails, error)
}

// WeChatStorageUserProvider uses Authelia's configured database as its user directory.
type WeChatStorageUserProvider struct{ directory WeChatDirectory }

func NewWeChatStorageUserProvider(directory WeChatDirectory) *WeChatStorageUserProvider {
	return &WeChatStorageUserProvider{directory: directory}
}

func (p *WeChatStorageUserProvider) StartupCheck() error {
	if p.directory == nil {
		return ErrOperationFailed
	}
	return nil
}

func (p *WeChatStorageUserProvider) lookup(appID, openID, username string) (*UserDetails, error) {
	if p.directory == nil {
		return nil, ErrOperationFailed
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return p.directory.LookupWeChatUser(ctx, appID, openID, username)
}

func (p *WeChatStorageUserProvider) ResolveWeChatUser(appID, openID string) (*UserDetails, error) {
	if appID == "" || openID == "" {
		return nil, ErrUserNotFound
	}
	return p.lookup(appID, openID, "")
}

func (p *WeChatStorageUserProvider) GetDetails(username string) (*UserDetails, error) {
	if username == "" {
		return nil, ErrUserNotFound
	}
	return p.lookup("", "", username)
}

func (p *WeChatStorageUserProvider) GetDetailsExtended(username string) (*UserDetailsExtended, error) {
	details, err := p.GetDetails(username)
	if err != nil {
		return nil, err
	}
	return &UserDetailsExtended{UserDetails: details}, nil
}

func (p *WeChatStorageUserProvider) CheckUserPassword(_, _ string) (bool, error) { return false, nil }
func (p *WeChatStorageUserProvider) UpdatePassword(_, _ string) error {
	return ErrWeChatPasswordDisabled
}
func (p *WeChatStorageUserProvider) ChangePassword(_, _, _ string) error {
	return ErrWeChatPasswordDisabled
}
func (p *WeChatStorageUserProvider) Close() error { return nil }
