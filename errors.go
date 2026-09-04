package toss

import (
	"errors"

	"github.com/kenshin579/toss-go/internal/auth"
	"github.com/kenshin579/toss-go/internal/httpclient"
)

// APIError 는 토스 API 의 4xx/5xx 응답이다. errors.As 로 아래 필드에 접근한다.
//
//	StatusCode int            // HTTP 상태코드
//	RequestID  string         // 응답 requestId(없으면 X-Request-Id 헤더)
//	Code       string         // 토스 에러 코드(unknown 값 허용, IsCode 로 비교)
//	Message    string         // 사용자 노출용 메시지(빈 문자열일 수 있음)
//	Data       map[string]any // 에러 해결 힌트(코드별로 다름, 없으면 nil)
//	RetryAfter time.Duration  // 429 의 Retry-After. 그 외에는 0
type APIError = httpclient.APIError

// AuthError 는 토큰 발급(POST /oauth2/token) 실패다. 공통 봉투가 아니라 OAuth2 표준 형식으로 내려온다.
//
//	StatusCode  int    // HTTP 상태코드(예: 403)
//	Code        string // OAuth2 error(예: access_denied)
//	Description string // OAuth2 error_description(예: IP address not allowed)
type AuthError = auth.Error

// IsCode 는 err 가 주어진 토스 에러 코드(예: "stock-not-found", "expired-token")의 *APIError 인지 판별한다.
func IsCode(err error, code string) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Code == code
}
