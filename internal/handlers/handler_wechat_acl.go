// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"github.com/authelia/authelia/v4/internal/configuration/schema"
	"github.com/authelia/authelia/v4/internal/middlewares"
)

type wechatACLRule struct {
	Domains        []string                          `json:"domains"`
	DomainPatterns []string                          `json:"domain_patterns"`
	Policy         string                            `json:"policy"`
	Subjects       [][]string                        `json:"subjects"`
	Resources      []string                          `json:"resources"`
	Networks       []string                          `json:"networks"`
	Methods        []string                          `json:"methods"`
	QueryCount     int                               `json:"query_count"`
	Query          [][]schema.AccessControlRuleQuery `json:"query"`
}

// ACL returns the active configuration in matching order to an enabled admin.
func (h *WeChatHandlers) ACL(ctx *middlewares.AutheliaCtx) {
	if _, ok := h.groupAdministrator(ctx); !ok {
		return
	}
	access := ctx.Configuration.AccessControl
	rules := make([]wechatACLRule, 0, len(access.Rules))
	for _, rule := range access.Rules {
		item := wechatACLRule{
			Domains: append([]string{}, rule.Domains...), DomainPatterns: []string{}, Policy: rule.Policy,
			Subjects: rule.Subjects, Resources: []string{}, Networks: []string{},
			Methods: append([]string{}, rule.Methods...), QueryCount: len(rule.Query),
			Query: rule.Query,
		}
		for _, pattern := range rule.DomainsRegex {
			item.DomainPatterns = append(item.DomainPatterns, pattern.String())
		}
		for _, pattern := range rule.Resources {
			item.Resources = append(item.Resources, pattern.String())
		}
		for _, network := range rule.Networks {
			if network != nil {
				item.Networks = append(item.Networks, network.String())
			}
		}
		rules = append(rules, item)
	}
	if err := ctx.SetJSONBody(map[string]any{"default_policy": access.DefaultPolicy, "rules": rules}); err != nil {
		ctx.Logger.WithError(err).Error("Failed to write ACL response")
	}
}
