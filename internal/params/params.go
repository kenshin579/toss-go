// Package params 는 쿼리 파라미터 조립과 필수값 검증 헬퍼다. zero-value 는 생략한다.
package params

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kenshin579/toss-go/tosstypes"
)

// Require 는 필수 문자열이 비어 있으면 에러를 돌려준다. 값은 그대로 전송되므로 공백 트리밍은 하지 않는다.
func Require(name, v string) error {
	if v == "" {
		return fmt.Errorf("toss: %s must not be empty", name)
	}
	return nil
}

// symbolPattern 은 종목 심볼 규칙(openapi components.parameters.Symbol): 영문 대/소문자, 숫자, '.', '-'.
var symbolPattern = regexp.MustCompile(`^[A-Za-z0-9.\-]+$`)

// indicatorSymbolPattern 은 시장 지표 심볼 규칙(GET /market-indicators/*): 영문 대/소문자, 숫자, '_' (예: KOSPI, KR_BOND_10Y).
var indicatorSymbolPattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// MaxSymbols 는 symbols= 쿼리에 넣을 수 있는 최대 심볼 수(openapi: 최대 200개).
const MaxSymbols = 200

func matchSymbol(re *regexp.Regexp, allowed, v string) error {
	if !re.MatchString(v) {
		return fmt.Errorf("toss: invalid symbol %q (allowed: %s)", v, allowed)
	}
	return nil
}

// Symbol 은 종목 심볼 형식을 검증한다(빈 값·공백·허용 외 문자 거부). 요청 전에 실패시켜 rate limit 을 아낀다.
func Symbol(v string) error { return matchSymbol(symbolPattern, "A-Z a-z 0-9 . -", v) }

// IndicatorSymbol 은 시장 지표 심볼 형식을 검증한다. 종목 심볼과 알파벳이 다르다('_' 허용, '.'/'-' 불허).
func IndicatorSymbol(v string) error { return matchSymbol(indicatorSymbolPattern, "A-Z a-z 0-9 _", v) }

func joinSymbols(symbols []string, validate func(string) error) (string, error) {
	if len(symbols) == 0 {
		return "", errors.New("toss: symbols must not be empty")
	}
	if len(symbols) > MaxSymbols {
		return "", fmt.Errorf("toss: too many symbols %d (max %d)", len(symbols), MaxSymbols)
	}
	for _, s := range symbols {
		if err := validate(s); err != nil {
			return "", err
		}
	}
	return strings.Join(symbols, ","), nil
}

// Symbols 는 종목 symbols= 쿼리 값을 만든다. 빈 목록, 형식 위반 원소, MaxSymbols 초과를 거부한다.
func Symbols(symbols []string) (string, error) { return joinSymbols(symbols, Symbol) }

// IndicatorSymbols 는 시장 지표 symbols= 쿼리 값을 만든다(규칙은 IndicatorSymbol).
func IndicatorSymbols(symbols []string) (string, error) { return joinSymbols(symbols, IndicatorSymbol) }

// AccountSeq 는 계좌 일련번호가 유효한지 검증한다. 0 이하면 계좌 헤더가 실리지 않아
// 서버가 account-header-required 를 돌려주므로 요청 전에 실패시킨다.
func AccountSeq(seq int64) error {
	if seq <= 0 {
		return fmt.Errorf("toss: accountSeq must be positive (got %d) — Accounts 로 조회한 값을 사용한다", seq)
	}
	return nil
}

// clientOrderIDPattern 은 clientOrderId 형식 규칙(영숫자와 -, _).
var clientOrderIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// MaxClientOrderIDLen 은 clientOrderId 최대 길이(토스 규칙).
const MaxClientOrderIDLen = 36

// ClientOrderIDFormat 은 clientOrderId 형식을 검증한다(1~36자, 영숫자와 -, _). 빈 값은 호출 측이 처리한다 —
// order/conditionalorder 는 빈 값을 "멱등성 미적용"으로 허용하고, 루트 toss.ValidateClientOrderID 는
// 필수로 거부한다. 세 곳(루트·order·conditionalorder)이 이 함수로 검증 규칙과 메시지를 공유한다.
func ClientOrderIDFormat(id string) error {
	if !clientOrderIDPattern.MatchString(id) {
		return fmt.Errorf("toss: invalid clientOrderId %q (allowed: A-Z a-z 0-9 - _)", id)
	}
	if len(id) > MaxClientOrderIDLen {
		return fmt.Errorf("toss: clientOrderId too long: %d chars (max %d)", len(id), MaxClientOrderIDLen)
	}
	return nil
}

// Str 은 s 가 비어 있지 않으면 설정한다.
func Str(v url.Values, key, s string) {
	if s != "" {
		v.Set(key, s)
	}
}

// Int 는 n > 0 이면 설정한다. 스펙상 모든 integer 파라미터는 minimum 1 이라 0 은 "미지정" 으로 안전하다.
func Int(v url.Values, key string, n int) {
	if n > 0 {
		v.Set(key, strconv.Itoa(n))
	}
}

// Bool 은 b 가 nil 이 아니면 설정한다(false 도 명시 전송).
func Bool(v url.Values, key string, b *bool) {
	if b != nil {
		v.Set(key, strconv.FormatBool(*b))
	}
}

// Time 은 t 가 nil 이 아니면 RFC3339 로 설정한다. `+` 는 url.Values 인코딩이 `%2B` 로 처리한다.
func Time(v url.Values, key string, t *time.Time) {
	if t != nil {
		v.Set(key, t.Format(time.RFC3339))
	}
}

// Date 는 d 가 비어 있지 않으면 설정한다.
func Date(v url.Values, key string, d tosstypes.Date) {
	if !d.IsZero() {
		v.Set(key, d.String())
	}
}
