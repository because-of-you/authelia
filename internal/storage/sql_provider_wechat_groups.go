// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"
	"database/sql"
	"errors"

	"github.com/authelia/authelia/v4/internal/wechat"
)

// groupAdmin checks both the authenticated identity and its current enabled admin membership.
func groupAdmin(ctx context.Context, conn wechatSQLConnection, actor wechat.MiniSession) error {
	var username, appID, openID, group string
	err := conn.QueryRowContext(ctx, `SELECT u.username, i.appid, i.openid, g.name FROM wechat_users u JOIN wechat_identities i ON i.user_id = u.id JOIN wechat_user_groups ug ON ug.user_id = u.id JOIN wechat_groups g ON g.id = ug.group_id WHERE u.username = ? AND i.appid = ? AND i.openid = ? AND u.enabled = ? AND g.name = ?`, actor.Username, actor.AppID, actor.OpenID, true, "admin").Scan(&username, &appID, &openID, &group)
	if errors.Is(err, sql.ErrNoRows) {
		return wechat.ErrForbidden
	}
	if err != nil {
		return err
	}
	if username != actor.Username || appID != actor.AppID || openID != actor.OpenID || group != "admin" {
		return wechat.ErrForbidden
	}
	return nil
}

func (s *wechatSQLStore) ListGroupManagement(ctx context.Context, actor wechat.MiniSession) (result wechat.GroupManagement, err error) {
	result.Groups = []string{}
	result.Users = []wechat.GroupUser{}
	conn := s.conn(ctx)
	if err = groupAdmin(ctx, conn, actor); err != nil {
		return result, err
	}
	rows, err := conn.QueryContext(ctx, `SELECT name FROM wechat_groups ORDER BY name`)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			rows.Close()
			return result, err
		}
		result.Groups = append(result.Groups, name)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()
	rows, err = conn.QueryContext(ctx, `SELECT u.username, COALESCE(NULLIF(p.nickname, ''), u.display_name), g.name FROM wechat_users u LEFT JOIN wechat_profiles p ON p.user_id = u.id LEFT JOIN wechat_user_groups ug ON ug.user_id = u.id LEFT JOIN wechat_groups g ON g.id = ug.group_id ORDER BY u.username, g.name`)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var username, displayName string
		var group sql.NullString
		if err = rows.Scan(&username, &displayName, &group); err != nil {
			return result, err
		}
		if len(result.Users) == 0 || result.Users[len(result.Users)-1].Username != username {
			result.Users = append(result.Users, wechat.GroupUser{Username: username, DisplayName: displayName, Groups: []string{}})
		}
		if group.Valid {
			i := len(result.Users) - 1
			result.Users[i].Groups = append(result.Users[i].Groups, group.String)
		}
	}
	return result, rows.Err()
}

// ChangeGroup checks authorization again within the same transaction as the change.
func (s *wechatSQLStore) ChangeGroup(ctx context.Context, actor wechat.MiniSession, action, group, username string) error {
	tx, err := s.provider.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	conn := wechatSQLConnection{tx}
	if err = groupAdmin(ctx, conn, actor); err != nil {
		return err
	}
	var result sql.Result
	switch action {
	case "create":
		result, err = conn.ExecContext(ctx, `INSERT INTO wechat_groups (name) SELECT ? WHERE NOT EXISTS (SELECT 1 FROM wechat_groups WHERE name = ?)`, group, group)
	case "delete":
		if group == "admin" {
			return wechat.ErrForbidden
		}
		if _, err = conn.ExecContext(ctx, `DELETE FROM wechat_user_groups WHERE group_id IN (SELECT id FROM wechat_groups WHERE name = ?)`, group); err != nil {
			return err
		}
		result, err = conn.ExecContext(ctx, `DELETE FROM wechat_groups WHERE name = ?`, group)
	case "add":
		result, err = conn.ExecContext(ctx, `INSERT INTO wechat_user_groups (user_id, group_id) SELECT u.id, g.id FROM wechat_users u JOIN wechat_groups g ON g.name = ? WHERE u.username = ? AND NOT EXISTS (SELECT 1 FROM wechat_user_groups ug WHERE ug.user_id = u.id AND ug.group_id = g.id)`, group, username)
	case "remove":
		result, err = conn.ExecContext(ctx, `DELETE FROM wechat_user_groups WHERE user_id IN (SELECT id FROM wechat_users WHERE username = ?) AND group_id IN (SELECT id FROM wechat_groups WHERE name = ?)`, username, group)
	default:
		return wechat.ErrConflict
	}
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return wechat.ErrConflict
	}
	return tx.Commit()
}
