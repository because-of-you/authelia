// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

package wechat

import (
	"context"
	"errors"
	"time"

	"github.com/authelia/authelia/v4/internal/authentication"
)

// ErrNotFound also covers expired, consumed, and invalid credentials.
var ErrNotFound = errors.New("wechat authorization not found or expired")

// ErrForbidden indicates that the current account cannot manage groups.
var ErrForbidden = errors.New("wechat group management forbidden")

// ErrConflict indicates that the requested group change cannot be applied.
var ErrConflict = errors.New("wechat group management conflict")

// Storage stores miniapp sessions and browser challenges using Authelia's configured storage.
type Storage interface {
	RegisterIdentity(ctx context.Context, appID, openID string) error
	LookupMiniUser(ctx context.Context, appID, openID string) (*authentication.UserDetails, bool, error)
	ListGroupManagement(ctx context.Context, actor MiniSession) (GroupManagement, error)
	ChangeGroup(ctx context.Context, actor MiniSession, action, group, username string) error
	Profile(ctx context.Context, session MiniSession) (Profile, error)
	SaveProfile(ctx context.Context, session MiniSession, profile Profile) error
	AuthenticationHistory(ctx context.Context, session MiniSession, before int64) (AuthenticationPage, error)
	CreateSession(ctx context.Context, token string, session MiniSession) error
	Session(ctx context.Context, token string, now time.Time) (MiniSession, error)
	DeleteSession(ctx context.Context, token string) error
	CreateChallenge(ctx context.Context, transaction, binding string, c Challenge, image []byte) error
	Image(ctx context.Context, transaction, binding string, now time.Time) ([]byte, error)
	Challenge(ctx context.Context, transaction, binding string) (Challenge, error)
	MarkScanned(ctx context.Context, transaction string, now time.Time) error
	Decide(ctx context.Context, transaction string, session MiniSession, status string, now time.Time) error
	Consume(ctx context.Context, transaction, binding string, now time.Time) error
}

// GroupManagement contains the directory visible to a group administrator.
type GroupManagement struct {
	Groups []string    `json:"groups"`
	Users  []GroupUser `json:"users"`
}

// GroupUser identifies a directory account and its current group assignments.
type GroupUser struct {
	Username    string   `json:"username"`
	DisplayName string   `json:"display_name"`
	Groups      []string `json:"groups"`
}

// Profile contains user-provided display information, not verified identity attributes.
type Profile struct {
	Nickname string
	Avatar   []byte
	Email    *string
}

// AuthenticationRecord is a limited view of an account's native authentication attempts.
type AuthenticationRecord struct {
	ID            int64     `json:"-"`
	Time          time.Time `json:"time"`
	Successful    bool      `json:"successful"`
	Banned        bool      `json:"banned"`
	Type          string    `json:"type"`
	RemoteIP      string    `json:"remote_ip"`
	RequestURI    string    `json:"request_uri"`
	RequestMethod string    `json:"request_method"`
}

// AuthenticationPage holds a bounded page and the cursor for older attempts.
type AuthenticationPage struct {
	Records    []AuthenticationRecord `json:"records"`
	NextCursor int64                  `json:"next_cursor"`
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
