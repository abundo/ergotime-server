package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/abundo/ergotime-server/internal/config"
	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token expired")
)

// Claims is the JWT payload stored in the session cookie.
type Claims struct {
	UserID  int64  `json:"uid"`
	Name    string `json:"name"`
	IsAdmin bool   `json:"adm"`
	jwt.RegisteredClaims
}

// TokenService issues and validates JWTs.
type TokenService struct {
	secret []byte
	ttl    time.Duration
	issuer string
}

func NewTokenService(cfg *config.Config) *TokenService {
	return &TokenService{
		secret: []byte(cfg.Auth.JWTSecret),
		ttl:    cfg.Auth.TokenTTL(),
		issuer: "ergotime-server",
	}
}

func (s *TokenService) Issue(userID int64, name string, isAdmin bool) (string, time.Time, error) {
	exp := time.Now().UTC().Add(s.ttl)
	claims := Claims{
		UserID:  userID,
		Name:    name,
		IsAdmin: isAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			Issuer:    s.issuer,
			Subject:   fmt.Sprintf("%d", userID),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := t.SignedString(s.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, exp, nil
}

func (s *TokenService) Parse(tokenStr string) (*Claims, error) {
	t, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}
	claims, ok := t.Claims.(*Claims)
	if !ok || !t.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
