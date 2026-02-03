package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"imagefinder/internal/config"
	"imagefinder/internal/db"
	"imagefinder/internal/httpserver"
	"imagefinder/internal/providers"
	"imagefinder/internal/security"
	"imagefinder/internal/services"
	"imagefinder/internal/store"
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
	session := security.NewSessionManager(cfg.SessionSecret, cfg.SessionTTL, cfg.CookieSecure)
	client := &http.Client{Timeout: 20 * time.Second}
	
	authService := &services.AuthService{Config: cfg, Store: store, Client: client}

	var embedder providers.Embedder
	if cfg.OpenAIMode == "api" {
		embedder = &providers.OpenAIEmbedder{Endpoint: cfg.OpenAIAPIEndpoint, APIKey: cfg.OpenAIAPIKey, Model: cfg.OpenAIModel, Client: client}
	} else {
		embedder = &providers.MockEmbedder{Dim: cfg.EmbeddingDim}
	}

	searchService := &services.SearchService{Store: store, Embedder: embedder, MaxLimit: cfg.MaxSearchLimit}

	handler := httpserver.NewServer(cfg, store, authService, searchService, session)

	server := &http.Server{Addr: cfg.HTTPAddr, Handler: handler}

	go func() {
		log.Printf("API サーバを起動しました: %s", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("サーバ起動に失敗: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("サーバ停止に失敗: %v", err)
	}
}
