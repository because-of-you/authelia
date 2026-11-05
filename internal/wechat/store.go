// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

package wechat

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound also covers expired, consumed, and invalid credentials.
var ErrNotFound = errors.New("wechat authorization not found or expired")

// Storage stores miniapp sessions and browser challenges using Authelia's configured storage.
type Storage interface {
	RegisterIdentity(ctx context.Context, appID, openID string) error
	Profile(ctx context.Context, session MiniSession) (Profile, error)
	SaveProfile(ctx context.Context, session MiniSession, profile Profile) error
	CreateSession(ctx context.Context, token string, session MiniSession) error
	Session(ctx context.Context, token string, now time.Time) (MiniSession, error)
	DeleteSession(ctx context.Context, token string) error
	CreateChallenge(ctx context.Context, transaction, binding string, c Challenge, image []byte) error
	Image(ctx context.Context, transaction, binding string, now time.Time) ([]byte, error)
	Challenge(ctx context.Context, transaction, binding string) (Challenge, error)
	Decide(ctx context.Context, transaction string, session MiniSession, status string, now time.Time) error
	Consume(ctx context.Context, transaction, binding string, now time.Time) error
}

// Profile contains user-provided display information, not verified identity attributes.
type Profile struct {
	Nickname string
	Avatar   []byte
}

type Challenge struct {
	TargetURL     string
	TargetDomain  string
	RequestMethod string
	FlowID        string
	Flow          string
	Subflow       string
	UserCode      string
	Status        string
	Username      string
	AppID         string
	OpenID        string
	ExpiresAt     time.Time
}

// MiniSession ties the bearer to the exact server-verified WeChat identity.
type MiniSession struct {
	Username  string
	AppID     string
	OpenID    string
	ExpiresAt time.Time
}
