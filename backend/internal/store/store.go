package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"imagefinder/internal/models"
	"imagefinder/internal/security"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
)

type Store struct {
	DB             *pgxpool.Pool
	Encryptor      security.Encryptor
	UseExactSearch bool
}

func (s *Store) UpsertUser(ctx context.Context, googleUserID, email string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.DB.QueryRow(ctx, "SELECT id FROM users WHERE google_user_id=$1", googleUserID).Scan(&id)
	if err == nil {
		_, updateErr := s.DB.Exec(ctx, "UPDATE users SET email=$1 WHERE id=$2", email, id)
		return id, updateErr
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, err
	}
	id = uuid.New()
	_, err = s.DB.Exec(ctx, "INSERT INTO users (id, google_user_id, email, created_at) VALUES ($1,$2,$3,$4)", id, googleUserID, email, time.Now())
	return id, err
}

func (s *Store) SaveTokens(ctx context.Context, userID uuid.UUID, accessToken, refreshToken string, expiresAt time.Time, scopes string) error {
	encAccess, encRefresh := accessToken, refreshToken
	if s.Encryptor != nil {
		var err error
		encAccess, err = s.Encryptor.Encrypt(accessToken)
		if err != nil {
			return err
		}
		if refreshToken != "" {
			encRefresh, err = s.Encryptor.Encrypt(refreshToken)
			if err != nil {
				return err
			}
		}
	}
	_, err := s.DB.Exec(ctx, `
INSERT INTO oauth_tokens (user_id, access_token, refresh_token, expires_at, scopes)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (user_id)
DO UPDATE SET access_token=EXCLUDED.access_token, refresh_token=EXCLUDED.refresh_token, expires_at=EXCLUDED.expires_at, scopes=EXCLUDED.scopes`,
		userID, encAccess, encRefresh, expiresAt, scopes)
	return err
}

func (s *Store) GetTokens(ctx context.Context, userID uuid.UUID) (models.OAuthToken, error) {
	var token models.OAuthToken
	var accessToken string
	var refreshToken string
	row := s.DB.QueryRow(ctx, "SELECT access_token, refresh_token, expires_at, scopes FROM oauth_tokens WHERE user_id=$1", userID)
	if err := row.Scan(&accessToken, &refreshToken, &token.ExpiresAt, &token.Scopes); err != nil {
		return token, err
	}
	if s.Encryptor != nil {
		var err error
		accessToken, err = s.Encryptor.Decrypt(accessToken)
		if err != nil {
			return token, err
		}
		if refreshToken != "" {
			refreshToken, err = s.Encryptor.Decrypt(refreshToken)
			if err != nil {
				return token, err
			}
		}
	}
	token.UserID = userID
	token.AccessToken = accessToken
	token.RefreshToken = refreshToken
	return token, nil
}

func (s *Store) CreateJob(ctx context.Context, userID uuid.UUID, jobType string) (uuid.UUID, error) {
	id := uuid.New()
	_, err := s.DB.Exec(ctx, "INSERT INTO jobs (id, user_id, type, status, progress, created_at) VALUES ($1,$2,$3,$4,$5,$6)", id, userID, jobType, "queued", 0, time.Now())
	return id, err
}

func (s *Store) UpdateJobStatus(ctx context.Context, jobID uuid.UUID, status string, progress int, errMessage *string) error {
	_, err := s.DB.Exec(ctx, `
UPDATE jobs
SET status=$2,
	progress=CASE WHEN $3 < 0 THEN progress ELSE $3 END,
	error_message=$4,
	started_at=CASE WHEN $2='running' AND started_at IS NULL THEN NOW() ELSE started_at END,
	finished_at=CASE WHEN $2 IN ('done','failed') THEN NOW() ELSE finished_at END
WHERE id=$1`, jobID, status, progress, errMessage)
	return err
}

func (s *Store) UpdateJobProgress(ctx context.Context, jobID uuid.UUID, progress int) error {
	_, err := s.DB.Exec(ctx, "UPDATE jobs SET progress=$2 WHERE id=$1", jobID, progress)
	return err
}

func (s *Store) GetLatestJob(ctx context.Context, userID uuid.UUID) (models.Job, error) {
	var job models.Job
	row := s.DB.QueryRow(ctx, `
SELECT id, user_id, type, status, progress, started_at, finished_at, error_message, created_at
FROM jobs
WHERE user_id=$1
ORDER BY created_at DESC
LIMIT 1`, userID)
	err := row.Scan(&job.ID, &job.UserID, &job.Type, &job.Status, &job.Progress, &job.StartedAt, &job.FinishedAt, &job.ErrorMessage, &job.CreatedAt)
	return job, err
}

func (s *Store) HasActiveJob(ctx context.Context, userID uuid.UUID) (bool, error) {
	var exists bool
	err := s.DB.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM jobs WHERE user_id=$1 AND status IN ('queued','running'))", userID).Scan(&exists)
	return exists, err
}

