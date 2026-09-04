package toss

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
)

// MaxClientOrderIDLen 은 clientOrderId 최대 길이(토스 규칙).
const MaxClientOrderIDLen = 36

var clientOrderIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// NewClientOrderID 는 멱등성 키로 쓸 새 clientOrderId 를 만든다(32자, URL-safe).
//
// 주문 생성/조건주문 생성에 이 값을 넣으면 (1) 같은 값으로 재요청할 때 토스가 이전 주문 결과를
// 그대로 돌려주고(10분 유효), (2) SDK 가 401 토큰 오류에 요청을 1회 재시도한다.
// 키가 없으면 SDK 는 쓰기 요청을 재시도하지 않는다 — 중복 주문을 만들지 않기 위해서다.
func NewClientOrderID() string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("toss: crypto/rand failed: " + err.Error()) // 이 실패는 프로세스가 정상 동작할 수 없는 상태다
	}
	return base64.RawURLEncoding.EncodeToString(b[:]) // 24바이트 → 32자
}

// ValidateClientOrderID 는 clientOrderId 형식을 검증한다(1~36자, 영숫자와 -, _).
func ValidateClientOrderID(id string) error {
	if id == "" {
		return fmt.Errorf("toss: clientOrderId must not be empty")
	}
	if len(id) > MaxClientOrderIDLen {
		return fmt.Errorf("toss: clientOrderId too long: %d chars (max %d)", len(id), MaxClientOrderIDLen)
	}
	if !clientOrderIDPattern.MatchString(id) {
		return fmt.Errorf("toss: invalid clientOrderId %q (allowed: A-Z a-z 0-9 - _)", strings.TrimSpace(id))
	}
	return nil
}
