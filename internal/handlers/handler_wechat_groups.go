// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/valyala/fasthttp"

	"github.com/authelia/authelia/v4/internal/middlewares"
	"github.com/authelia/authelia/v4/internal/wechat"
)

func (h *WeChatHandlers) groupAdministrator(ctx *middlewares.AutheliaCtx) (*wechatAuthenticatedSession, bool) {
	auth, ok := h.authenticated(ctx)
	if !ok {
		return nil, false
	}
	if auth.Enabled {
		for _, group := range auth.Details.Groups {
			if group == "admin" {
				return auth, true
			}
		}
	}
	wechatError(ctx, fasthttp.StatusForbidden, "仅启用的管理员可以访问此功能")
	return nil, false
}

// Groups lists groups and user memberships for administrators only.
func (h *WeChatHandlers) Groups(ctx *middlewares.AutheliaCtx) {
	auth, ok := h.groupAdministrator(ctx)
	if !ok {
		return
	}
	result, err := h.store.ListGroupManagement(ctx, auth.Session)
	if err != nil {
		if errors.Is(err, wechat.ErrForbidden) {
			wechatError(ctx, fasthttp.StatusForbidden, "没有用户组管理权限")
		} else {
			wechatError(ctx, fasthttp.StatusServiceUnavailable, "无法获取用户组")
		}
		return
	}
	if err = ctx.SetJSONBody(result); err != nil {
		ctx.Logger.WithError(err).Error("Failed to write group management response")
	}
}

func validWeChatGroupName(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || utf8.RuneCountInString(value) > 64 {
		return false
	}
	for _, r := range value {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' && r != '.' {
			return false
		}
	}
	return true
}

// ChangeGroups creates and deletes groups or modifies a user's membership.
func (h *WeChatHandlers) ChangeGroups(ctx *middlewares.AutheliaCtx) {
	auth, ok := h.groupAdministrator(ctx)
	if !ok {
		return
	}
	var body struct {
		Action   string `json:"action"`
		Group    string `json:"group"`
		Username string `json:"username"`
	}
	if !wechatBody(ctx, &body) || !validWeChatGroupName(body.Group) || len(body.Username) > 191 || ((body.Action == "add" || body.Action == "remove") && body.Username == "") || ((body.Action == "create" || body.Action == "delete") && body.Username != "") || (body.Action != "create" && body.Action != "delete" && body.Action != "add" && body.Action != "remove") {
		wechatError(ctx, fasthttp.StatusBadRequest, "用户组请求无效")
		return
	}
	if err := h.store.ChangeGroup(ctx, auth.Session, body.Action, body.Group, body.Username); err != nil {
		switch {
		case errors.Is(err, wechat.ErrForbidden):
			wechatError(ctx, fasthttp.StatusForbidden, "没有权限执行此操作")
		case errors.Is(err, wechat.ErrConflict):
			wechatError(ctx, fasthttp.StatusConflict, "用户组或成员状态已变化，请刷新重试")
		default:
			wechatError(ctx, fasthttp.StatusServiceUnavailable, "用户组更新失败")
		}
		return
	}
	ctx.ReplyOK()
}
