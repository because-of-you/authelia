// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/authelia/authelia/v4/internal/configuration/schema"
	"github.com/authelia/authelia/v4/internal/mocks"
	"github.com/authelia/authelia/v4/internal/wechat"
)

func TestWeChatACLRequiresEnabledAdminAndPreservesRuleOrder(t *testing.T) {
	for _, tc := range []struct {
		name    string
		enabled bool
		groups  []string
		want    int
	}{
		{"regular user", true, []string{"users"}, 403},
		{"disabled admin", false, []string{"admin"}, 403},
		{"enabled admin", true, []string{"admin"}, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := mocks.NewMockAutheliaCtx(t)
			defer mock.Close()
			mock.Ctx.Configuration.AccessControl = schema.AccessControl{DefaultPolicy: "deny", Rules: []schema.AccessControlRule{
				{Domains: []string{"first.example.com"}, Policy: "one_factor", Subjects: [][]string{{"group:engineering", "user:alice"}, {"group:admin"}}, Resources: []regexp.Regexp{*regexp.MustCompile(`^/api/reports([/?].*)?$`)}, Methods: []string{"GET"}, Query: [][]schema.AccessControlRuleQuery{{{Key: "format", Operator: "equal", Value: "json"}, {Key: "preview", Operator: "absent"}}, {{Key: "download", Operator: "present"}}}},
				{Domains: []string{"second.example.com"}, Policy: "deny", Resources: []regexp.Regexp{*regexp.MustCompile(`^/api/internal([/?].*)?$`)}},
			}}
			h := &WeChatHandlers{client: wechat.NewClient("wx0123456789abcdef", "secret", "develop"), store: &groupHandlerStore{enabled: tc.enabled, groups: tc.groups}}
			mock.Ctx.Request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 64))
			h.ACL(mock.Ctx)
			require.Equal(t, tc.want, mock.Ctx.Response.StatusCode())
			if tc.want != 200 {
				return
			}
			var response struct {
				Data struct {
					DefaultPolicy string          `json:"default_policy"`
					Rules         []wechatACLRule `json:"rules"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(mock.Ctx.Response.Body(), &response))
			require.Equal(t, "deny", response.Data.DefaultPolicy)
			require.Len(t, response.Data.Rules, 2)
			require.Equal(t, []string{"first.example.com"}, response.Data.Rules[0].Domains)
			require.Equal(t, [][]string{{"group:engineering", "user:alice"}, {"group:admin"}}, response.Data.Rules[0].Subjects)
			require.Equal(t, []string{`^/api/reports([/?].*)?$`}, response.Data.Rules[0].Resources)
			require.Equal(t, []string{"GET"}, response.Data.Rules[0].Methods)
			require.Equal(t, [][]schema.AccessControlRuleQuery{{{Key: "format", Operator: "equal", Value: "json"}, {Key: "preview", Operator: "absent"}}, {{Key: "download", Operator: "present"}}}, response.Data.Rules[0].Query)
			require.Equal(t, []string{"second.example.com"}, response.Data.Rules[1].Domains)
			require.Equal(t, "deny", response.Data.Rules[1].Policy)
			require.Equal(t, []string{`^/api/internal([/?].*)?$`}, response.Data.Rules[1].Resources)
		})
	}
}
