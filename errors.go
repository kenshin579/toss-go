package toss

import (
	"errors"

	"github.com/kenshin579/toss-go/internal/auth"
	"github.com/kenshin579/toss-go/internal/httpclient"
)

// APIError 는 토스 API 의 4xx/5xx 응답. errors.As 로 StatusCode/Code/RequestID/Data/RetryAfter 에 접근한다.
// Code 는 unknown 값을 허용하므로 문자열 비교로 판별한다(IsCode 참고).
type APIError = httpclient.APIError

// AuthError 는 토큰 발급(POST /oauth2/token) 실패. OAuth2 형식(error/error_description)을 담는다.
type AuthError = auth.Error

// IsCode 는 err 가 주어진 토스 에러 코드(예: "stock-not-found", "expired-token")의 *APIError 인지 판별한다.
func IsCode(err error, code string) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Code == code
}
