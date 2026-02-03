package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"imagefinder/internal/config"
	"imagefinder/internal/db"
	"imagefinder/internal/jobs"
	"imagefinder/internal/providers"
	"imagefinder/internal/security"
	"imagefinder/internal/services"
	"imagefinder/internal/store"

	"github.com/jackc/pgx/v5"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("DB 接続に失敗: %v", err)
	}
	defer pool.Close()
	if cfg.AutoMigrate {
		if err := db.EnsureSchema(ctx, pool, cfg.EmbeddingDim); err != nil {
			log.Fatalf("スキーマ作成に失敗: %v", err)
		}
	}

	var cipher *security.Cipher
	if cfg.TokenEncryptionKey != "" {
		cipher, err = security.NewCipherFromBase64(cfg.TokenEncryptionKey)
		if err != nil {
			log.Fatalf("暗号化キーの読み込みに失敗: %v", err)
		}
	}

	store := &store.Store{DB: pool, Cipher: cipher}
	client := &http.Client{Timeout: 30 * time.Second}
	
	authService := &services.AuthService{Config: cfg, Store: store, Client: client}

	var photosClient providers.GooglePhotosClient
	if cfg.GoogleClientID != "" {
		photosClient = &providers.HTTPGooglePhotosClient{BaseURL: cfg.GooglePhotosAPIBase, Client: client}
	} else {
		photosClient = &providers.MockGooglePhotosClient{Items: mockPhotos()}
	}

	var captioner providers.Captioner
	if cfg.GeminiMode == "api" {
		captioner = &providers.GeminiCaptioner{Endpoint: cfg.GeminiAPIEndpoint, APIKey: cfg.GeminiAPIKey, Client: client}
	} else {
		captioner = &providers.MockCaptioner{}
	}

	var embedder providers.Embedder
	if cfg.OpenAIMode == "api" {
		embedder = &providers.OpenAIEmbedder{Endpoint: cfg.OpenAIAPIEndpoint, APIKey: cfg.OpenAIAPIKey, Model: cfg.OpenAIModel, Client: client}
	} else {
		embedder = &providers.MockEmbedder{Dim: cfg.EmbeddingDim}
	}

	indexer := &services.Indexer{
		Store:       store,
		Photos:      photosClient,
		Captioner:   captioner,
		Embedder:    embedder,
		AuthService: authService,
		PageSize:    cfg.GooglePhotosPageSize,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("ワーカーを起動しました")
	for {
		select {
		case <-stop:
			log.Printf("ワーカーを停止します")
			return
		default:
		}

		job, err := store.AcquireNextJob(ctx)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				time.Sleep(cfg.JobPollInterval)
				continue
			}
			log.Printf("ジョブ取得に失敗: %v", err)
			time.Sleep(cfg.JobPollInterval)
			continue
		}

		switch job.Type {
		case jobs.TypeIndex:
			if err := indexer.Run(ctx, job); err != nil {
				message := err.Error()
				if err := store.UpdateJobStatus(ctx, job.ID, jobs.StatusFailed, -1, &message); err != nil {
					log.Printf("ジョブ失敗更新に失敗: %v", err)
				}
				continue
			}
			if err := store.UpdateJobStatus(ctx, job.ID, jobs.StatusDone, 100, nil); err != nil {
				log.Printf("ジョブ完了更新に失敗: %v", err)
			}
		default:
			message := "未対応のジョブ種別です"
			if err := store.UpdateJobStatus(ctx, job.ID, jobs.StatusFailed, -1, &message); err != nil {
				log.Printf("ジョブ失敗更新に失敗: %v", err)
			}
		}
	}
}

func mockPhotos() []providers.MediaItem {
	items := []providers.MediaItem{}
	base := time.Now().AddDate(0, 0, -10)
	for i := 0; i < 5; i++ {
		created := base.AddDate(0, 0, i)
		location := "東京"
		items = append(items, providers.MediaItem{
			ID:          fmt.Sprintf("mock-%d", i+1),
			BaseURL:     "https://images.unsplash.com/photo-1500530855697-b586d89ba3ee",
			MimeType:    "image/jpeg",
			CreatedTime: &created,
			Location:    &location,
		})
	}
	return items
}
