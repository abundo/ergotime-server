package auth

import (
	"testing"
	"time"

	"github.com/abundo/ergotime-server/internal/config"
)

func TestIssueAndParse(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.JWTSecret = "test-secret-key-for-jwt"
	cfg.Auth.TokenTTLHours = 1
	svc := NewTokenService(cfg)

	tok, exp, err := svc.Issue(42, "alice", true)
	if err != nil {
		t.Fatal(err)
	}
	if tok == "" {
		t.Fatal("empty token")
	}
	if time.Until(exp) < 50*time.Minute {
		t.Fatal("unexpected expiry")
	}

	claims, err := svc.Parse(tok)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != 42 || claims.Name != "alice" || !claims.IsAdmin {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestParseInvalid(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.JWTSecret = "test-secret-key-for-jwt"
	svc := NewTokenService(cfg)
	if _, err := svc.Parse("not.a.jwt"); err == nil {
		t.Fatal("expected error")
	}
}
