// Package token issues and verifies short-lived JWT access tokens.
// Refresh tokens are opaque random strings stored hashed in `sessions` (see auth).
package token

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Claims struct {
	jwt.RegisteredClaims
	SID  string `json:"sid"`
	Role string `json:"role"`
}

type Issuer struct {
	secret []byte
	issuer string
	ttl    time.Duration
}

func NewIssuer(secret []byte, issuer string, ttl time.Duration) *Issuer {
	return &Issuer{secret: secret, issuer: issuer, ttl: ttl}
}

func (i *Issuer) Issue(userID, sessionID uuid.UUID, role string) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(i.ttl)
	c := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			Issuer:    i.issuer,
			Audience:  jwt.ClaimStrings{i.issuer},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-30 * time.Second)),
			ExpiresAt: jwt.NewNumericDate(exp),
			ID:        uuid.NewString(),
		},
		SID:  sessionID.String(),
		Role: role,
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(i.secret)
	return s, exp, err
}

// Parse validates signature, algorithm (pinned to HS256 — blocks alg=none / key confusion),
// issuer, audience and expiry.
func (i *Issuer) Parse(raw string) (*Claims, error) {
	c := &Claims{}
	_, err := jwt.ParseWithClaims(raw, c, func(*jwt.Token) (any, error) { return i.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(i.issuer),
		jwt.WithAudience(i.issuer),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(30*time.Second),
	)
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(c.Subject); err != nil {
		return nil, errors.New("bad subject")
	}
	return c, nil
}
