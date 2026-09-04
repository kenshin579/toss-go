// Package httpclient 는 토스 Open API REST 호출의 단일 통로다.
// Bearer 토큰 주입, `{"result": ...}` 봉투 해제, `{"error": {...}}` → APIError 매핑,
// 401 토큰 오류 시 1회 재발급·재시도를 담당한다. 재시도(429/5xx)·스로틀링·캐싱은 하지 않는다.
package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kenshin579/toss-go/internal/strutil"
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

// Request 는 한 번의 HTTP 호출을 기술한다.
type Request struct {
	Method     string     // GET/POST/DELETE. 빈 값이면 GET
	Path       string     // 예: /api/v1/orders
	Query      url.Values // nil 가능
	Body       any        // nil 이 아니면 JSON 으로 직렬화해 전송
	AccountSeq int64      // 0 이 아니면 X-Tossinvest-Account 헤더
	Idempotent bool       // true 면 401 토큰 오류에 1회 재시도. 쓰기는 멱등성 키가 있을 때만 true
	Out        any        // nil 이면 응답 본문을 읽지 않는다(204 포함)
}

// Do 는 Request 를 실행하고 `result` 를 Out 으로 디코딩한다.
// Out 이 nil 이면 본문을 버린다(204 정상). Out 이 nil 이 아닌데 본문이 없거나 result 가 null 이면 에러.
// 4xx/5xx 는 *APIError. 401 이고 code 가 expired-token / invalid-token 이면 토큰을 무효화하고,
// Idempotent 인 요청만 정확히 1회 재시도한다(멱등성 키 없는 쓰기의 중복 실행 방지).
func (c *Client) Do(ctx context.Context, r Request) error {
	if r.Method == "" {
		r.Method = http.MethodGet
	}
	var payload []byte
	if r.Body != nil {
		b, err := json.Marshal(r.Body)
		if err != nil {
			return fmt.Errorf("toss: encode body %s: %w", r.Path, err)
		}
		payload = b
	}
	body, err := c.do(ctx, r, payload, false)
	if err != nil {
		return err
	}
	if r.Out == nil {
		return nil
	}
	if len(body) == 0 {
		return fmt.Errorf("toss: %s: response has no result", r.Path)
	}
	var env resultEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("toss: decode envelope %s: %w", r.Path, err)
	}
	if len(env.Result) == 0 || string(env.Result) == "null" {
		return fmt.Errorf("toss: %s: response has no result", r.Path)
	}
	if err := json.Unmarshal(env.Result, r.Out); err != nil {
		return fmt.Errorf("toss: decode result %s: %w", r.Path, err)
	}
	return nil
}

// Get 은 GET 조회 단축형이다. GET 은 항상 멱등이므로 401 재시도가 적용된다.
func (c *Client) Get(ctx context.Context, path string, query url.Values, out any) error {
	return c.Do(ctx, Request{Method: http.MethodGet, Path: path, Query: query, Out: out, Idempotent: true})
}

func (c *Client) do(ctx context.Context, r Request, payload []byte, retried bool) ([]byte, error) {
	tok, err := c.tokens.Token(ctx)
	if err != nil {
		return nil, err
	}
	u := c.baseURL + r.Path
	if len(r.Query) > 0 {
		u += "?" + r.Query.Encode()
	}
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload) // 재시도 때 다시 읽을 수 있도록 매번 새 Reader
	}
	req, err := http.NewRequestWithContext(ctx, r.Method, u, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if r.AccountSeq != 0 {
		req.Header.Set("X-Tossinvest-Account", strconv.FormatInt(r.AccountSeq, 10))
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("toss: %s %s: %w", r.Method, r.Path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("toss: read %s: %w", r.Path, err)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return body, nil
	}

	apiErr := parseError(resp, body)
	if resp.StatusCode == http.StatusUnauthorized && isTokenError(apiErr.Code) {
		// 서버가 거부한 토큰은 재시도 여부와 무관하게 캐시에서 제거한다(재시도 후 401 이어도 하루 동안 남지 않도록).
		c.tokens.Invalidate(tok)
		if !retried && r.Idempotent {
			return c.do(ctx, r, payload, true)
		}
	}
	return nil, apiErr
}

func isTokenError(code string) bool {
	return code == "expired-token" || code == "invalid-token"
}

func parseError(resp *http.Response, body []byte) *APIError {
	e := &APIError{StatusCode: resp.StatusCode, RequestID: resp.Header.Get("X-Request-Id")}
	e.RetryAfter = parseRetryAfter(resp.Header.Get("Retry-After"))
	var env errorEnvelope
	if json.Unmarshal(body, &env) == nil && env.Error != nil {
		if env.Error.RequestID != "" {
			e.RequestID = env.Error.RequestID
		}
		e.Code, e.Message, e.Data = env.Error.Code, env.Error.Message, env.Error.Data
		return e
	}
	e.Message = strutil.Truncate(string(body), maxErrorBody)
	return e
}

// parseRetryAfter 는 Retry-After 헤더(초 또는 HTTP-date)를 Duration 으로 바꾼다. 없거나 해석 불가면 0.
func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}
