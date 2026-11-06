// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"github.com/valyala/fasthttp"
	"strconv"

	"github.com/authelia/authelia/v4/internal/middlewares"
)

// AuthenticationHistory returns native authentication attempts for the verified account.
func (h *WeChatHandlers) AuthenticationHistory(ctx *middlewares.AutheliaCtx) {
	auth, ok := h.authenticated(ctx)
	if !ok {
		return
	}
	var before int64
	if raw := string(ctx.QueryArgs().Peek("before")); raw != "" {
		var err error
		if before, err = strconv.ParseInt(raw, 10, 64); err != nil || before <= 0 {
			wechatError(ctx, fasthttp.StatusBadRequest, "分页参数无效")
			return
		}
	}
	page, err := h.store.AuthenticationHistory(ctx, auth.Session, before)
	if err != nil {
		wechatError(ctx, fasthttp.StatusServiceUnavailable, "认证记录暂不可用")
		return
	}
	if err = ctx.SetJSONBody(page); err != nil {
		ctx.Logger.WithError(err).Error("Failed to write authentication history")
	}
}
