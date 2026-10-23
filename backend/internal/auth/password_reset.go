// Issues one-use emailed password reset tokens without revealing whether an address exists.

package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"crossover/internal/db"
	"crossover/internal/problem"
)

const passwordResetLifetime = time.Hour

func (s *Service) RequestPasswordReset(ctx context.Context, email string) (string, string, error) {
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
	tag, err := s.pool.Exec(ctx, `insert into account_password_resets (user_id, token_hash, expires_at) values ($1, $2, $3)
		on conflict (user_id) do update set token_hash = excluded.token_hash, expires_at = excluded.expires_at, created_at = now()
		where account_password_resets.created_at < now() - interval '1 minute'`, user.ID, hash[:], time.Now().Add(passwordResetLifetime))
	if err != nil || tag.RowsAffected() == 0 {
		return "", "", err
	}
	return user.Email, token, nil
}

func (s *Service) CompletePasswordReset(ctx context.Context, token, password string) error {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return problem.New("That password reset link is invalid or expired.")
	}
	hash := sha256.Sum256(raw)
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var userID pgtype.UUID
		var email string
		err := tx.QueryRow(ctx, `select u.id, u.email from account_password_resets r join users u on u.id = r.user_id where r.token_hash = $1 and r.expires_at > now() for update of r, u`, hash[:]).Scan(&userID, &email)
		if errors.Is(err, pgx.ErrNoRows) {
			return problem.New("That password reset link is invalid or expired.")
		}
		if err != nil {
			return err
		}
		_, passwordHash, err := credentials(email, password)
		if err != nil {
			return err
		}
		if err := db.New(tx).SetUserCredentials(ctx, db.SetUserCredentialsParams{ID: userID, Email: email, PasswordHash: passwordHash}); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `delete from account_password_resets where user_id = $1`, userID)
		return err
	})
}
