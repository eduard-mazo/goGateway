package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionRevoked  = errors.New("session revoked")
	ErrSessionExpired  = errors.New("session expired")
)

// SessionRow mirrors one row in the sessions table.
type SessionRow struct {
	ID               string    `db:"id"`
	UserID           int64     `db:"user_id"`
	RefreshTokenHash string    `db:"refresh_token_hash"`
	UserAgent        string    `db:"user_agent"`
	RemoteIP         string    `db:"remote_ip"`
	ExpiresAt        time.Time `db:"expires_at"`
	Revoked          bool      `db:"revoked"`
	CreatedAt        time.Time `db:"created_at"`
}

// SessionStore manages sessions backed by SQLite.
// Each login event creates one session row. The session ID is embedded in the
// JWT jti claim, allowing the AuthMiddleware to revoke individual sessions
// without invalidating other active logins for the same user.
type SessionStore struct {
	db *sqlx.DB
}

func NewSessionStore(db *sqlx.DB) *SessionStore { return &SessionStore{db: db} }

// Create inserts a new session and returns the session ID plus the raw
// (un-hashed) refresh token.  The caller must send both back to the client;
// the DB stores only the bcrypt hash of the refresh token.
func (s *SessionStore) Create(
	ctx context.Context,
	userID int64,
	userAgent, remoteIP string,
	duration time.Duration,
) (sessionID, rawRefreshToken string, err error) {
	sessionID = uuid.New().String()
	rawRefreshToken, err = GenerateRefreshToken()
	if err != nil {
		return "", "", err
	}
	hash, err := HashPassword(rawRefreshToken)
	if err != nil {
		return "", "", err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, refresh_token_hash, user_agent, remote_ip, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		sessionID, userID, hash, userAgent, remoteIP, time.Now().Add(duration),
	)
	return sessionID, rawRefreshToken, err
}

// Get returns the session row for a given session ID.
func (s *SessionStore) Get(ctx context.Context, sessionID string) (*SessionRow, error) {
	var row SessionRow
	err := s.db.GetContext(ctx, &row,
		`SELECT id, user_id, refresh_token_hash, user_agent, remote_ip,
		        expires_at, revoked, created_at
		   FROM sessions WHERE id = ?`, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSessionNotFound
	}
	return &row, err
}

// Validate checks that the session is active and the raw refresh token matches
// the stored hash.  Returns the session row on success.
func (s *SessionStore) Validate(ctx context.Context, sessionID, rawRefreshToken string) (*SessionRow, error) {
	row, err := s.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if row.Revoked {
		return nil, ErrSessionRevoked
	}
	if time.Now().After(row.ExpiresAt) {
		return nil, ErrSessionExpired
	}
	if !CheckPassword(row.RefreshTokenHash, rawRefreshToken) {
		return nil, errors.New("invalid refresh token")
	}
	return row, nil
}

// Revoke marks a single session as revoked (logout or compromise response).
func (s *SessionStore) Revoke(ctx context.Context, sessionID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET revoked = 1 WHERE id = ?`, sessionID)
	return err
}

// RevokeAllForUser revokes every session for a user (force-logout all devices).
func (s *SessionStore) RevokeAllForUser(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET revoked = 1 WHERE user_id = ?`, userID)
	return err
}

// Purge deletes expired and revoked session rows.  Call periodically (e.g.
// once per hour in a background goroutine) to keep the table small.
func (s *SessionStore) Purge(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE expires_at < ? OR revoked = 1`, time.Now())
	return err
}
