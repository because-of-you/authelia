// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/authelia/authelia/v4/internal/authentication"
	"github.com/authelia/authelia/v4/internal/wechat"
	"github.com/rpadovani/sqlx-v2"
)

type wechatSQLStore struct{ provider *SQLProvider }

type wechatSQLConnection struct{ SQLXConnection }

func (c wechatSQLConnection) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return c.SQLXConnection.ExecContext(ctx, c.Rebind(q), args...)
}
func (c wechatSQLConnection) QueryRowContext(ctx context.Context, q string, args ...any) *sqlx.Row {
	return c.SQLXConnection.QueryRowxContext(ctx, c.Rebind(q), args...)
}
func (s *wechatSQLStore) conn(ctx context.Context) wechatSQLConnection {
	return wechatSQLConnection{s.provider.conn(ctx)}
}
func (p *SQLProvider) WeChatStore() wechat.Storage { return &wechatSQLStore{provider: p} }

func (s *wechatSQLStore) Profile(ctx context.Context, session wechat.MiniSession) (profile wechat.Profile, err error) {
	err = s.conn(ctx).QueryRowContext(ctx, `SELECT p.nickname, p.avatar FROM wechat_profiles p JOIN wechat_users u ON u.id = p.user_id JOIN wechat_identities i ON i.user_id = u.id WHERE u.username = ? AND i.appid = ? AND i.openid = ?`, session.Username, session.AppID, session.OpenID).Scan(&profile.Nickname, &profile.Avatar)
	if errors.Is(err, sql.ErrNoRows) {
		return wechat.Profile{}, wechat.ErrNotFound
	}
	return profile, err
}

func (s *wechatSQLStore) SaveProfile(ctx context.Context, session wechat.MiniSession, profile wechat.Profile) error {
	tx, err := s.provider.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	conn := wechatSQLConnection{tx}
	var userID int64
	err = conn.QueryRowContext(ctx, `SELECT u.id FROM wechat_users u JOIN wechat_identities i ON i.user_id = u.id WHERE u.username = ? AND i.appid = ? AND i.openid = ?`, session.Username, session.AppID, session.OpenID).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return wechat.ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, `DELETE FROM wechat_profiles WHERE user_id = ?`, userID); err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, `INSERT INTO wechat_profiles (user_id, nickname, avatar) VALUES (?, ?, ?)`, userID, profile.Nickname, profile.Avatar); err != nil {
		return err
	}
	return tx.Commit()
}
func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// RegisterIdentity creates an unprivileged account for a verified WeChat identity.
// Existing identities, including disabled accounts, are never re-enabled or replaced.
func (s *wechatSQLStore) RegisterIdentity(ctx context.Context, appID, openID string) error {
	if appID == "" || openID == "" {
		return wechat.ErrNotFound
	}
	tx, err := s.provider.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	conn := wechatSQLConnection{tx}
	var existing int
	err = conn.QueryRowContext(ctx, `SELECT 1 FROM wechat_identities WHERE appid = ? AND openid = ?`, appID, openID).Scan(&existing)
	if err == nil {
		return wechat.ErrNotFound
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return err
	}
	username := "wechat_" + hex.EncodeToString(random[:])
	if _, err = conn.ExecContext(ctx, `INSERT INTO wechat_users (username, display_name, enabled) VALUES (?, ?, ?)`, username, "微信用户", false); err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, `INSERT INTO wechat_identities (user_id, appid, openid) SELECT id, ?, ? FROM wechat_users WHERE username = ?`, appID, openID, username); err != nil {
		return err
	}
	return tx.Commit()
}

