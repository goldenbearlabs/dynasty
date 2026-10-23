// Creates accounts before they belong to a league and verifies their email addresses.

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
	"crossover/internal/problem"
)

const verificationLifetime = 24 * time.Hour

func newVerificationToken() (string, [32]byte, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", [32]byte{}, err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), sha256.Sum256(raw[:]), nil
}

// Register creates an unverified account with no franchise. The caller emails the token.
func (s *Service) Register(ctx context.Context, email, password string) (string, error) {
	token, hash, err := newVerificationToken()
	if err != nil {
		return "", err
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		user, err := CreateUser(ctx, db.New(tx), email, password)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `insert into account_verifications (user_id, token_hash, expires_at) values ($1, $2, $3)`, user.ID, hash[:], time.Now().Add(verificationLifetime))
		return err
	})
	return token, err
}

// VerifyRegistration consumes a one-use link. Existing invited accounts have no row and remain verified.
func (s *Service) VerifyRegistration(ctx context.Context, token string) error {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return problem.New("That verification link is invalid or expired.")
	}
	hash := sha256.Sum256(raw)
	tag, err := s.pool.Exec(ctx, `update account_verifications set verified_at = now(), token_hash = null, expires_at = null where token_hash = $1 and verified_at is null and expires_at > now()`, hash[:])
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return problem.New("That verification link is invalid or expired.")
	}
	return nil
}

// IsVerified treats pre-existing invited accounts as verified without rewriting them.
func (s *Service) IsVerified(ctx context.Context, userID pgtype.UUID) (bool, error) {
	var verified bool
	err := s.pool.QueryRow(ctx, `select verified_at is not null from account_verifications where user_id = $1`, userID).Scan(&verified)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	return verified, err
}

// ResendVerification rotates an unverified account's link; unknown emails reveal nothing.
func (s *Service) ResendVerification(ctx context.Context, email string) (string, string, error) {
	user, err := db.New(s.pool).GetUserByEmail(ctx, normalize(email))
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	token, hash, err := newVerificationToken()
	if err != nil {
		return "", "", err
	}
	tag, err := s.pool.Exec(ctx, `update account_verifications set token_hash = $2, expires_at = $3 where user_id = $1 and verified_at is null`, user.ID, hash[:], time.Now().Add(verificationLifetime))
	if err != nil || tag.RowsAffected() == 0 {
		return "", "", err
	}
	return user.Email, token, nil
}

// AcceptInvite links a verified existing account to an unclaimed franchise.
func (s *Service) AcceptInvite(ctx context.Context, user db.User, invite string) error {
	verified, err := s.IsVerified(ctx, user.ID)
	if err != nil {
		return err
	}
	if !verified {
		return problem.New("Verify your email before joining a league.")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var dynastyID pgtype.UUID
		err := tx.QueryRow(ctx, `select dynasty_id from franchises where invite_token = $1 and user_id is null for update`, invite).Scan(&dynastyID)
		if errors.Is(err, pgx.ErrNoRows) {
			return problem.New("That invite link is not valid. Ask your commissioner for a new one.")
		}
		if err != nil {
			return err
		}
		var alreadyJoined bool
		if err := tx.QueryRow(ctx, `select exists(select 1 from franchises where user_id = $1 and dynasty_id = $2)`, user.ID, dynastyID).Scan(&alreadyJoined); err != nil {
			return err
		}
		if alreadyJoined {
			return problem.New("This account already manages a franchise in that league.")
		}
		_, err = tx.Exec(ctx, `update franchises set user_id = $1, invite_token = null where dynasty_id = $2 and invite_token = $3 and user_id is null`, user.ID, dynastyID, invite)
		return err
	})
}
