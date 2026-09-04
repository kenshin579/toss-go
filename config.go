package toss

import (
	"net/http"
	"time"
)

type clientOptions struct {
	baseURL    string
	timeout    time.Duration
	httpClient *http.Client
}

// Option 은 NewClient 의 functional option.
type Option func(*clientOptions)

// WithBaseURL 은 API 베이스 URL 을 지정한다(테스트/프록시용). 기본 https://openapi.tossinvest.com.
func WithBaseURL(u string) Option { return func(o *clientOptions) { o.baseURL = u } }

// WithTimeout 은 HTTP 타임아웃을 지정한다(기본 30s). WithHTTPClient 를 쓰면 무시된다.
func WithTimeout(d time.Duration) Option { return func(o *clientOptions) { o.timeout = d } }

// WithHTTPClient 는 사용자 정의 *http.Client 를 주입한다(토큰 발급·API 호출 모두 사용).
func WithHTTPClient(c *http.Client) Option { return func(o *clientOptions) { o.httpClient = c } }
