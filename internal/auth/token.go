// Package auth 는 토스 Open API 의 OAuth2 Client Credentials 토큰을 발급하고 메모리에 캐시한다.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// refreshMargin 은 만료 이 시간 전부터 새 토큰을 발급한다.
const refreshMargin = 60 * time.Second

// maxErrorBody 는 비-JSON 에러 바디를 Description 에 담을 때의 최대 길이.
const maxErrorBody = 200

// Error 는 토큰 엔드포인트의 실패 응답. 토스는 공통 봉투가 아니라 OAuth2 표준 형식
// ({"error","error_description"})으로 응답한다.
type Error struct {
	StatusCode  int
	Code        string // OAuth2 `error` (예: access_denied). 바디가 그 형식이 아니면 빈 값
	Description string // OAuth2 `error_description`, 또는 바디 앞부분
}

func (e *Error) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("toss: token request failed (status %d): %s", e.StatusCode, e.Description)
	}
	return fmt.Sprintf("toss: token request failed (status %d): %s: %s", e.StatusCode, e.Code, e.Description)
}

// TokenSource 는 access token 을 발급·캐시한다. 동시 호출에 안전하다.
type TokenSource struct {
	clientID     string
	clientSecret string
	tokenURL     string
	http         *http.Client
	now          func() time.Time

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

// New 는 TokenSource 를 만든다. baseURL 은 `https://openapi.tossinvest.com` 형태(끝 슬래시 없음).
func New(clientID, clientSecret, baseURL string, hc *http.Client) *TokenSource {
	return &TokenSource{
		clientID:     clientID,
		clientSecret: clientSecret,
		tokenURL:     strings.TrimRight(baseURL, "/") + "/oauth2/token",
		http:         hc,
		now:          time.Now,
	}
}

// Token 은 유효한 access token 을 돌려준다. 캐시가 없거나 만료 60초 이내면 새로 발급한다.
func (s *TokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && s.now().Before(s.expiresAt.Add(-refreshMargin)) {
		return s.token, nil
	}
	tok, expiresIn, err := s.issue(ctx)
	if err != nil {
		return "", err
	}
	s.token = tok
	s.expiresAt = s.now().Add(time.Duration(expiresIn) * time.Second)
	return tok, nil
}

// Invalidate 는 캐시를 비운다. API 가 401 토큰 오류를 돌려줬을 때 호출한다.
func (s *TokenSource) Invalidate() {
	s.mu.Lock()
	s.token = ""
	s.expiresAt = time.Time{}
	s.mu.Unlock()
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

type oauthError struct {
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

func (s *TokenSource) issue(ctx context.Context) (string, int64, error) {
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {s.clientID},
		"client_secret": {s.clientSecret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.http.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("toss: token request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("toss: read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		e := &Error{StatusCode: resp.StatusCode}
		var oe oauthError
		if json.Unmarshal(body, &oe) == nil && oe.Error != "" {
			e.Code, e.Description = oe.Error, oe.Description
		} else {
			e.Description = truncate(strings.TrimSpace(string(body)), maxErrorBody)
		}
		return "", 0, e
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", 0, fmt.Errorf("toss: decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", 0, fmt.Errorf("toss: token response has empty access_token")
	}
	return tr.AccessToken, tr.ExpiresIn, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
