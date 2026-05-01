package auth

import "time"

type Session struct {
	ID        string
	UserID    string
	UserAgent string
	IPAddress string
	RevokedAt *time.Time
	CreatedAt time.Time
	ExpiresAt time.Time
}

type RefreshToken struct {
	ID         string
	SessionID  string
	TokenHash  string
	RevokedAt  *time.Time
	GraceUntil *time.Time
	CreatedAt  time.Time
	ExpiresAt  time.Time
}
