package models

import (
	"time"

	"github.com/google/uuid"
	pgvector "github.com/pgvector/pgvector-go"
)

type User struct {
	ID           uuid.UUID `json:"id"`
	GoogleUserID string    `json:"google_user_id"`
	Email        string    `json:"email"`
	CreatedAt    time.Time `json:"created_at"`
}

type OAuthToken struct {
	UserID       uuid.UUID `json:"user_id"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scopes       string    `json:"scopes"`
}

type Job struct {
	ID           uuid.UUID  `json:"id"`
	UserID       uuid.UUID  `json:"user_id"`
	Type         string     `json:"type"`
	Status       string     `json:"status"`
	Progress     int        `json:"progress"`
	StartedAt    *time.Time `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at"`
	ErrorMessage *string    `json:"error_message"`
	CreatedAt    time.Time  `json:"created_at"`
}

type Photo struct {
	ID            uuid.UUID      `json:"id"`
	UserID        uuid.UUID      `json:"user_id"`
	GoogleMediaID string         `json:"google_media_id"`
	BaseURL       string         `json:"base_url"`
	CreatedTime   *time.Time     `json:"created_time"`
	Location      *string        `json:"location"`
	PeopleCount   *int           `json:"people_count"`
	Caption       *string        `json:"caption"`
	Embedding     pgvector.Vector `json:"embedding"`
	IndexedAt     time.Time      `json:"indexed_at"`
}
