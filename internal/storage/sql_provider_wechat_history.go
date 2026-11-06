// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"
	"net/url"

	"github.com/authelia/authelia/v4/internal/model"
	"github.com/authelia/authelia/v4/internal/wechat"
)

// AuthenticationHistory exposes a page of the verified account's attempts in insertion order.
func (s *wechatSQLStore) AuthenticationHistory(ctx context.Context, session wechat.MiniSession, before int64) (wechat.AuthenticationPage, error) {
	conn := s.conn(ctx)
	page := wechat.AuthenticationPage{Records: []wechat.AuthenticationRecord{}}
	query := `SELECT l.id, l.time, l.successful, l.banned, l.auth_type, l.remote_ip, COALESCE(l.request_uri, ''), l.request_method FROM authentication_logs l WHERE l.username = ? AND EXISTS (SELECT 1 FROM wechat_users u JOIN wechat_identities i ON i.user_id = u.id WHERE u.username = l.username AND i.appid = ? AND i.openid = ?)`
	args := []any{session.Username, session.AppID, session.OpenID}
	if before > 0 {
		query += ` AND l.id < ?`
		args = append(args, before)
	}
	query += ` ORDER BY l.id DESC LIMIT 11`
	rows, err := conn.QueryContext(ctx, conn.Rebind(query), args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var record wechat.AuthenticationRecord
		var ip model.NullIP
		var requestURI string
		if err = rows.Scan(&record.ID, &record.Time, &record.Successful, &record.Banned, &record.Type, &ip, &requestURI, &record.RequestMethod); err != nil {
			return page, err
		}
		if ip.IP != nil {
			record.RemoteIP = ip.String()
		}
		if parsed, parseErr := url.Parse(requestURI); parseErr == nil {
			parsed.User, parsed.RawQuery, parsed.Fragment, parsed.ForceQuery = nil, "", "", false
			record.RequestURI = parsed.String()
		}
		if len(page.Records) == 10 {
			page.NextCursor = page.Records[9].ID
			break
		}
		page.Records = append(page.Records, record)
	}
	return page, rows.Err()
}
