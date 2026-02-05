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

type PickerService struct {
	Store         *store.Store
	PickerClient  providers.PhotosPickerClient
	Captioner     providers.Captioner
	Embedder      providers.Embedder
	AuthService   *AuthService
	PageSize      int
	MaxTextLength int
}

type PickerImportResult struct {
	Imported  int
	Failed    int
	LastError error
}

func (p *PickerService) CreateSession(ctx context.Context, userID uuid.UUID) (providers.PickerSession, error) {
	tokens, err := p.AuthService.EnsureAccessToken(ctx, userID)
	if err != nil {
		return providers.PickerSession{}, err
	}
	return p.PickerClient.CreateSession(ctx, tokens.AccessToken)
}

func (p *PickerService) GetSession(ctx context.Context, userID uuid.UUID, sessionID string) (providers.PickerSession, error) {
	tokens, err := p.AuthService.EnsureAccessToken(ctx, userID)
	if err != nil {
		return providers.PickerSession{}, err
	}
	return p.PickerClient.GetSession(ctx, tokens.AccessToken, sessionID)
}

func (p *PickerService) ImportSession(ctx context.Context, userID uuid.UUID, sessionID string) (PickerImportResult, error) {
	tokens, err := p.AuthService.EnsureAccessToken(ctx, userID)
	if err != nil {
		return PickerImportResult{}, err
	}
	defer func() {
		if p.PickerClient != nil {
			p.PickerClient.DeleteSession(ctx, tokens.AccessToken, sessionID)
		}
	}()

	pageToken := ""
	result := PickerImportResult{}
	for {
		items, nextToken, err := p.PickerClient.ListMediaItems(ctx, tokens.AccessToken, sessionID, p.PageSize, pageToken)
		if err != nil {
			return result, err
		}
		for _, item := range items {
			if !strings.HasPrefix(item.MimeType, "image/") {
				continue
			}
			exists, err := p.Store.PhotoExistsByMediaID(ctx, item.ID)
			if err != nil {
				return result, err
			}
			if exists {
				continue
			}
			caption, err := p.Captioner.Caption(ctx, providers.CaptionInput{
				ImageURL:    item.BaseURL + "=d",
				CreatedTime: item.CreatedTime,
				AccessToken: tokens.AccessToken,
			})
			if err != nil {
				result.Failed++
				result.LastError = err
				continue
			}
			finalCaption := buildCaption(caption.Caption, item.CreatedTime, nil, caption.Labels)
			trimmedCaption := truncateText(finalCaption, p.MaxTextLength)
			embedding, err := p.Embedder.EmbedText(ctx, trimmedCaption)
			if err != nil {
				result.Failed++
				result.LastError = err
				continue
			}
			photo := models.Photo{
				ID:            uuid.New(),
				UserID:        userID,
				GoogleMediaID: item.ID,
				BaseURL:       item.BaseURL,
				CreatedTime:   item.CreatedTime,
				Caption:       &trimmedCaption,
				Embedding:     pgvector.NewVector(embedding),
				IndexedAt:     time.Now(),
			}
			if err := p.Store.SavePhoto(ctx, photo); err != nil {
				result.Failed++
				result.LastError = err
				continue
			}
			result.Imported++
		}
		if nextToken == "" {
			break
		}
		pageToken = nextToken
	}
	return result, nil
}
