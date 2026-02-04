package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"imagefinder/internal/config"
	"imagefinder/internal/models"
	"imagefinder/internal/retry"
	"imagefinder/internal/store"

	"github.com/google/uuid"
)

type AuthService struct {
	Config config.Config
	Store  *store.Store
	Client *http.Client
}

func (s *AuthService) AuthURL(state string) (string, error) {
	endpoint, err := url.Parse(s.Config.GoogleOAuthAuthURL)
	if err != nil {
		return "", err
	}
	query := endpoint.Query()
	query.Set("client_id", s.Config.GoogleClientID)
	query.Set("redirect_uri", s.Config.GoogleRedirectURL)
	query.Set("response_type", "code")
	query.Set("scope", strings.Join(s.Config.GoogleOAuthScopes, " "))
	query.Set("access_type", "offline")
	query.Set("prompt", "consent")
	query.Set("state", state)
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

func (s *AuthService) ExchangeCode(ctx context.Context, code string) (tokenResponse, error) {
	values := url.Values{}
	values.Set("code", code)
	values.Set("client_id", s.Config.GoogleClientID)
	values.Set("client_secret", s.Config.GoogleClientSecret)
	values.Set("redirect_uri", s.Config.GoogleRedirectURL)
	values.Set("grant_type", "authorization_code")

	var token tokenResponse
	err := retry.Do(ctx, s.Config.ExternalAPIRetryMax, s.Config.ExternalAPIRetryDelay, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Config.GoogleOAuthTokenURL, strings.NewReader(values.Encode()))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := s.Client.Do(req)
		if err != nil {
			return retry.MarkRetryable(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			return retry.MarkRetryable(fmt.Errorf("トークン交換エラー: %s", resp.Status))
		}
		if resp.StatusCode >= 300 {
			return fmt.Errorf("トークン交換エラー: %s", resp.Status)
		}
		if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return tokenResponse{}, err
	}
	return token, nil
}

func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (tokenResponse, error) {
	values := url.Values{}
	values.Set("refresh_token", refreshToken)
	values.Set("client_id", s.Config.GoogleClientID)
	values.Set("client_secret", s.Config.GoogleClientSecret)
	values.Set("grant_type", "refresh_token")

	var token tokenResponse
	err := retry.Do(ctx, s.Config.ExternalAPIRetryMax, s.Config.ExternalAPIRetryDelay, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Config.GoogleOAuthTokenURL, strings.NewReader(values.Encode()))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := s.Client.Do(req)
		if err != nil {
			return retry.MarkRetryable(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			return retry.MarkRetryable(fmt.Errorf("トークン更新エラー: %s", resp.Status))
		}
		if resp.StatusCode >= 300 {
			return fmt.Errorf("トークン更新エラー: %s", resp.Status)
		}
		if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return tokenResponse{}, err
	}
	return token, nil
}

func (s *AuthService) FetchUserInfo(ctx context.Context, accessToken string) (userInfo, error) {
	var info userInfo
	err := retry.Do(ctx, s.Config.ExternalAPIRetryMax, s.Config.ExternalAPIRetryDelay, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.Config.GoogleUserInfoURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		resp, err := s.Client.Do(req)
		if err != nil {
			return retry.MarkRetryable(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			return retry.MarkRetryable(fmt.Errorf("ユーザー情報取得エラー: %s", resp.Status))
		}
		if resp.StatusCode >= 300 {
			return fmt.Errorf("ユーザー情報取得エラー: %s", resp.Status)
		}
		if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return userInfo{}, err
	}
	return info, nil
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
	IDToken      string `json:"id_token"`
}

func (t tokenResponse) ExpiresAt() time.Time {
	return time.Now().Add(time.Duration(t.ExpiresIn) * time.Second)
}

type userInfo struct {
	Sub   string `json:"sub"`
	ID    string `json:"id"`
	Email string `json:"email"`
}

func (u userInfo) GoogleID() string {
	if u.Sub != "" {
		return u.Sub
	}
	return u.ID
}

func (s *AuthService) EnsureAccessToken(ctx context.Context, userID uuid.UUID) (models.OAuthToken, error) {
	tokens, err := s.Store.GetTokens(ctx, userID)
	if err != nil {
		return models.OAuthToken{}, err
	}
	if time.Now().After(tokens.ExpiresAt.Add(-1*time.Minute)) && tokens.RefreshToken != "" {
		refreshed, err := s.RefreshToken(ctx, tokens.RefreshToken)
		if err != nil {
			return models.OAuthToken{}, err
		}
		accessToken := refreshed.AccessToken
		expiresAt := refreshed.ExpiresAt()
		refreshToken := tokens.RefreshToken
		if refreshed.RefreshToken != "" {
			refreshToken = refreshed.RefreshToken
		}
		if err := s.Store.SaveTokens(ctx, userID, accessToken, refreshToken, expiresAt, refreshed.Scope); err != nil {
			return models.OAuthToken{}, err
		}
		tokens.AccessToken = accessToken
		tokens.ExpiresAt = expiresAt
		tokens.RefreshToken = refreshToken
	}
	return tokens, nil
}
