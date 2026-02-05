package services

import (
	"context"
	"errors"
	"strings"
	"sync"
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
	ImportTracker *PickerImportTracker
	PageSize      int
	MaxTextLength int
}

type PickerImportStatus struct {
	UserID    uuid.UUID `json:"-"`
	Status    string    `json:"status"`
	Total     int       `json:"total"`
	Processed int       `json:"processed"`
	Imported  int       `json:"imported"`
	Failed    int       `json:"failed"`
	Warning   string    `json:"warning,omitempty"`
	Error     string    `json:"error,omitempty"`
}

type PickerImportTracker struct {
	mu     sync.Mutex
	status map[string]*PickerImportStatus
}

func NewPickerImportTracker() *PickerImportTracker {
	return &PickerImportTracker{status: make(map[string]*PickerImportStatus)}
}

func (t *PickerImportTracker) Start(sessionID string, userID uuid.UUID) (*PickerImportStatus, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if current, ok := t.status[sessionID]; ok && current.Status == "running" {
		return nil, errors.New("取り込みがすでに進行中です")
	}
	status := &PickerImportStatus{UserID: userID, Status: "running"}
	t.status[sessionID] = status
	return status, nil
}

func (t *PickerImportTracker) Update(sessionID string, fn func(status *PickerImportStatus)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	status, ok := t.status[sessionID]
	if !ok {
		return
	}
	fn(status)
}

func (t *PickerImportTracker) Get(sessionID string, userID uuid.UUID) (PickerImportStatus, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	status, ok := t.status[sessionID]
	if !ok || status.UserID != userID {
		return PickerImportStatus{}, false
	}
	return *status, true
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

func (p *PickerService) StartImport(ctx context.Context, userID uuid.UUID, sessionID string) (PickerImportStatus, error) {
	if p.ImportTracker == nil {
		return PickerImportStatus{}, errors.New("取り込みトラッカーが設定されていません")
	}
	if sessionID == "" {
		return PickerImportStatus{}, errors.New("セッション ID が空です")
	}
	_, err := p.ImportTracker.Start(sessionID, userID)
	if err != nil {
		return PickerImportStatus{}, err
	}
	tokens, err := p.AuthService.EnsureAccessToken(ctx, userID)
	if err != nil {
		p.ImportTracker.Update(sessionID, func(status *PickerImportStatus) {
			status.Status = "failed"
			status.Error = err.Error()
		})
		return PickerImportStatus{}, err
	}

	items, err := p.collectMediaItems(ctx, tokens.AccessToken, sessionID)
	if err != nil {
		p.ImportTracker.Update(sessionID, func(status *PickerImportStatus) {
			status.Status = "failed"
			status.Error = err.Error()
		})
		return PickerImportStatus{}, err
	}
	p.ImportTracker.Update(sessionID, func(status *PickerImportStatus) {
		status.Total = len(items)
	})

	go p.runImport(userID, sessionID, tokens.AccessToken, items)

	status, _ := p.ImportTracker.Get(sessionID, userID)
	return status, nil
}

func (p *PickerService) GetImportStatus(userID uuid.UUID, sessionID string) (PickerImportStatus, bool) {
	if p.ImportTracker == nil {
		return PickerImportStatus{}, false
	}
	return p.ImportTracker.Get(sessionID, userID)
}

func (p *PickerService) collectMediaItems(ctx context.Context, accessToken, sessionID string) ([]providers.PickedMediaItem, error) {
	pageToken := ""
	items := []providers.PickedMediaItem{}
	for {
		page, nextToken, err := p.PickerClient.ListMediaItems(ctx, accessToken, sessionID, p.PageSize, pageToken)
		if err != nil {
			return nil, err
		}
		for _, item := range page {
			if !strings.HasPrefix(item.MimeType, "image/") {
				continue
			}
			items = append(items, item)
		}
		if nextToken == "" {
			break
		}
		pageToken = nextToken
	}
	return items, nil
}

func (p *PickerService) runImport(userID uuid.UUID, sessionID, accessToken string, items []providers.PickedMediaItem) {
	ctx := context.Background()
	defer func() {
		if p.PickerClient != nil {
			p.PickerClient.DeleteSession(ctx, accessToken, sessionID)
		}
	}()

	processed := 0
	imported := 0
	failed := 0
	lastWarning := ""

	for _, item := range items {
		exists, err := p.Store.PhotoExistsByMediaID(ctx, item.ID)
		if err != nil {
			failed++
			lastWarning = err.Error()
			processed++
			p.updateProgress(sessionID, processed, imported, failed, lastWarning)
			continue
		}
		if exists {
			processed++
			p.updateProgress(sessionID, processed, imported, failed, lastWarning)
			continue
		}
		caption, err := p.Captioner.Caption(ctx, providers.CaptionInput{
			ImageURL:    item.BaseURL + "=d",
			CreatedTime: item.CreatedTime,
			AccessToken: accessToken,
		})
		if err != nil {
			failed++
			lastWarning = err.Error()
			processed++
			p.updateProgress(sessionID, processed, imported, failed, lastWarning)
			continue
		}
		finalCaption := buildCaption(caption.Caption, item.CreatedTime, nil, caption.Labels)
		trimmedCaption := truncateText(finalCaption, p.MaxTextLength)
		embedding, err := p.Embedder.EmbedText(ctx, trimmedCaption)
		if err != nil {
			failed++
			lastWarning = err.Error()
			processed++
			p.updateProgress(sessionID, processed, imported, failed, lastWarning)
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
			failed++
			lastWarning = err.Error()
			processed++
			p.updateProgress(sessionID, processed, imported, failed, lastWarning)
			continue
		}
		imported++
		processed++
		p.updateProgress(sessionID, processed, imported, failed, lastWarning)
	}

	p.ImportTracker.Update(sessionID, func(status *PickerImportStatus) {
		status.Status = "done"
		status.Processed = processed
		status.Imported = imported
		status.Failed = failed
		status.Warning = lastWarning
	})
}

func (p *PickerService) updateProgress(sessionID string, processed, imported, failed int, warning string) {
	if p.ImportTracker == nil {
		return
	}
	p.ImportTracker.Update(sessionID, func(status *PickerImportStatus) {
		status.Status = "running"
		status.Processed = processed
		status.Imported = imported
		status.Failed = failed
		status.Warning = warning
	})
}
