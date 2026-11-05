// SPDX-FileCopyrightText: 2026 Authelia
//
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"testing"
	"time"

	"github.com/authelia/authelia/v4/internal/authentication"
	"github.com/authelia/authelia/v4/internal/authorization"
)

func TestWeChatIsOneFactorAndHasItsOwnAMR(t *testing.T) {
	s := NewDefaultUserSession()
	s.SetOneFactorWeChat(time.Unix(1700000000, 0), &authentication.UserDetails{Username: "alice", Groups: []string{"staff"}})
	if got := s.AuthenticationLevel(false); got != authentication.OneFactor {
		t.Fatalf("expected one factor, got %v", got)
	}
	if s.AuthenticationMethodRefs.UsernameAndPassword || s.AuthenticationMethodRefs.KnowledgeBasedAuthentication || s.AuthenticationMethodRefs.TOTP || s.AuthenticationMethodRefs.WebAuthn {
		t.Fatal("wechat must not impersonate a different authentication method")
	}
	claims := s.AuthenticationMethodRefs.MarshalRFC8176()
	parsed := authorization.NewAuthenticationMethodsReferencesFromClaim(claims)
	if !parsed.WeChat || !parsed.FactorPossession() || parsed.MultiFactorAuthentication() {
		t.Fatalf("invalid AMR round trip: %+v", parsed)
	}
	if s.Username != "alice" || len(s.Groups) != 1 || s.Groups[0] != "staff" {
		t.Fatalf("missing user details: %+v", s)
	}
}
