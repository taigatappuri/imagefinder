package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"imagefinder/internal/config"
	"imagefinder/internal/jobs"
	"imagefinder/internal/security"
	"imagefinder/internal/services"
	"imagefinder/internal/store"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Server struct {
	Config  config.Config
	Store   *store.Store
	Auth    *services.AuthService
	Search  *services.SearchService
	Picker  *services.PickerService
	Session *security.SessionManager
}

func NewServer(cfg config.Config, store *store.Store, auth *services.AuthService, search *services.SearchService, picker *services.PickerService, session *security.SessionManager) http.Handler {
	s := &Server{
		Config:  cfg,
		Store:   store,
		Auth:    auth,
		Search:  search,
		Picker:  picker,
		Session: session,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/config", s.handleConfig)
	mux.HandleFunc("/auth/google", s.handleAuthGoogle)
	mux.HandleFunc("/auth/callback", s.handleAuthCallback)
	mux.HandleFunc("/auth/logout", s.handleAuthLogout)
	mux.HandleFunc("/auth/me", s.handleAuthMe)
	mux.HandleFunc("/index/update", s.handleIndexUpdate)
	mux.HandleFunc("/index/status", s.handleIndexStatus)
	mux.HandleFunc("/picker/session", s.handlePickerSession)
	mux.HandleFunc("/picker/session/", s.handlePickerSessionDetail)
	mux.HandleFunc("/picker/import", s.handlePickerImport)
	mux.HandleFunc("/search", s.handleSearch)
	mux.HandleFunc("/photos/", s.handlePhotoRoutes)
	return s.withCORS(s.withLogging(mux))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET のみ対応しています")
		return
	}
	mode := s.Config.GooglePhotosMode
	writeJSON(w, http.StatusOK, map[string]any{
		"google_photos_mode": mode,
		"indexing_available": mode != "picker",
		"picker_available":   mode == "picker",
	})
}

func (s *Server) handleAuthGoogle(w http.ResponseWriter, r *http.Request) {
	if s.Config.GoogleClientID == "" || s.Config.GoogleClientSecret == "" {
		if s.Config.AppEnv == "development" {
			userID, err := s.Store.UpsertUser(r.Context(), "dev-user", "dev@example.com")
			if err != nil {
				writeError(w, http.StatusInternalServerError, "開発用ユーザーの作成に失敗しました")
				return
			}
			expiresAt := time.Now().Add(24 * time.Hour)
			if err := s.Store.SaveTokens(r.Context(), userID, "dev-access-token", "dev-refresh-token", expiresAt, "mock"); err != nil {
				writeError(w, http.StatusInternalServerError, "開発用トークンの保存に失敗しました")
				return
			}
			http.SetCookie(w, s.Session.CreateSessionCookie(userID.String()))
			http.Redirect(w, r, s.Config.FrontendBaseURL, http.StatusFound)
			return
		}
		writeError(w, http.StatusBadRequest, "Google OAuth の設定が不足しています")
		return
	}
	state, err := s.Session.NewState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "state 生成に失敗しました")
		return
	}
	url, err := s.Auth.AuthURL(state)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "認可 URL の生成に失敗しました")
		return
	}
	http.SetCookie(w, s.Session.CreateStateCookie(state))
	if r.Method == http.MethodPost {
		writeJSON(w, http.StatusOK, map[string]string{"url": url})
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		writeError(w, http.StatusBadRequest, "state または code が不足しています")
		return
	}
	if !s.Session.VerifyState(r, state) {
		writeError(w, http.StatusBadRequest, "state 検証に失敗しました")
		return
	}
	token, err := s.Auth.ExchangeCode(r.Context(), code)
	if err != nil {
		writeError(w, http.StatusBadRequest, "トークン交換に失敗しました")
		return
	}
	userInfo, err := s.Auth.FetchUserInfo(r.Context(), token.AccessToken)
	if err != nil {
		writeError(w, http.StatusBadRequest, "ユーザー情報の取得に失敗しました")
		return
	}
	googleID := userInfo.GoogleID()
	if googleID == "" {
		writeError(w, http.StatusBadRequest, "Google ユーザー ID が取得できませんでした")
		return
	}
	userID, err := s.Store.UpsertUser(r.Context(), googleID, userInfo.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ユーザー保存に失敗しました")
		return
	}
	expiresAt := token.ExpiresAt()
	if err := s.Store.SaveTokens(r.Context(), userID, token.AccessToken, token.RefreshToken, expiresAt, token.Scope); err != nil {
		writeError(w, http.StatusInternalServerError, "トークン保存に失敗しました")
		return
	}
	http.SetCookie(w, s.Session.CreateSessionCookie(userID.String()))
	http.Redirect(w, r, s.Config.FrontendBaseURL, http.StatusFound)
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, s.Session.ClearSessionCookie())
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	userID, err := s.requireUserID(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "user_id": userID.String()})
}