// LookupMiniUser resolves a verified WeChat identity regardless of its web access status.
func (s *wechatSQLStore) LookupMiniUser(ctx context.Context, appID, openID string) (*authentication.UserDetails, bool, error) {
	if appID == "" || openID == "" {
		return nil, false, authentication.ErrUserNotFound
	}
	conn := s.conn(ctx)
	query := `SELECT u.username, u.display_name, u.enabled, i.appid, i.openid, g.name FROM wechat_users u JOIN wechat_identities i ON i.user_id = u.id LEFT JOIN wechat_user_groups ug ON ug.user_id = u.id LEFT JOIN wechat_groups g ON g.id = ug.group_id WHERE i.appid = ? AND i.openid = ? ORDER BY g.name`
	rows, err := conn.QueryContext(ctx, conn.Rebind(query), appID, openID)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var details *authentication.UserDetails
	var enabled bool
	for rows.Next() {
		var username, displayName, storedAppID, storedOpenID string
		var rowEnabled bool
		var group sql.NullString
		if err = rows.Scan(&username, &displayName, &rowEnabled, &storedAppID, &storedOpenID, &group); err != nil {
			return nil, false, err
		}
		if username == "" || storedAppID != appID || storedOpenID != openID || (details != nil && details.Username != username) {
			return nil, false, authentication.ErrUserNotFound
		}
		if details == nil {
			details = &authentication.UserDetails{Username: username, DisplayName: displayName, Groups: []string{}}
			enabled = rowEnabled
		}
		if group.Valid && group.String != "" {
			details.Groups = append(details.Groups, group.String)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	if details == nil {
		return nil, false, authentication.ErrUserNotFound
	}
	return details, enabled, nil
}

// LookupWeChatUser resolves only an explicitly enabled account in the configured store.
func (p *SQLProvider) LookupWeChatUser(ctx context.Context, appID, openID, username string) (*authentication.UserDetails, error) {
	query := `SELECT u.username, u.display_name, u.email, g.name FROM wechat_users u `
	args := []any{}
	if username == "" {
		query += `JOIN wechat_identities i ON i.user_id = u.id `
	}
	query += `LEFT JOIN wechat_user_groups ug ON ug.user_id = u.id LEFT JOIN wechat_groups g ON g.id = ug.group_id WHERE `
	if username == "" {
		query += `i.appid = ? AND i.openid = ? AND u.enabled = ? ORDER BY g.name`
		args = append(args, appID, openID, true)
	} else {
		query += `u.username = ? AND u.enabled = ? ORDER BY g.name`
		args = append(args, username, true)
	}
	conn := p.conn(ctx)
	rows, err := conn.QueryContext(ctx, conn.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var details *authentication.UserDetails
	for rows.Next() {
		var user, displayName string
		var email, group sql.NullString
		if err = rows.Scan(&user, &displayName, &email, &group); err != nil {
			return nil, err
		}
		if user == "" || (details != nil && details.Username != user) {
			return nil, errors.New("invalid WeChat directory result")
		}
		if details == nil {
			details = &authentication.UserDetails{Username: user, DisplayName: displayName, Groups: []string{}}
			if email.Valid && email.String != "" {
				details.Emails = []string{email.String}
			}
		}
		if group.Valid && group.String != "" {
			details.Groups = append(details.Groups, group.String)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if details == nil {
		return nil, authentication.ErrUserNotFound
	}
	// Never accept a case-insensitive database match as the requested identity.
	if username != "" && details.Username != username {
		return nil, authentication.ErrUserNotFound
	}
	return details, nil
}

func (s *wechatSQLStore) CreateSession(ctx context.Context, token string, session wechat.MiniSession) error {
	_, err := s.conn(ctx).ExecContext(ctx, `INSERT INTO wechat_miniapp_sessions (token_hash, username, appid, openid, expires_at) VALUES (?, ?, ?, ?, ?)`, digest(token), session.Username, session.AppID, session.OpenID, session.ExpiresAt.Unix())
	return err
}

func (s *wechatSQLStore) Session(ctx context.Context, token string, now time.Time) (session wechat.MiniSession, err error) {
	var unix int64
	err = s.conn(ctx).QueryRowContext(ctx, `SELECT username, appid, openid, expires_at FROM wechat_miniapp_sessions WHERE token_hash = ? AND expires_at > ?`, digest(token), now.Unix()).Scan(&session.Username, &session.AppID, &session.OpenID, &unix)
	if errors.Is(err, sql.ErrNoRows) {
		return wechat.MiniSession{}, wechat.ErrNotFound
	}
	if err != nil {
		return wechat.MiniSession{}, err
	}
	session.ExpiresAt = time.Unix(unix, 0).UTC()
	return session, nil
}

func (s *wechatSQLStore) DeleteSession(ctx context.Context, token string) error {
	_, err := s.conn(ctx).ExecContext(ctx, `DELETE FROM wechat_miniapp_sessions WHERE token_hash = ?`, digest(token))
	return err
}

func (s *wechatSQLStore) CreateChallenge(ctx context.Context, transaction, binding string, c wechat.Challenge, image []byte) error {
	_, err := s.conn(ctx).ExecContext(ctx, `INSERT INTO wechat_qr_challenges (transaction_hash, browser_hash, target_url, target_domain, request_method, flow_id, flow, subflow, user_code, status, expires_at, qr_image) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)`, digest(transaction), digest(binding), c.TargetURL, c.TargetDomain, c.RequestMethod, c.FlowID, c.Flow, c.Subflow, c.UserCode, c.ExpiresAt.Unix(), image)
	return err
}

// Image returns the QR only to its original browser while it is pending.
func (s *wechatSQLStore) Image(ctx context.Context, transaction, binding string, now time.Time) (image []byte, err error) {
	err = s.conn(ctx).QueryRowContext(ctx, `SELECT qr_image FROM wechat_qr_challenges WHERE transaction_hash = ? AND browser_hash = ? AND status = 'pending' AND expires_at > ?`, digest(transaction), digest(binding), now.Unix()).Scan(&image)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, wechat.ErrNotFound
	}
	return image, err
}

func (s *wechatSQLStore) Challenge(ctx context.Context, transaction, binding string) (c wechat.Challenge, err error) {
	query := `SELECT target_url, target_domain, request_method, flow_id, flow, subflow, user_code, status, COALESCE(approved_username, ''), COALESCE(approved_appid, ''), COALESCE(approved_openid, ''), expires_at FROM wechat_qr_challenges WHERE transaction_hash = ?`
	args := []any{digest(transaction)}
	if binding != "" {
		query += ` AND browser_hash = ?`
		args = append(args, digest(binding))
	}
	var unix int64
	err = s.conn(ctx).QueryRowContext(ctx, query, args...).Scan(&c.TargetURL, &c.TargetDomain, &c.RequestMethod, &c.FlowID, &c.Flow, &c.Subflow, &c.UserCode, &c.Status, &c.Username, &c.AppID, &c.OpenID, &unix)
	if errors.Is(err, sql.ErrNoRows) {
		return wechat.Challenge{}, wechat.ErrNotFound
	}
	if err != nil {
		return wechat.Challenge{}, err
	}
	c.ExpiresAt = time.Unix(unix, 0).UTC()
	return c, nil
}

// MarkScanned marks an unexpired challenge without authenticating or approving it.
func (s *wechatSQLStore) MarkScanned(ctx context.Context, transaction string, now time.Time) error {
	result, err := s.conn(ctx).ExecContext(ctx, `UPDATE wechat_qr_challenges SET status = 'scanned' WHERE transaction_hash = ? AND status IN ('pending', 'scanned') AND expires_at > ?`, digest(transaction), now.Unix())
	return oneRow(result, err)
}

// Decide atomically approves or rejects a still-active unexpired challenge.
func (s *wechatSQLStore) Decide(ctx context.Context, transaction string, session wechat.MiniSession, status string, now time.Time) error {
	if status != "approved" && status != "rejected" {
		return wechat.ErrNotFound
	}
	query := `UPDATE wechat_qr_challenges SET status = ?, approved_username = ?, approved_appid = ?, approved_openid = ? WHERE transaction_hash = ? AND status IN ('pending', 'scanned') AND expires_at > ?`
	args := []any{status, session.Username, session.AppID, session.OpenID, digest(transaction), now.Unix()}
	if status == "approved" {
		query += ` AND EXISTS (SELECT 1 FROM wechat_users u JOIN wechat_identities i ON i.user_id = u.id WHERE u.username = ? AND u.enabled = ? AND i.appid = ? AND i.openid = ?)`
		args = append(args, session.Username, true, session.AppID, session.OpenID)
	}
	result, err := s.conn(ctx).ExecContext(ctx, query, args...)
	return oneRow(result, err)
}

// Consume atomically consumes an approved challenge bound to the requesting browser.
func (s *wechatSQLStore) Consume(ctx context.Context, transaction, binding string, now time.Time) error {
	result, err := s.conn(ctx).ExecContext(ctx, `UPDATE wechat_qr_challenges SET status = 'consumed' WHERE transaction_hash = ? AND browser_hash = ? AND status = 'approved' AND expires_at > ?`, digest(transaction), digest(binding), now.Unix())
	return oneRow(result, err)
}

func oneRow(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return wechat.ErrNotFound
	}
	return nil
}
