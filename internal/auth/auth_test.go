package auth

import (
	"testing"
	"time"
)

func TestHashVerifyRoundTrip(t *testing.T) {
	h, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := VerifyPassword(h, "correct horse battery staple")
	if err != nil || !ok {
		t.Fatalf("verify good password: ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword(h, "wrong")
	if err != nil || ok {
		t.Fatalf("verify bad password: ok=%v err=%v", ok, err)
	}
}

func TestVerifyRejectsMalformed(t *testing.T) {
	for _, bad := range []string{"", "plain", "$argon2id$v=19$bad", "$bcrypt$x$y$z$w$v"} {
		if _, err := VerifyPassword(bad, "x"); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestHashIsSalted(t *testing.T) {
	a, _ := HashPassword("same")
	b, _ := HashPassword("same")
	if a == b {
		t.Fatal("two hashes of the same password are identical (salt not applied)")
	}
}

func TestSessionLifecycle(t *testing.T) {
	m := NewSessionManager()
	id, err := m.Create()
	if err != nil {
		t.Fatal(err)
	}
	if !m.Valid(id) {
		t.Fatal("fresh session should be valid")
	}
	if m.Valid("nope") {
		t.Fatal("unknown id should be invalid")
	}
	m.Destroy(id)
	if m.Valid(id) {
		t.Fatal("destroyed session should be invalid")
	}
}

func TestSessionIdleExpiry(t *testing.T) {
	m := NewSessionManager()
	now := time.Now()
	m.now = func() time.Time { return now }
	id, _ := m.Create()

	now = now.Add(SessionIdleTimeout + time.Minute)
	if m.Valid(id) {
		t.Fatal("session past idle timeout should be invalid")
	}
}

func TestSessionSlidingWindow(t *testing.T) {
	m := NewSessionManager()
	now := time.Now()
	m.now = func() time.Time { return now }
	id, _ := m.Create()

	for i := 0; i < 5; i++ {
		now = now.Add(SessionIdleTimeout - time.Minute)
		if !m.Valid(id) {
			t.Fatalf("iteration %d: session should still be valid after use", i)
		}
	}
}

func TestDestroyAll(t *testing.T) {
	m := NewSessionManager()
	m.Create()
	m.Create()
	m.DestroyAll()
	if m.Count() != 0 {
		t.Fatalf("Count = %d after DestroyAll", m.Count())
	}
}

func TestLoginLimiter(t *testing.T) {
	now := time.Now()
	l := NewLoginLimiter(3, time.Minute)
	l.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if !l.Allow("1.2.3.4") {
			t.Fatalf("attempt %d should be allowed", i)
		}
	}
	if l.Allow("1.2.3.4") {
		t.Fatal("4th attempt should be blocked")
	}
	if !l.Allow("5.6.7.8") {
		t.Fatal("other IP should not be affected")
	}

	now = now.Add(2 * time.Minute)
	if !l.Allow("1.2.3.4") {
		t.Fatal("attempt after window should be allowed again")
	}
}

func TestLoginLimiterReset(t *testing.T) {
	l := NewLoginLimiter(2, time.Minute)
	l.Allow("ip")
	l.Allow("ip")
	l.Reset("ip")
	if !l.Allow("ip") {
		t.Fatal("Reset should clear the counter")
	}
}
