// Package secure holds every cryptographic primitive the app uses.
// Do not hand-roll crypto elsewhere — add it here, with a test.
package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"golang.org/x/crypto/argon2"
)

// ---- passwords (argon2id, OWASP 2024+ baseline: m=19MiB, t=2, p=1) --------

type ArgonParams struct {
	Memory  uint32 // KiB
	Time    uint32
	Threads uint8
	KeyLen  uint32
	SaltLen int
}

var DefaultArgon = ArgonParams{Memory: 19 * 1024, Time: 2, Threads: 1, KeyLen: 32, SaltLen: 16}

func HashPassword(pw string) (string, error) {
	p := DefaultArgon
	salt := make([]byte, p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(pw), salt, p.Time, p.Memory, p.Threads, p.KeyLen)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.Memory, p.Time, p.Threads, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

func VerifyPassword(pw, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("unsupported hash format")
	}
	var p ArgonParams
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Time, &p.Threads); err != nil {
		return false, err
	}
	enc := base64.RawStdEncoding
	salt, err := enc.DecodeString(parts[4])
	if err != nil {
		return false, err
	}
	want, err := enc.DecodeString(parts[5])
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(pw), salt, p.Time, p.Memory, p.Threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// DummyVerify burns the same CPU as a real verification. Call it when a user
// is not found so response timing does not reveal which accounts exist.
func DummyVerify(pw string) {
	_, _ = VerifyPassword(pw, dummyHash)
}

var dummyHash, _ = HashPassword("timing-equalizer")

// ---- random values ----------------------------------------------------------

// RandomToken returns n random bytes, base64url encoded (no padding).
func RandomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failing is unrecoverable
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// RandomDigits returns a uniformly random numeric code (for OTPs).
func RandomDigits(n int) string {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		d, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			panic(err)
		}
		sb.WriteByte(byte('0' + d.Int64()))
	}
	return sb.String()
}

func SHA256(b []byte) []byte { s := sha256.Sum256(b); return s[:] }

// ---- field encryption + blind indexes ---------------------------------------

// Keyring encrypts personal data at the field level (AES-256-GCM) and computes
// HMAC "blind indexes" so encrypted values can still be looked up by equality.
// Ciphertext format: version(1 byte) || nonce(12) || ciphertext+tag.
// Rotation: add a new version byte + key, keep old keys for Decrypt, re-encrypt lazily.
type Keyring struct {
	aead   cipher.AEAD
	idxKey []byte
}

const keyVersion byte = 1

func NewKeyring(encKey, idxKey []byte) (*Keyring, error) {
	if len(encKey) != 32 || len(idxKey) != 32 {
		return nil, errors.New("keyring: keys must be 32 bytes")
	}
	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Keyring{aead: aead, idxKey: idxKey}, nil
}

func (k *Keyring) Encrypt(plain []byte) ([]byte, error) {
	nonce := make([]byte, k.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := append([]byte{keyVersion}, nonce...)
	return k.aead.Seal(out, nonce, plain, []byte{keyVersion}), nil
}

func (k *Keyring) Decrypt(b []byte) ([]byte, error) {
	ns := k.aead.NonceSize()
	if len(b) < 1+ns || b[0] != keyVersion {
		return nil, errors.New("keyring: bad ciphertext")
	}
	return k.aead.Open(nil, b[1:1+ns], b[1+ns:], []byte{keyVersion})
}

// Index returns HMAC-SHA256 over the parts (length-prefixed to avoid ambiguity).
func (k *Keyring) Index(parts ...string) []byte {
	m := hmac.New(sha256.New, k.idxKey)
	for _, p := range parts {
		fmt.Fprintf(m, "%d:%s|", len(p), p)
	}
	return m.Sum(nil)
}

// Equal is a constant-time comparison.
func Equal(a, b []byte) bool { return hmac.Equal(a, b) }
