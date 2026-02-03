package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type SessionManager struct {
	Secret      []byte
	TTL         time.Duration
	CookieName  string
	StateCookie string
	Secure      bool
}

func NewSessionManager(secret string, ttl time.Duration, secure bool) *SessionManager {
	return &SessionManager{
		Secret:      []byte(secret),
		TTL:         ttl,
		CookieName:  "session",
		StateCookie: "oauth_state",
		Secure:      secure,
	}
}

func (s *SessionManager) NewState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (s *SessionManager) CreateStateCookie(state string) *http.Cookie {
	value := s.signPayload(state)
	return &http.Cookie{
		Name:     s.StateCookie,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.Secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(15 * time.Minute),
	}
}

func (s *SessionManager) VerifyState(r *http.Request, state string) bool {
	cookie, err := r.Cookie(s.StateCookie)
	if err != nil {
		return false
	}
	payload, ok := s.verifyPayload(cookie.Value)
	if !ok {
		return false
	}
	return payload == state
}

func (s *SessionManager) CreateSessionCookie(userID string) *http.Cookie {
	expiry := time.Now().Add(s.TTL)
	payload := userID + "|" + strconv.FormatInt(expiry.Unix(), 10)
	value := s.signPayload(payload)
	return &http.Cookie{
		Name:     s.CookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.Secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiry,
	}
}

func (s *SessionManager) ClearSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     s.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.Secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	}
}

func (s *SessionManager) ParseUserID(r *http.Request) (string, error) {
	cookie, err := r.Cookie(s.CookieName)
	if err != nil {
		return "", err
	}
	payload, ok := s.verifyPayload(cookie.Value)
	if !ok {
		return "", errors.New("セッション署名が不正です")
	}
	parts := strings.Split(payload, "|")
	if len(parts) != 2 {
		return "", errors.New("セッション形式が不正です")
	}
	expiry, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", err
	}
	if time.Now().After(time.Unix(expiry, 0)) {
		return "", errors.New("セッション期限切れです")
	}
	return parts[0], nil
}

func (s *SessionManager) signPayload(payload string) string {
	mac := hmac.New(sha256.New, s.Secret)
	mac.Write([]byte(payload))
	sig := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func (s *SessionManager) verifyPayload(value string) (string, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return "", false
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", false
	}
	mac := hmac.New(sha256.New, s.Secret)
	mac.Write(payloadBytes)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return "", false
	}
	return string(payloadBytes), true
}
