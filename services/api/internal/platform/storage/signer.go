package storage

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Signer produces and verifies short-lived capability tokens.
type Signer struct {
	secret []byte
}

func NewSigner(secret string) (*Signer, error) {
	if len(strings.TrimSpace(secret)) < 16 {
		return nil, fmt.Errorf("media signing secret must be at least 16 characters")
	}
	return &Signer{secret: []byte(secret)}, nil
}

// Sign returns "<expiresUnix>.<base64url hmac>" for the scope.
func (s *Signer) Sign(scope string, expires time.Time) string {
	exp := strconv.FormatInt(expires.Unix(), 10)
	return exp + "." + s.mac(scope, exp)
}

// Verify checks that the token is authentic and not expired.
func (s *Signer) Verify(scope, token string, now time.Time) error {
	expPart, sig, ok := strings.Cut(strings.TrimSpace(token), ".")
	if !ok || expPart == "" || sig == "" {
		return ErrInvalidSignature
	}
	expected := s.mac(scope, expPart)
	if subtle.ConstantTimeCompare([]byte(sig), []byte(expected)) != 1 {
		return ErrInvalidSignature
	}
	exp, err := strconv.ParseInt(expPart, 10, 64)
	if err != nil {
		return ErrInvalidSignature
	}
	if now.After(time.Unix(exp, 0)) {
		return ErrInvalidSignature
	}
	return nil
}

func (s *Signer) mac(scope, exp string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(scope))
	mac.Write([]byte{'\n'})
	mac.Write([]byte(exp))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func randomHex(bytes int) string {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buf)
}
