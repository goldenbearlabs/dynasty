// Manages credential changes and deletion while keeping league history separate from private account data.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"

	"crossover/internal/db"
	"crossover/internal/problem"
)

func checkCurrentPassword(user db.User, password string) error {
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return problem.New("Your current password is incorrect.")
	}
	return nil
}

func (s *Service) ChangePassword(ctx context.Context, userID pgtype.UUID, current, next string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var user db.User
		if err := tx.QueryRow(ctx, `select id, email, password_hash, sessions_valid_from, created_at from users where id = $1 for update`, userID).
			Scan(&user.ID, &user.Email, &user.PasswordHash, &user.SessionsValidFrom, &user.CreatedAt); err != nil {
			return err
		}
		if err := checkCurrentPassword(user, current); err != nil {
			return err
		}
		if current == next {
			return problem.New("Choose a different password.")
		}
		_, hash, err := credentials(user.Email, next)
		if err != nil {
			return err
		}
		if err := db.New(tx).SetUserCredentials(ctx, db.SetUserCredentialsParams{ID: userID, Email: user.Email, PasswordHash: hash}); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `delete from account_password_resets where user_id = $1`, userID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `delete from mobile_sessions where user_id = $1`, userID)
		return err
	})
}

// BeginEmailChange returns a short code only for delivery to the requested new address.
func (s *Service) BeginEmailChange(ctx context.Context, userID pgtype.UUID, password, address string) (string, string, error) {
	address = normalize(address)
	if _, _, err := credentials(address, "validation-only"); err != nil {
		return "", "", err
	}
	codeNumber, err := rand.Int(rand.Reader, big.NewInt(100000000))
	if err != nil {
		return "", "", err
	}
	code := fmt.Sprintf("%08d", codeNumber.Int64())
	hash := sha256.Sum256([]byte(code))
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var user db.User
		if err := tx.QueryRow(ctx, `select id, email, password_hash, sessions_valid_from, created_at from users where id = $1 for update`, userID).
			Scan(&user.ID, &user.Email, &user.PasswordHash, &user.SessionsValidFrom, &user.CreatedAt); err != nil {
			return err
		}
		if err := checkCurrentPassword(user, password); err != nil {
			return err
		}
		if address == user.Email {
			return problem.New("That is already your account email.")
		}
		var used bool
		if err := tx.QueryRow(ctx, `select exists(select 1 from users where email = $1)`, address).Scan(&used); err != nil {
			return err
		}
		if used {
			return problem.New("That email already has an account.")
		}
		tag, err := tx.Exec(ctx, `insert into account_email_changes (user_id, new_email, code_hash, expires_at)
			values ($1, $2, $3, now() + interval '15 minutes')
			on conflict (user_id) do update set new_email = excluded.new_email,
			code_hash = excluded.code_hash, expires_at = excluded.expires_at,
			attempts = 0, requested_at = now()
			where account_email_changes.requested_at < now() - interval '1 minute'`, userID, address, hash[:])
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return problem.New("Wait a minute before requesting another code.")
		}
		return nil
	})
	return address, code, err
}

func (s *Service) ConfirmEmailChange(ctx context.Context, userID pgtype.UUID, code string) error {
	var invalid bool
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var address string
		var hash []byte
		var expires time.Time
		var attempts int
		err := tx.QueryRow(ctx, `select new_email, code_hash, expires_at, attempts from account_email_changes where user_id = $1 for update`, userID).
			Scan(&address, &hash, &expires, &attempts)
		if errors.Is(err, pgx.ErrNoRows) {
			invalid = true
			return nil
		}
		if err != nil {
			return err
		}
		provided := sha256.Sum256([]byte(strings.TrimSpace(code)))
		if attempts >= 5 || !expires.After(time.Now()) || subtle.ConstantTimeCompare(hash, provided[:]) != 1 {
			invalid = true
			_, err = tx.Exec(ctx, `update account_email_changes set attempts = attempts + 1 where user_id = $1`, userID)
			return err
		}
		tag, err := tx.Exec(ctx, `update users set email = $2, sessions_valid_from = now() where id = $1`, userID, address)
		if err != nil {
			return emailTaken(err)
		}
		if tag.RowsAffected() != 1 {
			return pgx.ErrNoRows
		}
		if _, err = tx.Exec(ctx, `update account_verifications set verified_at = now(), token_hash = null, expires_at = null where user_id = $1`, userID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `delete from account_password_resets where user_id = $1`, userID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `delete from account_email_changes where user_id = $1`, userID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `delete from mobile_sessions where user_id = $1`, userID)
		return err
	})
	if err != nil {
		return err
	}
	if invalid {
		return problem.New("That code is invalid or expired. Request a new one if needed.")
	}
	return nil
}

type CommissionerTransfer struct {
	DynastyID   string              `json:"dynasty_id"`
	FranchiseID string              `json:"franchise_id"`
	Successors  []TransferCandidate `json:"successors"`
}

type TransferCandidate struct {
	FranchiseID string `json:"franchise_id"`
	Name        string `json:"name"`
}