func (s *Store) AcquireNextJob(ctx context.Context) (models.Job, error) {
	var job models.Job
	transaction, err := s.DB.Begin(ctx)
	if err != nil {
		return job, err
	}
	defer transaction.Rollback(ctx)
	row := transaction.QueryRow(ctx, `
SELECT id, user_id, type, status, progress, started_at, finished_at, error_message, created_at
FROM jobs
WHERE status='queued'
ORDER BY created_at
FOR UPDATE SKIP LOCKED
LIMIT 1`)
	if err := row.Scan(&job.ID, &job.UserID, &job.Type, &job.Status, &job.Progress, &job.StartedAt, &job.FinishedAt, &job.ErrorMessage, &job.CreatedAt); err != nil {
		return job, err
	}
	if _, err := transaction.Exec(ctx, "UPDATE jobs SET status='running', started_at=NOW() WHERE id=$1", job.ID); err != nil {
		return job, err
	}
	if err := transaction.Commit(ctx); err != nil {
		return job, err
	}
	job.Status = "running"
	return job, nil
}

func (s *Store) PhotoExistsByMediaID(ctx context.Context, googleMediaID string) (bool, error) {
	var exists bool
	err := s.DB.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM photos WHERE google_media_id=$1)", googleMediaID).Scan(&exists)
	return exists, err
}

func (s *Store) SavePhoto(ctx context.Context, photo models.Photo) error {
	_, err := s.DB.Exec(ctx, `
INSERT INTO photos (id, user_id, google_media_id, base_url, created_time, location, people_count, caption, embedding, indexed_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (google_media_id) DO UPDATE
SET base_url=EXCLUDED.base_url,
	created_time=EXCLUDED.created_time,
	location=EXCLUDED.location,
	people_count=EXCLUDED.people_count,
	caption=EXCLUDED.caption,
	embedding=EXCLUDED.embedding,
	indexed_at=EXCLUDED.indexed_at`,
		photo.ID, photo.UserID, photo.GoogleMediaID, photo.BaseURL, photo.CreatedTime, photo.Location, photo.PeopleCount, photo.Caption, photo.Embedding, photo.IndexedAt)
	return err
}

func (s *Store) GetPhotoByID(ctx context.Context, userID uuid.UUID, photoID uuid.UUID) (models.Photo, error) {
	var photo models.Photo
	row := s.DB.QueryRow(ctx, `
SELECT id, user_id, google_media_id, base_url, created_time, location, people_count, caption, embedding, indexed_at
FROM photos
WHERE id=$1 AND user_id=$2`, photoID, userID)
	err := row.Scan(&photo.ID, &photo.UserID, &photo.GoogleMediaID, &photo.BaseURL, &photo.CreatedTime, &photo.Location, &photo.PeopleCount, &photo.Caption, &photo.Embedding, &photo.IndexedAt)
	return photo, err
}

type SearchFilters struct {
	From     *time.Time
	To       *time.Time
	Location string
}

type SearchResult struct {
	ID        uuid.UUID  `json:"id"`
	BaseURL   string     `json:"base_url"`
	CreatedAt *time.Time `json:"created_time"`
	Location  *string    `json:"location"`
	Score     float32    `json:"score"`
}

func (s *Store) SearchPhotos(ctx context.Context, userID uuid.UUID, embedding []float32, filters SearchFilters, limit int) ([]SearchResult, error) {
	vector := pgvector.NewVector(embedding)
	query := "SELECT id, base_url, created_time, location, 1 - (embedding <=> $1) AS score FROM photos WHERE user_id=$2"
	args := []any{vector, userID}
	index := 3
	if filters.From != nil {
		query += fmt.Sprintf(" AND created_time >= $%d", index)
		args = append(args, *filters.From)
		index++
	}
	if filters.To != nil {
		query += fmt.Sprintf(" AND created_time <= $%d", index)
		args = append(args, *filters.To)
		index++
	}
	if strings.TrimSpace(filters.Location) != "" {
		query += fmt.Sprintf(" AND location ILIKE $%d", index)
		args = append(args, "%"+filters.Location+"%")
		index++
	}
	query += fmt.Sprintf(" ORDER BY embedding <=> $1 LIMIT $%d", index)
	args = append(args, limit)

	fetch := func(ctx context.Context, q string, qargs []any, qer interface {
		Query(context.Context, string, ...any) (pgx.Rows, error)
	}) ([]SearchResult, error) {
		rows, err := qer.Query(ctx, q, qargs...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		results := []SearchResult{}
		for rows.Next() {
			var item SearchResult
			if err := rows.Scan(&item.ID, &item.BaseURL, &item.CreatedAt, &item.Location, &item.Score); err != nil {
				return nil, err
			}
			results = append(results, item)
		}
		return results, rows.Err()
	}

	runExact := func() ([]SearchResult, error) {
		tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
		if err != nil {
			return []SearchResult{}, nil
		}
		defer tx.Rollback(ctx)

		if _, err := tx.Exec(ctx, "SET LOCAL enable_indexscan=off"); err != nil {
			return []SearchResult{}, nil
		}
		if _, err := tx.Exec(ctx, "SET LOCAL enable_bitmapscan=off"); err != nil {
			return []SearchResult{}, nil
		}
		if _, err := tx.Exec(ctx, "SET LOCAL enable_indexonlyscan=off"); err != nil {
			return []SearchResult{}, nil
		}
		results, err := fetch(ctx, query, args, tx)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return results, nil
		}
		return results, nil
	}

	if s.UseExactSearch {
		return runExact()
	}

	results, err := fetch(ctx, query, args, s.DB)
	if err != nil {
		return nil, err
	}
	if len(results) > 0 {
		return results, nil
	}

	return runExact()
}
