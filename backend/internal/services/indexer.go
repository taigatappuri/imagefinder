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
	Store         *store.Store
	Photos        providers.GooglePhotosClient
	Captioner     providers.Captioner
	Embedder      providers.Embedder
	AuthService   *AuthService
	PageSize      int
	MaxTextLength int
}

func (i *Indexer) Run(ctx context.Context, job models.Job) error {
	tokens, err := i.AuthService.EnsureAccessToken(ctx, job.UserID)
	if err != nil {
		return err
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
			trimmedCaption := truncateText(finalCaption, i.MaxTextLength)
			embedding, err := i.Embedder.EmbedText(ctx, trimmedCaption)
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
				Caption:       &trimmedCaption,
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
