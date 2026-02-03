package services

import (
	"context"
	"strings"
	"time"

	"imagefinder/internal/models"
	"imagefinder/internal/providers"
	"imagefinder/internal/store"

	"github.com/google/uuid"
	pgvector "github.com/pgvector/pgvector-go"
)

type Indexer struct {
	Store       *store.Store
	Photos      providers.GooglePhotosClient
	Captioner   providers.Captioner
	Embedder    providers.Embedder
	AuthService *AuthService
	PageSize    int
}

func (i *Indexer) Run(ctx context.Context, job models.Job) error {
	tokens, err := i.Store.GetTokens(ctx, job.UserID)
	if err != nil {
		return err
	}
	if time.Now().After(tokens.ExpiresAt.Add(-1 * time.Minute)) && tokens.RefreshToken != "" {
		refreshed, err := i.AuthService.RefreshToken(ctx, tokens.RefreshToken)
		if err != nil {
			return err
		}
		accessToken := refreshed.AccessToken
		expiresAt := refreshed.ExpiresAt()
		refreshToken := tokens.RefreshToken
		if refreshed.RefreshToken != "" {
			refreshToken = refreshed.RefreshToken
		}
		if err := i.Store.SaveTokens(ctx, job.UserID, accessToken, refreshToken, expiresAt, refreshed.Scope); err != nil {
			return err
		}
		tokens.AccessToken = accessToken
		tokens.ExpiresAt = expiresAt
	}

	pageToken := ""
	processed := 0
	for {
		items, nextToken, err := i.Photos.ListMediaItems(ctx, tokens.AccessToken, i.PageSize, pageToken)
		if err != nil {
			return err
		}
		for _, item := range items {
			if !strings.HasPrefix(item.MimeType, "image/") {
				continue
			}
			exists, err := i.Store.PhotoExistsByMediaID(ctx, item.ID)
			if err != nil {
				return err
			}
			if exists {
				continue
			}
			caption, err := i.Captioner.Caption(ctx, providers.CaptionInput{
				ImageURL:    item.BaseURL + "=d",
				CreatedTime: item.CreatedTime,
				Location:    item.Location,
				AccessToken: tokens.AccessToken,
			})
			if err != nil {
				return err
			}
			finalCaption := buildCaption(caption.Caption, item.CreatedTime, item.Location, caption.Labels)
			embedding, err := i.Embedder.EmbedText(ctx, finalCaption)
			if err != nil {
				return err
			}
			photo := models.Photo{
				ID:            uuid.New(),
				UserID:        job.UserID,
				GoogleMediaID: item.ID,
				BaseURL:       item.BaseURL,
				CreatedTime:   item.CreatedTime,
				Location:      item.Location,
				PeopleCount:   &caption.PeopleCount,
				Caption:       &finalCaption,
				Embedding:     pgvector.NewVector(embedding),
				IndexedAt:     time.Now(),
			}
			if err := i.Store.SavePhoto(ctx, photo); err != nil {
				return err
			}
			processed++
			progress := processed
			if progress > 99 {
				progress = 99
			}
			if err := i.Store.UpdateJobProgress(ctx, job.ID, progress); err != nil {
				return err
			}
		}
		if nextToken == "" {
			break
		}
		pageToken = nextToken
	}
	return nil
}

func buildCaption(base string, createdTime *time.Time, location *string, labels []string) string {
	pieces := []string{}
	if strings.TrimSpace(base) != "" {
		pieces = append(pieces, base)
	}
	if createdTime != nil {
		pieces = append(pieces, createdTime.Format("2006-01-02"))
	}
	if location != nil {
		pieces = append(pieces, "場所:"+*location)
	}
	if len(labels) > 0 {
		pieces = append(pieces, "ラベル:"+strings.Join(labels, ","))
	}
	if len(pieces) == 0 {
		return "写真の説明"
	}
	return strings.Join(pieces, " ")
}