func (s *Service) DeletionOptions(ctx context.Context, userID pgtype.UUID) ([]CommissionerTransfer, error) {
	rows, err := s.pool.Query(ctx, `select f.dynasty_id, f.id from franchises f where f.user_id = $1 and f.is_commissioner
		and not exists (select 1 from franchises other where other.dynasty_id = f.dynasty_id and other.is_commissioner and other.id <> f.id)`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	options := []CommissionerTransfer{}
	for rows.Next() {
		var dynastyID, franchiseID pgtype.UUID
		if err := rows.Scan(&dynastyID, &franchiseID); err != nil {
			return nil, err
		}
		options = append(options, CommissionerTransfer{DynastyID: dynastyID.String(), FranchiseID: franchiseID.String(), Successors: []TransferCandidate{}})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range options {
		rows, err := s.pool.Query(ctx, `select id, name from franchises where dynasty_id = $1 and user_id is not null and user_id <> $2 order by name`, options[i].DynastyID, userID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id pgtype.UUID
			var name string
			if err := rows.Scan(&id, &name); err != nil {
				rows.Close()
				return nil, err
			}
			options[i].Successors = append(options[i].Successors, TransferCandidate{FranchiseID: id.String(), Name: name})
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return options, nil
}

func (s *Service) DeleteAccount(ctx context.Context, userID pgtype.UUID, password string, transfers map[string]string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var user db.User
		if err := tx.QueryRow(ctx, `select id, email, password_hash, sessions_valid_from, created_at from users where id = $1 for update`, userID).
			Scan(&user.ID, &user.Email, &user.PasswordHash, &user.SessionsValidFrom, &user.CreatedAt); err != nil {
			return err
		}
		if err := checkCurrentPassword(user, password); err != nil {
			return err
		}
		// Serialize deletions inside each dynasty before locking franchise rows, so
		// two commissioners cannot both observe the other and then both leave.
		lockRows, err := tx.Query(ctx, `select distinct dynasty_id::text from franchises where user_id = $1 and is_commissioner order by 1`, userID)
		if err != nil {
			return err
		}
		lockIDs := []string{}
		for lockRows.Next() {
			var id string
			if err := lockRows.Scan(&id); err != nil {
				lockRows.Close()
				return err
			}
			lockIDs = append(lockIDs, id)
		}
		err = lockRows.Err()
		lockRows.Close()
		if err != nil {
			return err
		}
		for _, id := range lockIDs {
			if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock(hashtextextended($1, 0))`, id); err != nil {
				return err
			}
		}
		rows, err := tx.Query(ctx, `select id, dynasty_id, is_commissioner from franchises where user_id = $1 order by dynasty_id for update`, userID)
		if err != nil {
			return err
		}
		type owned struct {
			id, dynasty  pgtype.UUID
			commissioner bool
		}
		franchises := []owned{}
		for rows.Next() {
			var f owned
			if err := rows.Scan(&f.id, &f.dynasty, &f.commissioner); err != nil {
				rows.Close()
				return err
			}
			franchises = append(franchises, f)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, f := range franchises {
			var oldName string
			if err := tx.QueryRow(ctx, `select name from franchises where id = $1`, f.id).Scan(&oldName); err != nil {
				return err
			}
			// Notifications are disposable; remove copies of this manager's name
			// delivered to other inboxes without erasing historical transactions.
			if _, err := tx.Exec(ctx, `delete from notifications n using franchises recipient
				where n.franchise_id = recipient.id and recipient.dynasty_id = $1
				and left(n.title, length($2) + 1) = $2 || ' '`, f.dynasty, oldName); err != nil {
				return err
			}
			if f.commissioner {
				var others bool
				if err := tx.QueryRow(ctx, `select exists(select 1 from franchises where dynasty_id = $1 and is_commissioner and id <> $2)`, f.dynasty, f.id).Scan(&others); err != nil {
					return err
				}
				if !others {
					chosen := transfers[f.dynasty.String()]
					var successor pgtype.UUID
					if successor.Scan(chosen) != nil {
						return problem.New("Choose another manager to become commissioner before deleting your account.")
					}
					tag, err := tx.Exec(ctx, `update franchises set is_commissioner = true where id = $1 and dynasty_id = $2 and user_id is not null and user_id <> $3`, successor, f.dynasty, userID)
					if err != nil {
						return err
					}
					if tag.RowsAffected() != 1 {
						return problem.New("Choose an active manager in this league as commissioner.")
					}
				}
			}
			for _, table := range []string{"player_nicknames", "watchlist_entries", "rankings", "draft_queue", "draft_autopick", "trade_block", "trade_wants", "trade_targets", "notifications", "draft_pick_reactions"} {
				if _, err := tx.Exec(ctx, "delete from "+table+" where franchise_id = $1", f.id); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(ctx, `update waiver_claims set status = 'cancelled', reason = 'Account deleted', resolved_at = now() where franchise_id = $1 and status = 'pending'`, f.id); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `update trades set note = '' where proposed_by = $1`, f.id); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `delete from franchise_teams where franchise_id = $1`, f.id); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `update franchises set user_id = null, name = 'Former manager', manager_name = 'Former manager', image_url = '', invite_token = null,
				slug = 'former-' || replace(id::text, '-', ''), is_commissioner = false where id = $1`, f.id); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `delete from users where id = $1`, userID)
		return err
	})
}