func (s *Server) handleIndexUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST のみ対応しています")
		return
	}
	if s.Config.GooglePhotosMode == "picker" {
		writeError(w, http.StatusBadRequest, "Google Photos Picker を使用してください")
		return
	}
	userID, err := s.requireUserID(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "認証が必要です")
		return
	}
	active, err := s.Store.HasActiveJob(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ジョブ状態の確認に失敗しました")
		return
	}
	if active {
		writeError(w, http.StatusConflict, "進行中のジョブがあります")
		return
	}
	jobID, err := s.Store.CreateJob(r.Context(), userID, jobs.TypeIndex)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ジョブ作成に失敗しました")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"job_id": jobID.String()})
}

func (s *Server) handleIndexStatus(w http.ResponseWriter, r *http.Request) {
	userID, err := s.requireUserID(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "認証が必要です")
		return
	}
	job, err := s.Store.GetLatestJob(r.Context(), userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusOK, map[string]any{"job": nil})
			return
		}
		writeError(w, http.StatusInternalServerError, "ジョブ取得に失敗しました")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job": job})
}

func (s *Server) handlePickerSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST のみ対応しています")
		return
	}
	if s.Picker == nil || s.Picker.PickerClient == nil {
		writeError(w, http.StatusBadRequest, "Google Photos Picker の設定が不足しています")
		return
	}
	userID, err := s.requireUserID(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "認証が必要です")
		return
	}
	session, err := s.Picker.CreateSession(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Picker セッションの作成に失敗しました")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id":     session.ID,
		"picker_uri":     session.PickerURI,
		"polling_config": session.PollingConfig,
	})
}

func (s *Server) handlePickerSessionDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "GET または DELETE のみ対応しています")
		return
	}
	if s.Picker == nil || s.Picker.PickerClient == nil {
		writeError(w, http.StatusBadRequest, "Google Photos Picker の設定が不足しています")
		return
	}
	userID, err := s.requireUserID(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "認証が必要です")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/picker/session/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "セッション ID が不正です")
		return
	}
	if r.Method == http.MethodDelete {
		tokens, err := s.Auth.EnsureAccessToken(r.Context(), userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "アクセストークンの取得に失敗しました")
			return
		}
		if err := s.Picker.PickerClient.DeleteSession(r.Context(), tokens.AccessToken, id); err != nil {
			writeError(w, http.StatusBadRequest, "セッションの削除に失敗しました")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		return
	}
	session, err := s.Picker.GetSession(r.Context(), userID, id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Picker セッションの取得に失敗しました")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id":      session.ID,
		"media_items_set": session.MediaItemsSet,
		"polling_config":  session.PollingConfig,
	})
}

