// Package httpclient 는 토스 Open API REST 호출의 단일 통로다.
// Bearer 토큰 주입, `{"result": ...}` 봉투 해제, `{"error": {...}}` → APIError 매핑,
// 401 토큰 오류 시 1회 재발급·재시도를 담당한다. 재시도(429/5xx)·스로틀링·캐싱은 하지 않는다.
package httpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL 은 토스 Open API 서버.
const DefaultBaseURL = "https://openapi.tossinvest.com"

// DefaultTimeout 은 기본 HTTP 타임아웃.
const DefaultTimeout = 30 * time.Second

// maxErrorBody 는 봉투가 아닌 에러 바디를 Message 에 담을 때의 최대 길이.
const maxErrorBody = 200

// TokenProvider 는 access token 공급자(internal/auth.TokenSource 가 구현).
type TokenProvider interface {
	Token(ctx context.Context) (string, error)
	Invalidate(stale string)
}

// APIError 는 토스 API 의 4xx/5xx 응답이다. Code 는 flat string 이며 unknown 값을 허용한다.
type APIError struct {
	StatusCode int
	RequestID  string         // 바디 requestId, 없으면 응답 헤더 X-Request-Id
	Code       string         // 예: stock-not-found, invalid-token. 봉투가 아닌 바디면 빈 값
	Message    string         // 토스 메시지(빈 값일 수 있음) 또는 봉투가 아닌 바디 앞 200자
	Data       map[string]any // 해결 힌트(에러 코드별 서브셋). 없으면 nil
	RetryAfter time.Duration  // 429 의 Retry-After 헤더. 없으면 0
}

func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "toss: %d", e.StatusCode)
	if e.Code != "" {
		b.WriteString(" " + e.Code)
	}
	if e.Message != "" {
		b.WriteString(": " + e.Message)
	}
	if e.RequestID != "" {
		fmt.Fprintf(&b, " (requestId=%s)", e.RequestID)
	}
	return b.String()
}

// Config 는 Client 생성 인자.
type Config struct {
	BaseURL    string        // 빈 값이면 DefaultBaseURL
	Timeout    time.Duration // HTTPClient 가 nil 일 때 적용. 0 이면 DefaultTimeout
	HTTPClient *http.Client  // nil 이면 Timeout 적용 기본 클라이언트
	Tokens     TokenProvider // 필수
}

// Client 는 토스 REST HTTP 계층.
type Client struct {
	baseURL string
	http    *http.Client
	tokens  TokenProvider
}

// New 는 Config 로 Client 를 만든다.
func New(cfg Config) *Client {
	base := cfg.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	hc := cfg.HTTPClient
	if hc == nil {
		timeout := cfg.Timeout
		if timeout == 0 {
			timeout = DefaultTimeout
		}
		hc = &http.Client{Timeout: timeout}
	}
	return &Client{baseURL: strings.TrimRight(base, "/"), http: hc, tokens: cfg.Tokens}
}

type resultEnvelope struct {
	Result json.RawMessage `json:"result"`
}

type errorEnvelope struct {
	Error *struct {
		RequestID string         `json:"requestId"`
		Code      string         `json:"code"`
		Message   string         `json:"message"`
		Data      map[string]any `json:"data"`
	} `json:"error"`
}

// Get 은 GET {baseURL}{path}?{query} 를 호출해 `result` 를 out 으로 디코딩한다.
// out 이 nil 이면 바디를 버린다. 4xx/5xx 는 *APIError.
// 401 이고 code 가 expired-token / invalid-token 이면 토큰을 무효화하고 정확히 1회 재시도한다.
func (c *Client) Get(ctx context.Context, path string, query url.Values, out any) error {
	body, err := c.do(ctx, path, query, false)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	var env resultEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("toss: decode envelope %s: %w", path, err)
	}
	if len(env.Result) == 0 || string(env.Result) == "null" {
		return nil
	}
	if err := json.Unmarshal(env.Result, out); err != nil {
		return fmt.Errorf("toss: decode result %s: %w", path, err)
	}
	return nil
}

func (c *Client) do(ctx context.Context, path string, query url.Values, retried bool) ([]byte, error) {
	tok, err := c.tokens.Token(ctx)
	if err != nil {
		return nil, err
	}
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("toss: GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("toss: read %s: %w", path, err)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return body, nil
	}

	apiErr := parseError(resp, body)
	if resp.StatusCode == http.StatusUnauthorized && !retried && isTokenError(apiErr.Code) {
		c.tokens.Invalidate(tok)
		return c.do(ctx, path, query, true)
	}
	return nil, apiErr
}

func isTokenError(code string) bool {
	return code == "expired-token" || code == "invalid-token"
}

func parseError(resp *http.Response, body []byte) *APIError {
	e := &APIError{StatusCode: resp.StatusCode, RequestID: resp.Header.Get("X-Request-Id")}
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil {
			e.RetryAfter = time.Duration(secs) * time.Second
		}
	}
	var env errorEnvelope
	if json.Unmarshal(body, &env) == nil && env.Error != nil {
		if env.Error.RequestID != "" {
			e.RequestID = env.Error.RequestID
		}
		e.Code, e.Message, e.Data = env.Error.Code, env.Error.Message, env.Error.Data
		return e
	}
	msg := strings.TrimSpace(string(body))
	if len(msg) > maxErrorBody {
		msg = msg[:maxErrorBody]
	}
	e.Message = msg
	return e
}
