// Package auth is accounts and sign-in: passwords, and the signed tokens
// that keep a manager signed in between visits.
//
// A manager gets an account by opening their franchise's invite link and
// choosing an email and password. Signing in returns a JWT, which the web
// layer keeps in a cookie.
package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"crossover/internal/db"
	"crossover/internal/problem"
)

// TokenLifetime is how long a sign-in lasts without a visit. The web layer
// renews the token on visits, so an active manager is never signed out.
const TokenLifetime = 30 * 24 * time.Hour

const minPasswordLength = 8

// errBadLogin is deliberately the same for an unknown email and a wrong
// password, so neither can be probed for.
var errBadLogin = problem.New("That email and password do not match an account.")

type Service struct {
	pool *pgxpool.Pool
	key  []byte // signs tokens
}

// NewService loads the token signing key, creating it on first start. The
// key lives in the database so tokens survive restarts with nothing to
// configure.
func NewService(ctx context.Context, pool *pgxpool.Pool) (*Service, error) {
	q := db.New(pool)
	fresh := make([]byte, 32)
	rand.Read(fresh)
	if err := q.InsertSecret(ctx, db.InsertSecretParams{Name: "jwt_key", Value: fresh}); err != nil {
		return nil, err
	}
	key, err := q.GetSecret(ctx, "jwt_key")
	if err != nil {
		return nil, err
	}
	return &Service{pool: pool, key: key}, nil
}

// CreateUser adds an account inside the caller's transaction.
func CreateUser(ctx context.Context, q *db.Queries, email, password string) (db.User, error) {
	email, hash, err := credentials(email, password)
	if err != nil {
		return db.User{}, err
	}
	user, err := q.CreateUser(ctx, db.CreateUserParams{Email: email, PasswordHash: hash})
	return user, emailTaken(err)
}

// Login checks an email and password.
func (s *Service) Login(ctx context.Context, email, password string) (db.User, error) {
	user, err := db.New(s.pool).GetUserByEmail(ctx, normalize(email))
	if errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, errBadLogin
	}
	if err != nil {
		return db.User{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return db.User{}, errBadLogin
	}
	return user, nil
}

// Claim uses a franchise's invite link to set the account that runs it:
// a new account the first time, or new credentials for the existing one
// when the commissioner has issued a fresh link as a reset. The link is
// used up, and any earlier sign-ins for the account stop working.
func (s *Service) Claim(ctx context.Context, invite, email, password string) (db.User, error) {
	var user db.User
	err := db.InTx(ctx, s.pool, func(q *db.Queries) error {
		franchise, err := q.GetFranchiseByInvite(ctx, invite)
		if errors.Is(err, pgx.ErrNoRows) {
			return problem.New("That invite link is not valid. Ask your commissioner for a new one.")
		}
		if err != nil {
			return err
		}

		if franchise.UserID.Valid {
			email, hash, err := credentials(email, password)
			if err != nil {
				return err
			}
			if err := q.SetUserCredentials(ctx, db.SetUserCredentialsParams{ID: franchise.UserID, Email: email, PasswordHash: hash}); err != nil {
				return emailTaken(err)
			}
			user, err = q.GetUser(ctx, franchise.UserID)
			if err != nil {
				return err
			}
		} else if user, err = CreateUser(ctx, q, email, password); err != nil {
			return err
		}
		return q.ClaimFranchise(ctx, db.ClaimFranchiseParams{ID: franchise.ID, UserID: user.ID})
	})
	return user, err
}

// Token issues a signed token for the user and reports when it expires.
func (s *Service) Token(user db.User) (string, time.Time) {
	now := time.Now()
	expires := now.Add(TokenLifetime)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   user.ID.String(),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(expires),
	})
	signed, _ := token.SignedString(s.key) // HS256 with a byte key cannot fail
	return signed, expires
}

// Session is who a valid token belongs to.
type Session struct {
	User      db.User
	Franchise db.Franchise
	Expires   time.Time
}

// ErrNoSession means the token is missing, invalid, expired or cancelled.
var ErrNoSession = errors.New("not signed in")

// Verify checks a token and loads the account and franchise behind it.
func (s *Service) Verify(ctx context.Context, signed string) (Session, error) {
	var claims jwt.RegisteredClaims
	_, err := jwt.ParseWithClaims(signed, &claims, func(*jwt.Token) (any, error) { return s.key, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil || claims.IssuedAt == nil {
		return Session{}, ErrNoSession
	}
	var userID pgtype.UUID
	if userID.Scan(claims.Subject) != nil {
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
	// Token times are whole seconds, so compare at that precision.
	if claims.IssuedAt.Time.Before(user.SessionsValidFrom.Time.Truncate(time.Second)) {
		return Session{}, ErrNoSession
	}
	franchise, err := q.GetFranchiseByUser(ctx, user.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNoSession
	}
	return Session{User: user, Franchise: franchise, Expires: claims.ExpiresAt.Time}, err
}

// credentials validates an email and password and returns the stored forms.
func credentials(email, password string) (string, string, error) {
	email = normalize(email)
	at := strings.Index(email, "@")
	if at < 1 || at == len(email)-1 || strings.ContainsAny(email, " \t") {
		return "", "", problem.New("Enter a valid email address.")
	}
	if len(password) < minPasswordLength {
		return "", "", problem.New("Choose a password of at least %d characters.", minPasswordLength)
	}
	if len(password) > 72 { // bcrypt reads no further
		return "", "", problem.New("Choose a password of at most 72 characters.")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return email, string(hash), err
}

func normalize(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// emailTaken turns a unique violation on users.email into a readable refusal.
func emailTaken(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return problem.New("That email already has an account.")
	}
	return err
}
