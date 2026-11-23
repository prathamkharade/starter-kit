package secure

import (
	"bytes"
	"testing"
)

func TestPasswordRoundTrip(t *testing.T) {
	h, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := VerifyPassword("correct horse battery staple", h); !ok {
		t.Fatal("expected match")
	}
	if ok, _ := VerifyPassword("wrong", h); ok {
		t.Fatal("expected mismatch")
	}
}

func TestKeyring(t *testing.T) {
	k, err := NewKeyring(bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32))
	if err != nil {
		t.Fatal(err)
	}
	ct, _ := k.Encrypt([]byte("+491701234567"))
	pt, err := k.Decrypt(ct)
	if err != nil || string(pt) != "+491701234567" {
		t.Fatalf("roundtrip failed: %v %q", err, pt)
	}
	ct[len(ct)-1] ^= 1
	if _, err := k.Decrypt(ct); err == nil {
		t.Fatal("tampering not detected")
	}
	if !Equal(k.Index("a", "b"), k.Index("a", "b")) || Equal(k.Index("ab", ""), k.Index("a", "b")) {
		t.Fatal("index not deterministic or ambiguous")
	}
}

func TestRandomDigits(t *testing.T) {
	c := RandomDigits(6)
	if len(c) != 6 {
		t.Fatalf("len=%d", len(c))
	}
	for _, r := range c {
		if r < '0' || r > '9' {
			t.Fatalf("non-digit %q", r)
		}
	}
}
