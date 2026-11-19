package auth

import (
	"testing"
	"time"
)

func TestLoginLimiterBlocksAfterFailures(t *testing.T) {
	limiter := newLoginLimiter(2, 15*time.Minute)
	keys := loginKeys("127.0.0.1", "alice")

	if !limiter.allow(keys...) {
		t.Fatal("first attempt should be allowed")
	}
	limiter.fail(keys...)
	limiter.fail(keys...)
	if limiter.allow(keys...) {
		t.Fatal("attempt after reaching the limit must be blocked")
	}
}

func TestLoginLimiterResetsOnSuccess(t *testing.T) {
	limiter := newLoginLimiter(1, 15*time.Minute)
	keys := loginKeys("127.0.0.1", "alice")

	limiter.fail(keys...)
	limiter.reset(keys...)
	if !limiter.allow(keys...) {
		t.Fatal("reset should clear the counter")
	}
}

func TestLoginLimiterSeparatesUsernames(t *testing.T) {
	limiter := newLoginLimiter(2, 15*time.Minute)
	alice := loginKeys("127.0.0.1", "alice")
	bob := loginKeys("127.0.0.1", "bob")

	limiter.fail(alice...)
	if !limiter.allow(bob...) {
		t.Fatal("one username must not block another before the IP limit is reached")
	}
}
