// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

package authentication

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type testWeChatDirectory struct {
	err                     error
	appID, openID, username string
}

func (d *testWeChatDirectory) LookupWeChatUser(_ context.Context, appID, openID, username string) (*UserDetails, error) {
	d.appID, d.openID, d.username = appID, openID, username
	if d.err != nil {
		return nil, d.err
	}
	return &UserDetails{Username: "Alice"}, nil
}

func TestWeChatStorageUserProvider(t *testing.T) {
	directory := &testWeChatDirectory{}
	p := NewWeChatStorageUserProvider(directory)
	require.NoError(t, p.StartupCheck())
	details, err := p.ResolveWeChatUser("wx-app", "openid")
	require.NoError(t, err)
	require.Equal(t, "Alice", details.Username)
	require.Equal(t, "wx-app", directory.appID)
	require.Equal(t, "openid", directory.openID)
	_, err = p.GetDetails("Alice")
	require.NoError(t, err)
	require.Equal(t, "Alice", directory.username)
	directory.err = errors.New("unavailable")
	_, err = p.ResolveWeChatUser("wx-app", "openid")
	require.Error(t, err)
	valid, err := p.CheckUserPassword("Alice", "password")
	require.NoError(t, err)
	require.False(t, valid)
	require.ErrorIs(t, p.UpdatePassword("Alice", "new"), ErrWeChatPasswordDisabled)
	require.ErrorIs(t, p.ChangePassword("Alice", "old", "new"), ErrWeChatPasswordDisabled)
	require.NoError(t, p.Close())
}