func (s *Server) handlePickerImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST のみ対応しています")
		return
	}
	if s.Picker == nil || s.Picker.PickerClient == nil {
		writeError(w, http.StatusBadRequest, "Google Photos Picker の設定が不足しています")
		return
	}
	userID, err := s.requireUserID(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "認証が必要です")
		return
	}
	var payload struct {
		SessionID string `json:"session_id"`
	}
	if err := decodeJSON(r, &payload); err != nil || strings.TrimSpace(payload.SessionID) == "" {
		writeError(w, http.StatusBadRequest, "セッション ID が不正です")
		return
	}
	imported, err := s.Picker.ImportSession(r.Context(), userID, payload.SessionID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Picker からの取り込みに失敗しました")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"imported": imported})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST のみ対応しています")
		return
	}
	userID, err := s.requireUserID(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "認証が必要です")
		return
	}
	var payload struct {
		Query   string `json:"query"`
		Limit   int    `json:"limit"`
		Filters struct {
			From     string `json:"from"`
			To       string `json:"to"`
			Location string `json:"location"`
		} `json:"filters"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "入力が不正です")
		return
	}
	query := strings.TrimSpace(payload.Query)
	if query == "" {
		writeError(w, http.StatusBadRequest, "検索クエリが空です")
		return
	}
	if s.Config.MaxQueryLength > 0 && len([]rune(query)) > s.Config.MaxQueryLength {
		writeError(w, http.StatusBadRequest, "検索クエリが長すぎます")
		return
	}
	if payload.Limit <= 0 {
		writeError(w, http.StatusBadRequest, "limit は 1 以上にしてください")
		return
	}
	if s.Config.MaxSearchLimit > 0 && payload.Limit > s.Config.MaxSearchLimit {
		writeError(w, http.StatusBadRequest, "limit が上限を超えています")
		return
	}
	if s.Config.MaxLocationLength > 0 && len([]rune(strings.TrimSpace(payload.Filters.Location))) > s.Config.MaxLocationLength {
		writeError(w, http.StatusBadRequest, "場所フィルタが長すぎます")
		return
	}
	var fromPtr *time.Time
	if payload.Filters.From != "" {
		parsed, err := time.Parse("2006-01-02", payload.Filters.From)
		if err != nil {
			writeError(w, http.StatusBadRequest, "開始日の形式が不正です")
			return
		}
		fromPtr = &parsed
	}
	var toPtr *time.Time
	if payload.Filters.To != "" {
		parsed, err := time.Parse("2006-01-02", payload.Filters.To)
		if err != nil {
			writeError(w, http.StatusBadRequest, "終了日の形式が不正です")
			return
		}
		toPtr = &parsed
	}
	output, err := s.Search.Search(r.Context(), userID, services.SearchInput{
		Query:    query,
		Limit:    payload.Limit,
		From:     fromPtr,
		To:       toPtr,
		Location: payload.Filters.Location,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "検索に失敗しました")
		return
	}
	writeJSON(w, http.StatusOK, output)
}

func (s *Server) handlePhotoRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/photos/")
	if strings.HasSuffix(path, "/thumbnail") {
		id := strings.TrimSuffix(path, "/thumbnail")
		s.handlePhotoThumbnail(w, r, id)
		return
	}
	s.handlePhotoDetail(w, r, path)
}

func (s *Server) handlePhotoDetail(w http.ResponseWriter, r *http.Request, id string) {
	userID, err := s.requireUserID(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "認証が必要です")
		return
	}
	photoID, err := uuid.Parse(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "ID が不正です")
		return
	}
	photo, err := s.Store.GetPhotoByID(r.Context(), userID, photoID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "写真が見つかりません")
			return
		}
		writeError(w, http.StatusInternalServerError, "写真の取得に失敗しました")
		return
	}
	response := struct {
		ID          uuid.UUID  `json:"id"`
		BaseURL     string     `json:"base_url"`
		CreatedTime *time.Time `json:"created_time"`
		Location    *string    `json:"location"`
		PeopleCount *int       `json:"people_count"`
		Caption     *string    `json:"caption"`
	}{
		ID:          photo.ID,
		BaseURL:     photo.BaseURL,
		CreatedTime: photo.CreatedTime,
		Location:    photo.Location,
		PeopleCount: photo.PeopleCount,
		Caption:     photo.Caption,
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handlePhotoThumbnail(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET のみ対応しています")
		return
	}
	userID, err := s.requireUserID(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "認証が必要です")
		return
	}
	photoID, err := uuid.Parse(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "ID が不正です")
		return
	}
	photo, err := s.Store.GetPhotoByID(r.Context(), userID, photoID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "写真が見つかりません")
			return
		}
		writeError(w, http.StatusInternalServerError, "写真の取得に失敗しました")
		return
	}
	tokens, err := s.Auth.EnsureAccessToken(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "アクセストークンの取得に失敗しました")
		return
	}
	thumbURL := buildThumbnailURL(photo.BaseURL)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, thumbURL, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "サムネイル取得に失敗しました")
		return
	}
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	resp, err := s.Auth.Client.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "サムネイル取得に失敗しました")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		writeError(w, http.StatusBadGateway, "サムネイル取得に失敗しました")
		return
	}
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.WriteHeader(http.StatusOK)
	io.Copy(w, resp.Body)
}

func buildThumbnailURL(base string) string {
	if strings.Contains(base, "googleusercontent") || strings.Contains(base, "photoslibrary") || strings.Contains(base, "googleapis") {
		return base + "=w600-h600"
	}
	return base
}

func (s *Server) requireUserID(r *http.Request) (uuid.UUID, error) {
	value, err := s.Session.ParseUserID(r)
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(value)
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && originAllowed(origin, s.Config.AllowedCORSOrigins) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.NewString()
			r.Header.Set("X-Request-ID", requestID)
		}
		w.Header().Set("X-Request-ID", requestID)
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		duration := time.Since(start)
		log.Printf("{\"request_id\":\"%s\",\"method\":\"%s\",\"path\":\"%s\",\"status\":%d,\"duration_ms\":%d}", requestID, r.Method, r.URL.Path, recorder.status, duration.Milliseconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.status = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func originAllowed(origin string, allowed []string) bool {
	for _, item := range allowed {
		if strings.TrimSpace(item) == origin {
			return true
		}
	}
	return false
}

func decodeJSON(r *http.Request, dest any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(dest)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
