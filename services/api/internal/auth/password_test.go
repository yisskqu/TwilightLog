package auth

import (
	"strings"
	"testing"
)

func TestPasswordHashAndVerify(t *testing.T) {
	encoded, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if strings.Contains(encoded, "correct horse") {
		t.Fatal("hash must not contain the plain password")
	}
	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=65536,t=3,p=2$") {
		t.Fatalf("unexpected hash format: %s", encoded)
	}
	if !VerifyPassword("correct horse battery staple", encoded) {
		t.Fatal("expected password to verify")
	}
	if VerifyPassword("wrong password", encoded) {
		t.Fatal("wrong password must not verify")
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	for _, encoded := range []string{"", "plain", "$argon2i$v=19$m=65536,t=3,p=2$abc$def", "$argon2id$v=19$bad$abc$def"} {
		if VerifyPassword("password", encoded) {
			t.Fatalf("malformed hash %q must not verify", encoded)
		}
	}
}

func TestIsStrongEnough(t *testing.T) {
	if IsStrongEnough("1234567") {
		t.Fatal("7 characters must be rejected")
	}
	if !IsStrongEnough("12345678") {
		t.Fatal("8 characters must be accepted")
	}
}
