// Issues and verifies revocable mobile bearer tokens without changing browser JWT cookies.

package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/db"
)

const MobileTokenLifetime = 30 * 24 * time.Hour

func mobileHash(token string) ([32]byte, error) {
	var empty [32]byte
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != 32 {
		return empty, ErrNoSession
	}
	return sha256.Sum256(decoded), nil
}

// IssueMobile creates an opaque token; only its hash is stored in PostgreSQL.
func (s *Service) IssueMobile(ctx context.Context, user db.User) (string, time.Time, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(bytes)
	hash := sha256.Sum256(bytes)
	expires := time.Now().Add(MobileTokenLifetime)
	_, err := s.pool.Exec(ctx, `insert into mobile_sessions (token_hash, user_id, expires_at) values ($1, $2, $3)`, hash[:], user.ID, expires)
	return token, expires, err
}

// VerifyMobile checks expiry, credential-reset time, and the linked franchise.
func (s *Service) VerifyMobile(ctx context.Context, token string) (Session, error) {
	hash, err := mobileHash(token)
	if err != nil {
		return Session{}, err
	}
	var userID pgtype.UUID
	var created, expires time.Time
	err = s.pool.QueryRow(ctx, `select user_id, created_at, expires_at from mobile_sessions where token_hash = $1`, hash[:]).Scan(&userID, &created, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNoSession
	}
	if err != nil {
		return Session{}, err
	}
	if !expires.After(time.Now()) {
		return Session{}, ErrNoSession
	}
	q := db.New(s.pool)
	user, err := q.GetUser(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNoSession
	}
	if err != nil {
		return Session{}, err
	}
	if created.Before(user.SessionsValidFrom.Time) {
		return Session{}, ErrNoSession
	}
	franchise, err := q.GetFranchiseByUser(ctx, user.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{User: user, Expires: expires}, nil
	}
	return Session{User: user, Franchise: franchise, Expires: expires}, err
}

// RevokeMobile ends one device session without signing out the website.
func (s *Service) RevokeMobile(ctx context.Context, token string) error {
	hash, err := mobileHash(token)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `delete from mobile_sessions where token_hash = $1`, hash[:])
	return err
}

// RefreshMobile replaces a verified token with a fresh one.
func (s *Service) RefreshMobile(ctx context.Context, token string) (string, time.Time, Session, error) {
	session, err := s.VerifyMobile(ctx, token)
	if err != nil {
		return "", time.Time{}, Session{}, err
	}
	fresh, expires, err := s.IssueMobile(ctx, session.User)
	if err != nil {
		return "", time.Time{}, Session{}, err
	}
	if err := s.RevokeMobile(ctx, token); err != nil {
		_ = s.RevokeMobile(ctx, fresh)
		return "", time.Time{}, Session{}, err
	}
	session.Expires = expires
	return fresh, expires, session, nil
}
