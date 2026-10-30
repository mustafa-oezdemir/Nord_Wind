package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthenticated    = errors.New("unauthenticated")
	ErrRateLimited        = errors.New("rate limited")
)

type Service struct {
	repository *Repository
	dummyHash  string
}

func NewService(repository *Repository) (*Service, error) {
	dummyHash, err := HashPassword("nordwinds-dummy-password")
	if err != nil {
		return nil, err
	}
	return &Service{repository: repository, dummyHash: dummyHash}, nil
}

func NormalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if len(email) < 3 || len(email) > 254 || !utf8.ValidString(email) || strings.ContainsAny(email, "\x00\r\n") {
		return "", ErrInvalidInput
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || !strings.Contains(email, "@") {
		return "", ErrInvalidInput
	}
	return email, nil
}

func (s *Service) Register(ctx context.Context, rawEmail, password string) (Customer, error) {
	email, err := NormalizeEmail(rawEmail)
	if err != nil {
		return Customer{}, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return Customer{}, err
	}
	return s.repository.CreateCustomer(ctx, email, hash)
}

func (s *Service) Login(ctx context.Context, rawEmail, password, source, oldToken string) (Customer, string, Session, error) {
	email, err := NormalizeEmail(rawEmail)
	if err != nil || len(password) == 0 || len(password) > 1024 {
		return Customer{}, "", Session{}, ErrInvalidCredentials
	}
	now := time.Now().UTC()
	if err := s.repository.PruneLoginAttempts(ctx, now); err != nil {
		return Customer{}, "", Session{}, err
	}
	accountKey := sha256.Sum256([]byte("account:" + email))
	sourceKey := sha256.Sum256([]byte("source:" + source))
	accountAttempts, err := s.repository.CountLoginAttempt(ctx, accountKey[:], now)
	if err != nil {
		return Customer{}, "", Session{}, err
	}
	sourceAttempts, err := s.repository.CountLoginAttempt(ctx, sourceKey[:], now)
	if err != nil {
		return Customer{}, "", Session{}, err
	}
	if accountAttempts > 5 || sourceAttempts > 100 {
		return Customer{}, "", Session{}, ErrRateLimited
	}
	customer, storedHash, err := s.repository.CustomerByEmail(ctx, email)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Customer{}, "", Session{}, err
	}
	hasPassword := storedHash != ""
	if errors.Is(err, ErrNotFound) || !hasPassword {
		storedHash = s.dummyHash
	}
	valid := VerifyPassword(storedHash, password)
	if !valid || errors.Is(err, ErrNotFound) || !hasPassword {
		return Customer{}, "", Session{}, ErrInvalidCredentials
	}
	token, err := randomToken()
	if err != nil {
		return Customer{}, "", Session{}, err
	}
	csrf, err := randomToken()
	if err != nil {
		return Customer{}, "", Session{}, err
	}
	newHash := TokenHash(token)
	oldHash := TokenHash(oldToken)
	if err := s.repository.ReplaceSession(ctx, oldHash, newHash, customer, csrf, now); err != nil {
		return Customer{}, "", Session{}, err
	}
	_ = s.repository.ClearLoginAttempts(ctx, accountKey[:])
	return customer, token, Session{Customer: customer, CSRFToken: csrf, ExpiresAt: now.Add(12 * time.Hour)}, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (Session, error) {
	hash := TokenHash(token)
	if len(hash) != 32 {
		return Session{}, ErrUnauthenticated
	}
	session, err := s.repository.ActiveSession(ctx, hash, time.Now().UTC())
	if errors.Is(err, ErrNotFound) {
		return Session{}, ErrUnauthenticated
	}
	return session, err
}

func (s *Service) Logout(ctx context.Context, token string) error {
	hash := TokenHash(token)
	if len(hash) != 32 {
		return ErrUnauthenticated
	}
	return s.repository.DeleteSession(ctx, hash)
}

func TokenHash(token string) []byte {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 || base64.RawURLEncoding.EncodeToString(raw) != token {
		return nil
	}
	hash := sha256.Sum256(raw)
	return hash[:]
}

func randomToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
