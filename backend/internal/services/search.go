package services

import (
	"context"
	"strings"
	"time"

	"imagefinder/internal/providers"
	"imagefinder/internal/store"

	"github.com/google/uuid"
)

type SearchService struct {
	Store    *store.Store
	Embedder providers.Embedder
	MaxLimit int
}

type SearchInput struct {
	Query    string
	Limit    int
	From     *time.Time
	To       *time.Time
	Location string
}

type SearchOutput struct {
	Items []store.SearchResult `json:"items"`
}

func (s *SearchService) Search(ctx context.Context, userID uuid.UUID, input SearchInput) (SearchOutput, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}
	if s.MaxLimit > 0 && limit > s.MaxLimit {
		limit = s.MaxLimit
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return SearchOutput{Items: []store.SearchResult{}}, nil
	}
	embedding, err := s.Embedder.EmbedText(ctx, query)
	if err != nil {
		return SearchOutput{}, err
	}
	results, err := s.Store.SearchPhotos(ctx, userID, embedding, store.SearchFilters{
		From:     input.From,
		To:       input.To,
		Location: input.Location,
	}, limit)
	if err != nil {
		return SearchOutput{}, err
	}
	return SearchOutput{Items: results}, nil
}
