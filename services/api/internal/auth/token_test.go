package auth

import (
	"testing"
	"time"
)

const testSecret = "test_secret_with_at_least_32_characters_long"

func TestAccessTokenRoundTrip(t *testing.T) {
	raw, err := newAccessToken(testSecret, "user-1", "session-1", "editor", "alice", time.Minute)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	claims, err := parseAccessToken(testSecret, raw)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if claims.Subject != "user-1" || claims.SessionID != "session-1" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if claims.Role != "editor" || claims.Username != "alice" {
		t.Fatalf("unexpected custom claims: %+v", claims)
	}
}

func TestAccessTokenRejectsWrongSecret(t *testing.T) {
	raw, err := newAccessToken(testSecret, "user-1", "session-1", "admin", "alice", time.Minute)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	if _, err := parseAccessToken("another_secret_with_at_least_32_characters", raw); err == nil {
		t.Fatal("token signed with another secret must be rejected")
	}
}

func TestAccessTokenRejectsExpired(t *testing.T) {
	raw, err := newAccessToken(testSecret, "user-1", "session-1", "admin", "alice", -time.Minute)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	if _, err := parseAccessToken(testSecret, raw); err == nil {
		t.Fatal("expired token must be rejected")
	}
}

func TestRefreshTokenIsRandomAndHashed(t *testing.T) {
	first, err := newRefreshToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	second, err := newRefreshToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if first == second {
		t.Fatal("refresh tokens must be unique")
	}
	if hashRefreshToken(first) == first {
		t.Fatal("refresh token must be stored hashed")
	}
}
