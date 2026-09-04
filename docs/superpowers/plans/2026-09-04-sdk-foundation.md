# toss-go SDK 기반 + 조회 API (v0.1.0) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 토스증권 Open API 의 Go 클라이언트 — OAuth2 토큰 자동 관리 + `{result}` 봉투 해제 + 에러 매핑을 갖춘 기반 위에 조회 21 ops(시세 5·종목 8·시장정보 3·랭킹 1·지표 3)를 구현하고 v0.1.0 으로 릴리스한다.

**Architecture:** 루트 `toss.Client` 가 `internal/auth.TokenSource`(토큰 발급·메모리 캐시)와 `internal/httpclient.Client`(Bearer 주입·봉투 해제·`APIError` 매핑·401 토큰오류 1회 재시도)를 소유하고, OpenAPI tag 별 패키지(`marketdata`, `stockinfo`, `marketinfo`, `ranking`, `indicators`)를 필드로 노출한다. 공용 enum·`Date` 는 `tosstypes`. 수치는 `shopspring/decimal`, null 가능 필드는 포인터. 응답 타입은 2026-09-04 캡처한 실응답 fixture 로 검증한다.

**Tech Stack:** Go 1.25 (로컬 1.26), `github.com/shopspring/decimal`, 표준 `net/http`·`httptest`. 외부 의존성은 decimal 하나.

**Spec:** `docs/superpowers/specs/2026-09-04-sdk-foundation-design.md`
**Branch:** `feature/sdk-foundation` (스펙·fixture 커밋 완료: 7b0a9e4, dd89c17)
**Reference:** `../fmp-go` (동일 구조의 형제 라이브러리), `docs/api/openapi.json` (스키마 정본)

---

## 파일 구조

| 경로 | 책임 |
|---|---|
| `go.mod` | module `github.com/kenshin579/toss-go`, go 1.25, dep decimal |
| `tosstypes/types.go` (+`_test.go`) | `Date`, `Currency`, `Market`, `SecurityType`, `StockStatus`, `Interval`, `IndicatorInterval`, `RankingType`, `RankingDuration`, `MarketCountry`, `RateChangeType`, `WarningType` |
| `internal/auth/token.go` (+`_test.go`) | `TokenSource` — 발급·캐시·`Invalidate`, `Error`(OAuth2 에러 형식) |
| `internal/httpclient/client.go` (+`_test.go`) | `Client.Get` — Bearer, 봉투 해제, `APIError`, 401 재시도, 429 RetryAfter |
| `internal/strutil/strutil.go` (+`_test.go`) | 에러 메시지용 rune-safe Truncate (auth·httpclient 공용) |
| `internal/testutil/server.go` | 테스트용 `httptest` 서버 + 고정 토큰 `httpclient.Client` 생성 |
| `internal/params/params.go` (+`_test.go`) | 쿼리 조립(zero-value 생략, RFC3339) + 필수값/`Symbol`·`Symbols`(최대 200개, 형식 검증) 헬퍼 |
| `internal/fetch/fetch.go` (+`_test.go`) | `One`/`List` 제네릭 조회 헬퍼 — `httpclient.Get` 호출과 결과 포인터/슬라이스 반환만 담당 |
| `client.go` / `config.go` / `from_env.go` / `errors.go` (+`client_test.go`) | 루트 진입점, Option, env, 에러 재수출·`IsCode` |
| `marketdata/` | `client.go`, `prices.go`, `orderbook.go`, `trades.go`, `price_limits.go`, `candles.go`, 테스트, `testdata/` |
| `stockinfo/` | `client.go`, `stocks.go`, `warnings.go`, `trend.go`(5종 매매동향 + 제네릭 페이지), 테스트, `testdata/` |
| `marketinfo/` | `client.go`, `exchange_rate.go`, `calendar.go`, 테스트, `testdata/` |
| `ranking/` | `client.go`, `rankings.go`, 테스트, `testdata/` |
| `indicators/` | `client.go`, `prices.go`, `candles.go`, `investor_trading.go`, 테스트, `testdata/` |
| `examples/basic/main.go` | 시세·캔들 조회 예시 |
| `integration_test.go` | `-tags integration`, 자격 증명 있을 때만 |
| `scripts/release.sh` | fmp-go 복사본 |
| `README.md` | 설치·인증·사용·커버리지 |

캡처된 fixture 는 `testdata/captured/*.json` 에 있다(봉투 포함 `{"result": ...}` 원문). 각 태스크에서 해당 패키지 `testdata/` 로 `git mv` 한다. 없는 fixture 는 아래 공통 명령으로 캡처한다. 허용 IP 가 등록되지 않아 토큰이 403 이면 각 태스크의 캡처 스텝이 `openapi.json` 의 응답 예시(`examples`)로 자동 대체한다 — 예시도 토스가 작성한 실제 구조라 디코딩 테스트에는 충분하며, 나중에 허용 IP 에서 재캡처해 교체할 수 있다:

```bash
# 공통: 토큰 발급 → $TOKEN. 각 태스크의 캡처 스텝에서 이 3줄을 먼저 실행한다.
eval "$(grep -E '^export TOSS_CLIENT_(ID|SECRET)=' ~/.zshrc)"
TOKEN=$(curl -s --compressed -X POST https://openapi.tossinvest.com/oauth2/token -H 'Content-Type: application/x-www-form-urlencoded' -d grant_type=client_credentials -d "client_id=$TOSS_CLIENT_ID" -d "client_secret=$TOSS_CLIENT_SECRET" | jq -r .access_token)
if [ -n "$TOKEN" ] && [ "$TOKEN" != "null" ]; then echo TOKEN_OK; else echo "TOKEN_UNAVAILABLE → openapi.json 예시로 대체"; fi
J=docs/api/openapi.json; ex() { jq --arg p "$1" --arg n "$2" '.paths[$p].get.responses."200".content."application/json".examples[$n].value' $J; }
```

커밋 메시지는 항상 아래 트레일러로 끝낸다:
```
Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>
```

---

### Task 1: 모듈 초기화 + `tosstypes`

**Files:**
- Create: `go.mod`, `tosstypes/types.go`, `tosstypes/types_test.go`

- [ ] **Step 1: go.mod 생성 + decimal 의존성**

```bash
cd /Users/user/src/workspace_moneyflow/toss-go && git branch --show-current && go mod init github.com/kenshin579/toss-go && go mod edit -go=1.25 && go get github.com/shopspring/decimal@v1.4.0 && cat go.mod
```
Expected: 브랜치 `feature/sdk-foundation`, go.mod 에 `go 1.25` 와 `require github.com/shopspring/decimal v1.4.0`.

- [ ] **Step 2: Date 실패 테스트 작성**

```bash
mkdir -p tosstypes && cat > tosstypes/types_test.go << 'EOF'
package tosstypes

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDate_Time(t *testing.T) {
	d := Date("2026-09-03")
	got, err := d.Time()
	if err != nil {
		t.Fatalf("Time: %v", err)
	}
	if got.Year() != 2026 || got.Month() != time.September || got.Day() != 3 {
		t.Errorf("got %v", got)
	}
	if _, off := got.Zone(); off != 9*3600 {
		t.Errorf("zone offset = %d, want KST(+9h)", off)
	}
}

func TestDate_Time_Invalid(t *testing.T) {
	if _, err := Date("2026/09/03").Time(); err == nil {
		t.Error("want error for invalid format")
	}
}

func TestDate_IsZero(t *testing.T) {
	if !Date("").IsZero() {
		t.Error("empty must be zero")
	}
	if Date("2026-09-03").IsZero() {
		t.Error("non-empty must not be zero")
	}
}

func TestNewDate(t *testing.T) {
	ts := time.Date(2026, 9, 3, 23, 59, 0, 0, KST)
	if got := NewDate(ts); got != "2026-09-03" {
		t.Errorf("NewDate(KST) = %q", got)
	}
	// UTC 15:30 = KST 다음날 00:30 → KST 기준 날짜
	utc := time.Date(2026, 9, 3, 15, 30, 0, 0, time.UTC)
	if got := NewDate(utc); got != "2026-09-04" {
		t.Errorf("NewDate(UTC) = %q, want KST date", got)
	}
}

func TestDate_JSON(t *testing.T) {
	var v struct {
		D  Date  `json:"d"`
		P  *Date `json:"p"`
		N  *Date `json:"n"`
	}
	if err := json.Unmarshal([]byte(`{"d":"2026-09-03","p":"2026-09-02","n":null}`), &v); err != nil {
		t.Fatal(err)
	}
	if v.D != "2026-09-03" || v.P == nil || *v.P != "2026-09-02" || v.N != nil {
		t.Errorf("decoded %+v", v)
	}
	out, _ := json.Marshal(v.D)
	if string(out) != `"2026-09-03"` {
		t.Errorf("marshal = %s", out)
	}

	var nul struct {
		D Date `json:"d"`
	}
	if err := json.Unmarshal([]byte(`{"d":null}`), &nul); err != nil || nul.D != "" {
		t.Errorf("null into value Date: %q %v", nul.D, err)
	}
	var np *Date
	if out, _ := json.Marshal(np); string(out) != "null" {
		t.Errorf("marshal nil *Date = %s", out)
	}
}
EOF
go test ./tosstypes/ 2>&1 | head -5
```
Expected: 컴파일 에러(`undefined: Date`, `KST`, `NewDate`).

- [ ] **Step 3: tosstypes 구현**

```bash
cat > tosstypes/types.go << 'EOF'
// Package tosstypes 는 toss-go 전역에서 쓰는 공용 타입(날짜, 열거값)이다.
// 열거값은 문자열 타입 + 상수로 두며, 토스가 새 값을 추가해도 거부하지 않고 그대로 보존한다.
package tosstypes

import (
	"fmt"
	"time"
)

// KST 는 토스증권 API 의 기준 타임존(UTC+9).
var KST = time.FixedZone("KST", 9*3600)

// Date 는 `YYYY-MM-DD` 형식의 날짜 문자열이다. JSON 에서 그대로 문자열로 오간다.
type Date string

// NewDate 는 t 를 KST 로 변환한 뒤 그 날짜로 Date 를 만든다. 토스 API 의 날짜 파라미터(until, date 등)는
// 모두 KST 기준이므로 UTC 서버에서 time.Now() 를 넘겨도 어긋나지 않는다.
// 미국 현지 기준 날짜가 필요하면 Date(t.In(loc).Format("2006-01-02")) 로 직접 만든다.
func NewDate(t time.Time) Date { return Date(t.In(KST).Format("2006-01-02")) }

// String 은 원문 문자열을 돌려준다.
func (d Date) String() string { return string(d) }

// IsZero 는 빈 값 여부.
func (d Date) IsZero() bool { return d == "" }

// Time 은 KST 자정 시각으로 변환한다. 형식이 맞지 않으면 에러.
// 날짜만 의미 있는 값이므로 Year/Month/Day 용도로 쓰고, 미국 현지 기준 날짜(UsMarketDay.date 등)는
// 시각(instant) 비교에 쓰지 않는다.
func (d Date) Time() (time.Time, error) {
	t, err := time.ParseInLocation("2006-01-02", string(d), KST)
	if err != nil {
		return time.Time{}, fmt.Errorf("tosstypes: invalid date %q: %w", string(d), err)
	}
	return t, nil
}

// Currency 는 통화 코드.
type Currency string

const (
	CurrencyKRW Currency = "KRW"
	CurrencyUSD Currency = "USD"
)

// MarketCountry 는 시장 국가.
type MarketCountry string

const (
	MarketCountryKR MarketCountry = "KR"
	MarketCountryUS MarketCountry = "US"
)

// Market 은 거래소/시장 구분.
type Market string

const (
	MarketKOSPI  Market = "KOSPI"
	MarketKOSDAQ Market = "KOSDAQ"
	MarketNYSE   Market = "NYSE"
	MarketNASDAQ Market = "NASDAQ"
	MarketAMEX   Market = "AMEX"
	MarketKRETC  Market = "KR_ETC"
	MarketUSETC  Market = "US_ETC"
)

// SecurityType 은 증권 종류.
type SecurityType string

const (
	SecurityTypeStock              SecurityType = "STOCK"
	SecurityTypeForeignStock       SecurityType = "FOREIGN_STOCK"
	SecurityTypeDepositaryReceipt  SecurityType = "DEPOSITARY_RECEIPT"
	SecurityTypeInfrastructureFund SecurityType = "INFRASTRUCTURE_FUND"
	SecurityTypeREIT               SecurityType = "REIT"
	SecurityTypeETF                SecurityType = "ETF"
	SecurityTypeForeignETF         SecurityType = "FOREIGN_ETF"
	SecurityTypeETN                SecurityType = "ETN"
	SecurityTypeStockWarrants      SecurityType = "STOCK_WARRANTS"
)

// StockStatus 는 상장 상태.
type StockStatus string

const (
	StockStatusScheduled StockStatus = "SCHEDULED"
	StockStatusActive    StockStatus = "ACTIVE"
	StockStatusDelisted  StockStatus = "DELISTED"
)

// Interval 은 캔들 봉 단위.
type Interval string

const (
	Interval1m Interval = "1m"
	Interval1d Interval = "1d"
)

// IndicatorInterval 은 시장 지표 투자자별 매매대금의 집계 단위.
type IndicatorInterval string

const (
	IndicatorInterval1d  IndicatorInterval = "1d"
	IndicatorInterval1w  IndicatorInterval = "1w"
	IndicatorInterval1mo IndicatorInterval = "1mo"
	IndicatorInterval1y  IndicatorInterval = "1y"
)

// RankingType 은 랭킹 종류.
type RankingType string

const (
	RankingTypeMarketTradingAmount          RankingType = "MARKET_TRADING_AMOUNT"
	RankingTypeMarketTradingVolume          RankingType = "MARKET_TRADING_VOLUME"
	RankingTypeTopGainers                   RankingType = "TOP_GAINERS"
	RankingTypeTopLosers                    RankingType = "TOP_LOSERS"
	RankingTypeTossSecuritiesTradingAmount  RankingType = "TOSS_SECURITIES_TRADING_AMOUNT"
	RankingTypeTossSecuritiesTradingVolume  RankingType = "TOSS_SECURITIES_TRADING_VOLUME"
)

// RankingDuration 은 랭킹 집계 기간.
type RankingDuration string

const (
	RankingDurationRealtime RankingDuration = "realtime"
	RankingDuration1d       RankingDuration = "1d"
	RankingDuration1w       RankingDuration = "1w"
	RankingDuration1mo      RankingDuration = "1mo"
	RankingDuration3mo      RankingDuration = "3mo"
	RankingDuration6mo      RankingDuration = "6mo"
	RankingDuration1y       RankingDuration = "1y"
)

// RateChangeType 은 환율 변동 방향.
type RateChangeType string

const (
	RateChangeTypeUp    RateChangeType = "UP"
	RateChangeTypeEqual RateChangeType = "EQUAL"
	RateChangeTypeDown  RateChangeType = "DOWN"
)

// WarningType 은 매수 유의사항 종류.
type WarningType string

const (
	WarningTypeLiquidationTrading WarningType = "LIQUIDATION_TRADING"
	WarningTypeOverheated         WarningType = "OVERHEATED"
	WarningTypeInvestmentWarning  WarningType = "INVESTMENT_WARNING"
	WarningTypeInvestmentRisk     WarningType = "INVESTMENT_RISK"
	WarningTypeVIStaticAndDynamic WarningType = "VI_STATIC_AND_DYNAMIC"
	WarningTypeVIStatic           WarningType = "VI_STATIC"
	WarningTypeVIDynamic          WarningType = "VI_DYNAMIC"
	WarningTypeStockWarrants      WarningType = "STOCK_WARRANTS"
)
EOF
gofmt -l tosstypes; go vet ./tosstypes/ && go test ./tosstypes/ -v 2>&1 | tail -8
```
Expected: gofmt 출력 없음, `PASS` 5 tests (`TestDate_Time`, `_Invalid`, `_IsZero`, `TestNewDate`, `TestDate_JSON`).

- [ ] **Step 4: 커밋**

```bash
git add go.mod go.sum tosstypes && git commit -m "feat: 모듈 초기화 + tosstypes(Date, 열거값)

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 2: `internal/auth` — 토큰 발급·캐시

**Files:**
- Create: `internal/auth/token.go`, `internal/auth/token_test.go`

- [ ] **Step 1: 실패 테스트 작성**

```bash
mkdir -p internal/auth && cat > internal/auth/token_test.go << 'EOF'
package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

// newServer 는 토큰 엔드포인트 스텁. issued 는 발급 요청 횟수.
func newServer(t *testing.T, status int, body string, issued *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/token" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q", ct)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("client_id") != "id" || r.Form.Get("client_secret") != "sec" {
			t.Errorf("form = %v", r.Form)
		}
		atomic.AddInt32(issued, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

const okBody = `{"access_token":"tok-1","token_type":"Bearer","expires_in":86399}`

func TestToken_IssuesAndCaches(t *testing.T) {
	var issued int32
	srv := newServer(t, 200, okBody, &issued)
	defer srv.Close()
	s := New("id", "sec", srv.URL, srv.Client())

	for i := 0; i < 3; i++ {
		tok, err := s.Token(context.Background())
		if err != nil {
			t.Fatalf("Token: %v", err)
		}
		if tok != "tok-1" {
			t.Errorf("tok = %q", tok)
		}
	}
	if got := atomic.LoadInt32(&issued); got != 1 {
		t.Errorf("issued %d times, want 1", got)
	}
}

func TestToken_RefreshesNearExpiry(t *testing.T) {
	var issued int32
	srv := newServer(t, 200, okBody, &issued)
	defer srv.Close()
	s := New("id", "sec", srv.URL, srv.Client())
	now := time.Now()
	s.now = func() time.Time { return now }

	if _, err := s.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	// 만료 61초 전: 아직 유효
	s.now = func() time.Time { return now.Add(86399*time.Second - 61*time.Second) }
	if _, err := s.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&issued); got != 1 {
		t.Fatalf("issued %d, want 1 (still valid)", got)
	}
	// 만료 59초 전: 여유(60s) 안 → 재발급
	s.now = func() time.Time { return now.Add(86399*time.Second - 59*time.Second) }
	if _, err := s.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&issued); got != 2 {
		t.Errorf("issued %d, want 2 (refreshed)", got)
	}
}

func TestToken_ConcurrentIssuesOnce(t *testing.T) {
	var issued int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&issued, 1)
		<-release // 모든 goroutine 이 Token() 에 진입할 때까지 첫 발급을 붙잡아 둔다
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(okBody))
	}))
	defer srv.Close()
	s := New("id", "sec", srv.URL, srv.Client())

	const n = 100
	var started, wg sync.WaitGroup
	started.Add(n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			started.Done()
			if _, err := s.Token(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	started.Wait()
	time.Sleep(20 * time.Millisecond) // goroutine 들이 mutex 대기열에 쌓이도록
	close(release)
	wg.Wait()
	if got := atomic.LoadInt32(&issued); got != 1 {
		t.Errorf("issued %d, want 1", got)
	}
}

func TestToken_InvalidateForcesReissue(t *testing.T) {
	var issued int32
	srv := newServer(t, 200, okBody, &issued)
	defer srv.Close()
	s := New("id", "sec", srv.URL, srv.Client())
	if _, err := s.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.Invalidate("tok-1")
	if _, err := s.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&issued); got != 2 {
		t.Errorf("issued %d, want 2", got)
	}

	// 캐시된 토큰과 다른 stale 값은 무시해야 한다(다른 goroutine 이 이미 받아 둔 새 토큰을 지우지 않기 위함).
	s.Invalidate("stale-other")
	if _, err := s.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&issued); got != 2 {
		t.Errorf("issued %d, want 2 (mismatch must not clear)", got)
	}
}

func TestToken_OAuth2Error(t *testing.T) {
	var issued int32
	srv := newServer(t, 403, `{"error":"access_denied","error_description":"IP address not allowed"}`, &issued)
	defer srv.Close()
	s := New("id", "sec", srv.URL, srv.Client())
	_, err := s.Token(context.Background())
	var ae *Error
	if !errors.As(err, &ae) {
		t.Fatalf("want *Error, got %T %v", err, err)
	}
	if ae.StatusCode != 403 || ae.Code != "access_denied" || ae.Description != "IP address not allowed" {
		t.Errorf("got %+v", ae)
	}
	if got := ae.Error(); got != "toss: token request failed (status 403): access_denied: IP address not allowed" {
		t.Errorf("Error() = %q", got)
	}
}

func TestToken_NonJSONError(t *testing.T) {
	var issued int32
	srv := newServer(t, 502, `<html>bad gateway</html>`, &issued)
	defer srv.Close()
	s := New("id", "sec", srv.URL, srv.Client())
	_, err := s.Token(context.Background())
	var ae *Error
	if !errors.As(err, &ae) {
		t.Fatalf("want *Error, got %v", err)
	}
	if ae.StatusCode != 502 || ae.Code != "" || ae.Description != "<html>bad gateway</html>" {
		t.Errorf("got %+v", ae)
	}
}

func TestToken_EmptyAccessToken(t *testing.T) {
	var issued int32
	srv := newServer(t, 200, `{"access_token":"","token_type":"Bearer","expires_in":10}`, &issued)
	defer srv.Close()
	s := New("id", "sec", srv.URL, srv.Client())
	if _, err := s.Token(context.Background()); err == nil {
		t.Error("want error for empty access_token")
	}
}

func TestToken_InvalidExpiresIn(t *testing.T) {
	var issued int32
	srv := newServer(t, 200, `{"access_token":"tok","token_type":"Bearer","expires_in":0}`, &issued)
	defer srv.Close()
	s := New("id", "sec", srv.URL, srv.Client())
	if _, err := s.Token(context.Background()); err == nil || !strings.Contains(err.Error(), "expires_in") {
		t.Errorf("want expires_in error, got %v", err)
	}
}

func TestToken_ErrorBodyTruncatedOnRuneBoundary(t *testing.T) {
	var issued int32
	body := strings.Repeat("가", 100) + "\n\n" + strings.Repeat("나", 100) // 3바이트 문자 200개 = 600바이트
	srv := newServer(t, 500, body, &issued)
	defer srv.Close()
	s := New("id", "sec", srv.URL, srv.Client())
	_, err := s.Token(context.Background())
	var ae *Error
	if !errors.As(err, &ae) {
		t.Fatalf("want *Error, got %v", err)
	}
	if !utf8.ValidString(ae.Description) || len(ae.Description) > 200 || strings.Contains(ae.Description, "\n") {
		t.Errorf("Description = %q (len %d)", ae.Description, len(ae.Description))
	}
}

func TestNew_NilHTTPClientDefaults(t *testing.T) {
	s := New("id", "sec", "https://example.invalid", nil)
	if s.hc == nil {
		t.Error("nil hc must default to http.DefaultClient")
	}
}
EOF
go test ./internal/auth/ 2>&1 | head -5
```
Expected: 컴파일 에러(`undefined: New`, `Error`).

- [ ] **Step 2: 구현**

```bash
cat > internal/auth/token.go << 'EOF'
// Package auth 는 토스 Open API 의 OAuth2 Client Credentials 토큰을 발급하고 메모리에 캐시한다.
// 토스는 client 당 유효 토큰이 1개이므로, 같은 client_id 를 여러 프로세스가 쓰면 서로의 토큰을 무효화한다 — 메모리 캐시로는 막을 수 없다.
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

	"github.com/kenshin579/toss-go/internal/strutil"
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
	msg := fmt.Sprintf("toss: token request failed (status %d)", e.StatusCode)
	if e.Code != "" {
		msg += ": " + e.Code
	}
	if e.Description != "" {
		msg += ": " + e.Description
	}
	return msg
}

// TokenSource 는 access token 을 발급·캐시한다. 동시 호출에 안전하다.
type TokenSource struct {
	clientID     string
	clientSecret string
	tokenURL     string
	hc           *http.Client
	now          func() time.Time

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

// New 는 TokenSource 를 만든다. baseURL 은 `https://openapi.tossinvest.com` 형태(끝 슬래시 없음).
func New(clientID, clientSecret, baseURL string, hc *http.Client) *TokenSource {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &TokenSource{
		clientID:     clientID,
		clientSecret: clientSecret,
		tokenURL:     strings.TrimRight(baseURL, "/") + "/oauth2/token",
		hc:           hc,
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
	issuedAt := s.now()
	tok, expiresIn, err := s.issue(ctx)
	if err != nil {
		return "", err
	}
	s.token = tok
	s.expiresAt = issuedAt.Add(time.Duration(expiresIn) * time.Second)
	return tok, nil
}

// Invalidate 는 stale 이 현재 캐시된 토큰일 때만 캐시를 비운다. API 가 401 토큰 오류를 돌려줬을 때
// 그 요청에 썼던 토큰을 넘긴다. 다른 goroutine 이 이미 새 토큰을 받아 둔 경우를 지우지 않기 위한 비교다
// (토스는 client 당 유효 토큰이 1개라 재발급이 이전 토큰을 즉시 무효화한다).
func (s *TokenSource) Invalidate(stale string) {
	s.mu.Lock()
	if s.token == stale {
		s.token = ""
		s.expiresAt = time.Time{}
	}
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

	resp, err := s.hc.Do(req)
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
			e.Description = strutil.Truncate(string(body), maxErrorBody)
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
	if tr.ExpiresIn <= 0 {
		return "", 0, fmt.Errorf("toss: token response has invalid expires_in %d", tr.ExpiresIn)
	}
	return tr.AccessToken, tr.ExpiresIn, nil
}
EOF
gofmt -l internal; go vet ./internal/auth/ && go test ./internal/auth/ -race -v 2>&1 | tail -10
```
Expected: gofmt 출력 없음, 10 tests PASS (`-race` 포함).

- [ ] **Step 3: 커밋**

```bash
git add internal/auth && git commit -m "feat(auth): OAuth2 client credentials 토큰 발급 + 메모리 캐시

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 3: `internal/httpclient` + `internal/testutil`

**Files:**
- Create: `internal/httpclient/client.go`, `internal/httpclient/client_test.go`, `internal/testutil/server.go`

- [ ] **Step 1: 실패 테스트 작성**

```bash
mkdir -p internal/httpclient && cat > internal/httpclient/client_test.go << 'EOF'
package httpclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

// stubTokens 는 고정 토큰 TokenProvider. invalidated 는 Invalidate 호출 횟수.
type stubTokens struct {
	tokens      []string // 호출 순서대로 반환, 마지막 값 반복
	calls       int32
	invalidated int32

	mu        sync.Mutex
	lastStale string
}

func (s *stubTokens) Token(context.Context) (string, error) {
	i := int(atomic.AddInt32(&s.calls, 1)) - 1
	if i >= len(s.tokens) {
		i = len(s.tokens) - 1
	}
	return s.tokens[i], nil
}
func (s *stubTokens) Invalidate(stale string) {
	atomic.AddInt32(&s.invalidated, 1)
	s.mu.Lock()
	s.lastStale = stale
	s.mu.Unlock()
}

func newClient(t *testing.T, h http.HandlerFunc, tokens ...string) (*Client, *stubTokens, func()) {
	t.Helper()
	srv := httptest.NewServer(h)
	st := &stubTokens{tokens: tokens}
	if len(tokens) == 0 {
		st.tokens = []string{"tok"}
	}
	c := New(Config{BaseURL: srv.URL, HTTPClient: srv.Client(), Tokens: st})
	return c, st, srv.Close
}

func TestGet_UnwrapsResultAndSendsBearer(t *testing.T) {
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/prices" || r.URL.Query().Get("symbols") != "005930,AAPL" {
			t.Errorf("unexpected %s", r.URL.String())
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Accept = %q", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":[{"symbol":"005930"},{"symbol":"AAPL"}]}`))
	})
	defer done()
	var out []struct{ Symbol string }
	if err := c.Get(context.Background(), "/api/v1/prices", url.Values{"symbols": {"005930,AAPL"}}, &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[1].Symbol != "AAPL" {
		t.Errorf("out = %+v", out)
	}
}

func TestGet_EmptyResultArray(t *testing.T) {
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":[]}`))
	})
	defer done()
	var out []struct{ Symbol string }
	if err := c.Get(context.Background(), "/x", nil, &out); err != nil {
		t.Fatal(err)
	}
	if out == nil || len(out) != 0 {
		t.Errorf("want empty non-nil slice, got %#v", out)
	}
}

func TestGet_MapsErrorEnvelope(t *testing.T) {
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "hdr-id")
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"error":{"requestId":"01HXYZ","code":"stock-not-found","message":"요청한 종목을 찾을 수 없습니다.","data":{"field":"symbol"}}}`))
	})
	defer done()
	err := c.Get(context.Background(), "/api/v1/stocks/ZZZ/warnings", nil, nil)
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *APIError, got %T %v", err, err)
	}
	if ae.StatusCode != 404 || ae.RequestID != "01HXYZ" || ae.Code != "stock-not-found" || ae.Data["field"] != "symbol" {
		t.Errorf("got %+v", ae)
	}
	want := "toss: 404 stock-not-found: 요청한 종목을 찾을 수 없습니다. (requestId=01HXYZ)"
	if ae.Error() != want {
		t.Errorf("Error() = %q", ae.Error())
	}
}

func TestGet_NonEnvelopeErrorBody(t *testing.T) {
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "hdr-id")
		w.WriteHeader(403)
		_, _ = w.Write([]byte(strings.Repeat("x", 300)))
	})
	defer done()
	err := c.Get(context.Background(), "/x", nil, nil)
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *APIError, got %v", err)
	}
	if ae.StatusCode != 403 || ae.Code != "" || len(ae.Message) != 200 || ae.RequestID != "hdr-id" {
		t.Errorf("got status=%d code=%q msglen=%d reqid=%q", ae.StatusCode, ae.Code, len(ae.Message), ae.RequestID)
	}
}

func TestGet_RetryAfterOn429(t *testing.T) {
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "3")
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"error":{"requestId":"r","code":"rate-limit-exceeded","message":""}}`))
	})
	defer done()
	err := c.Get(context.Background(), "/x", nil, nil)
	var ae *APIError
	if !errors.As(err, &ae) || ae.StatusCode != 429 || ae.RetryAfter != 3*time.Second {
		t.Errorf("got %+v (err=%v)", ae, err)
	}
}

func TestGet_RetriesOnceOnExpiredToken(t *testing.T) {
	var n int32
	c, st, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) == 1 {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error":{"requestId":"r","code":"expired-token","message":"expired"}}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok2" {
			t.Errorf("retry Authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"result":{"ok":true}}`))
	}, "tok1", "tok2")
	defer done()
	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.Get(context.Background(), "/x", nil, &out); err != nil {
		t.Fatal(err)
	}
	if !out.OK || atomic.LoadInt32(&n) != 2 || atomic.LoadInt32(&st.invalidated) != 1 {
		t.Errorf("ok=%v calls=%d invalidated=%d", out.OK, n, st.invalidated)
	}
	st.mu.Lock()
	if st.lastStale != "tok1" {
		t.Errorf("Invalidate called with %q, want tok1", st.lastStale)
	}
	st.mu.Unlock()
}

func TestGet_DoesNotRetryTwice(t *testing.T) {
	var n int32
	c, st, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"requestId":"r","code":"invalid-token","message":""}}`))
	})
	defer done()
	err := c.Get(context.Background(), "/x", nil, nil)
	var ae *APIError
	if !errors.As(err, &ae) || ae.Code != "invalid-token" {
		t.Fatalf("want invalid-token APIError, got %v", err)
	}
	if atomic.LoadInt32(&n) != 2 || atomic.LoadInt32(&st.invalidated) != 2 {
		t.Errorf("calls=%d invalidated=%d, want 2/2", n, st.invalidated)
	}
}

func TestGet_401OtherCodeNoRetry(t *testing.T) {
	var n int32
	c, st, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"requestId":"r","code":"login-user-not-found","message":""}}`))
	})
	defer done()
	_ = c.Get(context.Background(), "/x", nil, nil)
	if atomic.LoadInt32(&n) != 1 || atomic.LoadInt32(&st.invalidated) != 0 {
		t.Errorf("calls=%d invalidated=%d, want 1/0", n, st.invalidated)
	}
}

func TestGet_ContextCanceled(t *testing.T) {
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"result":null}`))
	})
	defer done()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := c.Get(ctx, "/x", nil, nil); err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("want DeadlineExceeded, got %v", err)
	}
}

func TestGet_DecodeError(t *testing.T) {
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":{"n":"notanumber"}}`))
	})
	defer done()
	var out struct {
		N int `json:"n"`
	}
	if err := c.Get(context.Background(), "/x", nil, &out); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("want decode error, got %v", err)
	}
}

func TestNew_Defaults(t *testing.T) {
	c := New(Config{Tokens: &stubTokens{tokens: []string{"t"}}})
	if c.baseURL != DefaultBaseURL {
		t.Errorf("baseURL = %q", c.baseURL)
	}
	if c.http.Timeout != 30*time.Second {
		t.Errorf("timeout = %v", c.http.Timeout)
	}
}

type failingTokens struct{ err error }

func (f failingTokens) Token(context.Context) (string, error) { return "", f.err }
func (failingTokens) Invalidate(string)                       {}

func TestGet_TokenErrorSurfaces(t *testing.T) {
	want := errors.New("boom")
	c := New(Config{BaseURL: "http://127.0.0.1:0", Tokens: failingTokens{err: want}})
	if err := c.Get(context.Background(), "/x", nil, nil); !errors.Is(err, want) {
		t.Errorf("want token error passthrough, got %v", err)
	}
}

func TestGet_NullResultIsError(t *testing.T) {
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":null}`))
	})
	defer done()
	var out struct{ A int }
	if err := c.Get(context.Background(), "/x", nil, &out); err == nil || !strings.Contains(err.Error(), "no result") {
		t.Errorf("want no-result error, got %v", err)
	}
	// out == nil 이면 바디를 보지 않으므로 에러 없음
	if err := c.Get(context.Background(), "/x", nil, nil); err != nil {
		t.Errorf("nil out must not error: %v", err)
	}
}

func TestParseRetryAfter(t *testing.T) {
	if d := parseRetryAfter(" 3 "); d != 3*time.Second {
		t.Errorf("seconds = %v", d)
	}
	if d := parseRetryAfter("-1"); d != 0 {
		t.Errorf("negative = %v", d)
	}
	if d := parseRetryAfter("garbage"); d != 0 {
		t.Errorf("garbage = %v", d)
	}
	future := time.Now().Add(90 * time.Second).UTC().Format(http.TimeFormat)
	if d := parseRetryAfter(future); d < 80*time.Second || d > 91*time.Second {
		t.Errorf("http-date = %v", d)
	}
	past := time.Now().Add(-90 * time.Second).UTC().Format(http.TimeFormat)
	if d := parseRetryAfter(past); d != 0 {
		t.Errorf("past date = %v", d)
	}
}

func TestGet_NonEnvelopeErrorBodyRuneSafe(t *testing.T) {
	c, _, done := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(502)
		_, _ = w.Write([]byte(strings.Repeat("가", 100)))
	})
	defer done()
	err := c.Get(context.Background(), "/x", nil, nil)
	var ae *APIError
	if !errors.As(err, &ae) || !utf8.ValidString(ae.Message) || len(ae.Message) > 200 {
		t.Errorf("got %+v (%v)", ae, err)
	}
}
EOF
go test ./internal/httpclient/ 2>&1 | head -5
```
Expected: 컴파일 에러(`undefined: Client`, `New`, `Config`, `APIError`, `DefaultBaseURL`).

- [ ] **Step 2: 구현**

```bash
cat > internal/httpclient/client.go << 'EOF'
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

// Get 은 GET {baseURL}{path}?{query} 를 호출해 `result` 를 out 으로 디코딩한다.
// out 이 nil 이면 바디를 버린다. 2xx 인데 result 가 없거나 null 이면 에러(토스는 2xx 에 항상 result 를 채운다). 4xx/5xx 는 *APIError.
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
		return fmt.Errorf("toss: %s: response has no result", path)
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
	if resp.StatusCode == http.StatusUnauthorized && isTokenError(apiErr.Code) {
		// 서버가 거부한 토큰은 재시도 여부와 무관하게 캐시에서 제거한다(재시도 후 401 이어도 하루 동안 남지 않도록).
		c.tokens.Invalidate(tok)
		if !retried {
			return c.do(ctx, path, query, true)
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
EOF
gofmt -l internal; go vet ./internal/httpclient/ && go test ./internal/httpclient/ -race -v 2>&1 | tail -14
```
Expected: gofmt 출력 없음, 15 tests PASS. (`TestGet_EmptyResultArray` 는 `{"result":[]}` 가 `[]T{}` 로 디코딩되어 non-nil 빈 슬라이스여야 한다 — `json.Unmarshal` 의 표준 동작.)

- [ ] **Step 3: testutil 작성** — 그룹 패키지 테스트가 공유하는 스텁 서버.

```bash
mkdir -p internal/testutil && cat > internal/testutil/server.go << 'EOF'
// Package testutil 은 그룹 패키지 단위 테스트용 스텁 서버 헬퍼다.
package testutil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/kenshin579/toss-go/internal/httpclient"
)

type staticTokens struct{}

func (staticTokens) Token(context.Context) (string, error) { return "test-token", nil }
func (staticTokens) Invalidate(string)                     {}

// Expect 는 스텁 서버가 검증할 요청 조건.
type Expect struct {
	Path  string     // 정확히 일치해야 하는 경로
	Query url.Values // 나열된 키는 값이 정확히 일치해야 하고, 나열되지 않은 키는 있으면 안 된다
}

// NewServer 는 요청을 검증하고 status/body 를 돌려주는 서버와 그에 연결된 httpclient 를 만든다.
// Bearer 헤더가 "test-token" 인지도 검증한다.
func NewServer(t *testing.T, want Expect, status int, body []byte) (*httpclient.Client, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != want.Path {
			t.Errorf("path = %q, want %q", r.URL.Path, want.Path)
		}
		got := r.URL.Query()
		for k := range want.Query {
			if got.Get(k) != want.Query.Get(k) {
				t.Errorf("query %s = %q, want %q", k, got.Get(k), want.Query.Get(k))
			}
		}
		for k := range got {
			if _, ok := want.Query[k]; !ok {
				t.Errorf("unexpected query %s=%q", k, got.Get(k))
			}
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	c := httpclient.New(httpclient.Config{BaseURL: srv.URL, HTTPClient: srv.Client(), Tokens: staticTokens{}})
	return c, srv.Close
}

// Fixture 는 testdata/<name> 을 읽는다.
func Fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}
EOF
gofmt -l internal; go vet ./internal/... && echo VET_OK
```
Expected: `VET_OK`.

- [ ] **Step 3b: internal/strutil (auth·httpclient 공용 절단 헬퍼)**

```bash
mkdir -p internal/strutil && cat > internal/strutil/strutil.go << 'EOF'
// Package strutil 은 에러 메시지 정리용 소형 문자열 헬퍼다.
package strutil

import (
	"strings"
	"unicode/utf8"
)

// Truncate 는 s 의 연속 공백·개행을 한 칸으로 합친 뒤 최대 n 바이트로 자른다. UTF-8 문자 경계를 지킨다.
func Truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
EOF
gofmt -l internal; echo OK
```

```bash
cat > internal/strutil/strutil_test.go << 'EOF'
package strutil

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncate(t *testing.T) {
	if got := Truncate("  a \n\n b  ", 100); got != "a b" {
		t.Errorf("collapse = %q", got)
	}
	if got := Truncate("abcdef", 3); got != "abc" {
		t.Errorf("ascii = %q", got)
	}
	s := strings.Repeat("가", 100) // 300 bytes
	got := Truncate(s, 200)
	if !utf8.ValidString(got) || len(got) != 198 || utf8.RuneCountInString(got) != 66 {
		t.Errorf("rune boundary: len=%d runes=%d valid=%v", len(got), utf8.RuneCountInString(got), utf8.ValidString(got))
	}
	if got := Truncate("", 5); got != "" {
		t.Errorf("empty = %q", got)
	}
}
EOF
go test ./internal/strutil/
```
Expected: `ok`.

이어서 `internal/auth/token.go` 의 로컬 `truncate` 를 지우고 `strutil.Truncate` 를 쓰도록, `internal/httpclient/client.go` 의 에러 바디 절단도 `strutil.Truncate` 를 쓰도록 맞춘다(위 두 파일 heredoc 에 이미 반영됨).

- [ ] **Step 4: 커밋**

```bash
git add internal/httpclient internal/testutil internal/strutil && git commit -m "feat(httpclient): Bearer 주입·result 봉투 해제·APIError 매핑·401 토큰 재시도 + 테스트 헬퍼

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 4: `internal/params` + `internal/fetch` + `marketdata` (5 ops)

**Files:**
- Create: `internal/params/params.go`, `internal/params/params_test.go`
- Create: `internal/fetch/fetch.go`, `internal/fetch/fetch_test.go`
- Create: `marketdata/client.go`, `marketdata/prices.go`, `marketdata/orderbook.go`, `marketdata/trades.go`, `marketdata/price_limits.go`, `marketdata/candles.go`, `marketdata/marketdata_test.go`
- Move: `testdata/captured/{prices_symbols_005930_AAPL_,prices_symbols_ZZZZZZ_,orderbook_symbol_005930_,trades_symbol_005930_count_2_,price_limits_symbol_005930_,candles_symbol_005930_interval_1d_count_2_}.json` → `marketdata/testdata/`

- [ ] **Step 1: params 헬퍼 테스트 + 구현** (쿼리 조립 규칙: zero-value 는 생략, 시각은 RFC3339)

```bash
mkdir -p internal/params && cat > internal/params/params_test.go << 'EOF'
package params

import (
	"net/url"
	"testing"
	"time"

	"github.com/kenshin579/toss-go/tosstypes"
)

func TestRequire(t *testing.T) {
	if err := Require("symbol", ""); err == nil || err.Error() != "toss: symbol must not be empty" {
		t.Errorf("got %v", err)
	}
	if err := Require("symbol", " "); err != nil {
		t.Error("whitespace is not empty; format is validated by Symbol")
	}
	if err := Require("symbol", "005930"); err != nil {
		t.Error(err)
	}
}

func TestSymbol(t *testing.T) {
	for _, ok := range []string{"005930", "AAPL", "BRK.B", "BF-B", "aapl"} {
		if err := Symbol(ok); err != nil {
			t.Errorf("Symbol(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", " 005930", "005930 ", "삼성", "A/B", "a,b"} {
		if err := Symbol(bad); err == nil {
			t.Errorf("Symbol(%q) must fail", bad)
		}
	}
}

func TestSymbols(t *testing.T) {
	if got, err := Symbols([]string{"005930", "AAPL"}); err != nil || got != "005930,AAPL" {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := Symbols(nil); err == nil {
		t.Error("empty must fail")
	}
	if _, err := Symbols([]string{"005930", ""}); err == nil {
		t.Error("empty element must fail")
	}
	many := make([]string, MaxSymbols+1)
	for i := range many {
		many[i] = "A"
	}
	if _, err := Symbols(many); err == nil {
		t.Error("over max must fail")
	}
	if _, err := Symbols(many[:MaxSymbols]); err != nil {
		t.Errorf("exactly max must pass: %v", err)
	}
}

func TestSetters_SkipZero(t *testing.T) {
	v := url.Values{}
	Str(v, "s", "")
	Int(v, "n", 0)
	Bool(v, "b", nil)
	Time(v, "t", nil)
	Date(v, "d", "")
	if len(v) != 0 {
		t.Errorf("zero values must be skipped, got %v", v)
	}
}

func TestSetters_Values(t *testing.T) {
	v := url.Values{}
	f := false
	ts := time.Date(2026, 9, 1, 0, 0, 0, 0, tosstypes.KST)
	Str(v, "s", "x")
	Int(v, "n", 7)
	Bool(v, "b", &f)
	Time(v, "t", &ts)
	Date(v, "d", tosstypes.Date("2026-09-02"))
	want := "b=false&d=2026-09-02&n=7&s=x&t=2026-09-01T00%3A00%3A00%2B09%3A00"
	if got := v.Encode(); got != want {
		t.Errorf("Encode = %q, want %q", got, want)
	}
}
EOF
cat > internal/params/params.go << 'EOF'
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

// symbolPattern 은 토스 심볼 규칙(openapi components.parameters.Symbol): 영문 대/소문자, 숫자, '.', '-'.
var symbolPattern = regexp.MustCompile(`^[A-Za-z0-9.\-]+$`)

// MaxSymbols 는 symbols= 쿼리에 넣을 수 있는 최대 심볼 수(openapi: 최대 200개).
const MaxSymbols = 200

// Symbol 은 심볼 형식을 검증한다(빈 값·공백·허용 외 문자 거부). 요청을 보내기 전에 실패시켜 rate limit 을 아낀다.
func Symbol(v string) error {
	if !symbolPattern.MatchString(v) {
		return fmt.Errorf("toss: invalid symbol %q (allowed: A-Z a-z 0-9 . -)", v)
	}
	return nil
}

// Symbols 는 symbols= 쿼리 값을 만든다. 빈 목록, 형식 위반 원소, MaxSymbols 초과를 거부한다.
func Symbols(symbols []string) (string, error) {
	if len(symbols) == 0 {
		return "", errors.New("toss: symbols must not be empty")
	}
	if len(symbols) > MaxSymbols {
		return "", fmt.Errorf("toss: too many symbols %d (max %d)", len(symbols), MaxSymbols)
	}
	for _, s := range symbols {
		if err := Symbol(s); err != nil {
			return "", err
		}
	}
	return strings.Join(symbols, ","), nil
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
EOF
gofmt -l internal; go vet ./internal/params/ && go test ./internal/params/ 2>&1 | tail -2
```
Expected: `ok  	github.com/kenshin579/toss-go/internal/params`.

- [ ] **Step 1b: internal/fetch (제네릭 One/List 헬퍼)**

```bash
mkdir -p internal/fetch && cat > internal/fetch/fetch.go << 'EOF'
// Package fetch 는 그룹 패키지가 공유하는 제네릭 조회 헬퍼다. 검증·쿼리 조립은 호출 측이 하고,
// 여기서는 httpclient.Get 호출과 결과 포인터/슬라이스 반환만 담당한다.
package fetch

import (
	"context"
	"net/url"

	"github.com/kenshin579/toss-go/internal/httpclient"
)

// One 은 result 객체 하나를 *T 로 디코딩한다. 실패 시 nil 과 에러.
func One[T any](ctx context.Context, hc *httpclient.Client, path string, q url.Values) (*T, error) {
	var out T
	if err := hc.Get(ctx, path, q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List 는 result 배열을 []T 로 디코딩한다. 빈 배열은 nil 이 아닌 빈 슬라이스, 실패 시 nil 과 에러.
func List[T any](ctx context.Context, hc *httpclient.Client, path string, q url.Values) ([]T, error) {
	out := []T{}
	if err := hc.Get(ctx, path, q, &out); err != nil {
		return nil, err
	}
	return out, nil
}
EOF
cat > internal/fetch/fetch_test.go << 'EOF'
package fetch

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/kenshin579/toss-go/internal/httpclient"
	"github.com/kenshin579/toss-go/internal/testutil"
)

type item struct {
	Symbol string `json:"symbol"`
}

func TestOne(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/one", Query: url.Values{"a": {"1"}}}, 200, []byte(`{"result":{"symbol":"X"}}`))
	defer done()
	got, err := One[item](context.Background(), hc, "/one", url.Values{"a": {"1"}})
	if err != nil || got == nil || got.Symbol != "X" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestOne_ErrorReturnsNil(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/one"}, 404, []byte(`{"error":{"requestId":"r","code":"stock-not-found","message":""}}`))
	defer done()
	got, err := One[item](context.Background(), hc, "/one", nil)
	var ae *httpclient.APIError
	if got != nil || !errors.As(err, &ae) || ae.Code != "stock-not-found" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestList(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/list"}, 200, []byte(`{"result":[{"symbol":"A"},{"symbol":"B"}]}`))
	defer done()
	got, err := List[item](context.Background(), hc, "/list", nil)
	if err != nil || len(got) != 2 || got[1].Symbol != "B" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestList_EmptyIsNonNil(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/list"}, 200, []byte(`{"result":[]}`))
	defer done()
	got, err := List[item](context.Background(), hc, "/list", nil)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestList_ErrorReturnsNil(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/list"}, 500, []byte(`oops`))
	defer done()
	got, err := List[item](context.Background(), hc, "/list", nil)
	if got != nil || err == nil {
		t.Fatalf("got %#v, %v", got, err)
	}
}
EOF
go test ./internal/fetch/ 2>&1 | tail -2
```
Expected: `ok  	github.com/kenshin579/toss-go/internal/fetch`.

- [ ] **Step 2: fixture 이동**

```bash
mkdir -p marketdata/testdata && git mv testdata/captured/prices_symbols_005930_AAPL_.json marketdata/testdata/prices.json && git mv testdata/captured/prices_symbols_ZZZZZZ_.json marketdata/testdata/prices_empty.json && git mv testdata/captured/orderbook_symbol_005930_.json marketdata/testdata/orderbook.json && git mv testdata/captured/trades_symbol_005930_count_2_.json marketdata/testdata/trades.json && git mv testdata/captured/price_limits_symbol_005930_.json marketdata/testdata/price_limits.json && git mv testdata/captured/candles_symbol_005930_interval_1d_count_2_.json marketdata/testdata/candles.json && ls marketdata/testdata
```
Expected: 6개 파일.

- [ ] **Step 3: 실패 테스트 작성**

```bash
cat > marketdata/marketdata_test.go << 'EOF'
package marketdata

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/kenshin579/toss-go/internal/testutil"
	"github.com/kenshin579/toss-go/tosstypes"
)

func TestPrices(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/prices", Query: url.Values{"symbols": {"005930,AAPL"}}}, 200, testutil.Fixture(t, "prices.json"))
	defer done()
	got, err := New(hc).Prices(context.Background(), "005930", "AAPL")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Symbol != "005930" || got[0].LastPrice.String() != "248000" || got[0].Currency != tosstypes.CurrencyKRW {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[0].Timestamp == nil || got[0].Timestamp.Hour() != 19 {
		t.Errorf("Timestamp = %v", got[0].Timestamp)
	}
	if got[1].LastPrice.String() != "330.02" || got[1].Currency != tosstypes.CurrencyUSD {
		t.Errorf("got[1] = %+v", got[1])
	}
}

func TestPrices_EmptyResult(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/prices", Query: url.Values{"symbols": {"ZZZZZZ"}}}, 200, testutil.Fixture(t, "prices_empty.json"))
	defer done()
	got, err := New(hc).Prices(context.Background(), "ZZZZZZ")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("want empty, got %+v", got)
	}
}

func TestPrices_NoSymbols(t *testing.T) {
	if _, err := New(nil).Prices(context.Background()); err == nil { // nil client: 검증이 요청 전에 실패해야 한다
		t.Error("want error")
	}
}

func TestOrderbook(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/orderbook", Query: url.Values{"symbol": {"005930"}}}, 200, testutil.Fixture(t, "orderbook.json"))
	defer done()
	ob, err := New(hc).Orderbook(context.Background(), "005930")
	if err != nil {
		t.Fatal(err)
	}
	if len(ob.Asks) != 10 || len(ob.Bids) != 10 {
		t.Errorf("asks=%d bids=%d", len(ob.Asks), len(ob.Bids))
	}
	if ob.Asks[0].Price.String() != "248000" || ob.Asks[0].Volume.String() != "33855" {
		t.Errorf("asks[0] = %+v", ob.Asks[0])
	}
	if ob.Timestamp == nil || ob.Currency != tosstypes.CurrencyKRW {
		t.Errorf("ts=%v cur=%s", ob.Timestamp, ob.Currency)
	}
}

func TestTrades(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/trades", Query: url.Values{"symbol": {"005930"}, "count": {"2"}}}, 200, testutil.Fixture(t, "trades.json"))
	defer done()
	got, err := New(hc).Trades(context.Background(), "005930", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].Price.String() != "247500" || got[1].Volume.String() != "200" || got[1].Timestamp.IsZero() {
		t.Errorf("got = %+v", got)
	}
}

func TestTrades_DefaultCountOmitted(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/trades", Query: url.Values{"symbol": {"005930"}}}, 200, testutil.Fixture(t, "trades.json"))
	defer done()
	if _, err := New(hc).Trades(context.Background(), "005930", 0); err != nil {
		t.Fatal(err)
	}
}

func TestPriceLimits(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/price-limits", Query: url.Values{"symbol": {"005930"}}}, 200, testutil.Fixture(t, "price_limits.json"))
	defer done()
	pl, err := New(hc).PriceLimits(context.Background(), "005930")
	if err != nil {
		t.Fatal(err)
	}
	if pl.UpperLimitPrice == nil || pl.UpperLimitPrice.String() != "325000" || pl.LowerLimitPrice == nil || pl.LowerLimitPrice.String() != "175000" {
		t.Errorf("got %+v", pl)
	}
}

func TestCandles(t *testing.T) {
	before := time.Date(2026, 9, 1, 0, 0, 0, 0, tosstypes.KST)
	adj := false
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/candles", Query: url.Values{
		"symbol": {"005930"}, "interval": {"1d"}, "count": {"2"}, "before": {"2026-09-01T00:00:00+09:00"}, "adjusted": {"false"},
	}}, 200, testutil.Fixture(t, "candles.json"))
	defer done()
	page, err := New(hc).Candles(context.Background(), CandlesParams{Symbol: "005930", Interval: tosstypes.Interval1d, Count: 2, Before: &before, Adjusted: &adj})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Candles) != 2 {
		t.Fatalf("candles = %d", len(page.Candles))
	}
	c0 := page.Candles[0]
	if c0.OpenPrice.String() != "252000" || c0.HighPrice.String() != "255500" || c0.LowPrice.String() != "243000" || c0.ClosePrice.String() != "248000" || c0.Volume.String() != "21475989" {
		t.Errorf("c0 = %+v", c0)
	}
	if c0.Timestamp.Year() != 2026 || c0.Timestamp.Day() != 3 {
		t.Errorf("Timestamp = %v", c0.Timestamp)
	}
	if page.NextBefore == nil || page.NextBefore.Day() != 1 {
		t.Errorf("NextBefore = %v", page.NextBefore)
	}
}

func TestCandles_Validation(t *testing.T) {
	c := New(nil) // nil client: 검증이 요청 전에 실패해야 한다
	if _, err := c.Candles(context.Background(), CandlesParams{Interval: tosstypes.Interval1d}); err == nil {
		t.Error("want error for empty symbol")
	}
	if _, err := c.Candles(context.Background(), CandlesParams{Symbol: "005930"}); err == nil {
		t.Error("want error for empty interval")
	}
}

func TestEmptySymbolRejected(t *testing.T) {
	c := New(nil) // nil client: 검증이 요청 전에 실패해야 한다
	ctx := context.Background()
	if _, err := c.Orderbook(ctx, ""); err == nil {
		t.Error("Orderbook")
	}
	if _, err := c.Trades(ctx, " 005930", 1); err == nil {
		t.Error("Trades with whitespace")
	}
	if _, err := c.PriceLimits(ctx, "삼성"); err == nil {
		t.Error("PriceLimits non-ascii")
	}
	if _, err := c.Prices(ctx, "005930", ""); err == nil {
		t.Error("Prices empty element")
	}
}
EOF
go test ./marketdata/ 2>&1 | head -5
```
Expected: 컴파일 에러(`undefined: New`, `CandlesParams`).

- [ ] **Step 4: 구현**

```bash
cat > marketdata/client.go << 'EOF'
// Package marketdata 는 토스 Open API 시세(Market Data) 그룹 — 현재가·호가·체결·상하한가·캔들.
// toss.Client.MarketData 로 접근한다.
package marketdata

import "github.com/kenshin579/toss-go/internal/httpclient"

// Client 는 시세 sub-client.
type Client struct {
	http *httpclient.Client
}

// New 는 internal 용도 — toss.NewClient 가 호출한다.
func New(hc *httpclient.Client) *Client { return &Client{http: hc} }
EOF
cat > marketdata/prices.go << 'EOF'
package marketdata

import (
	"context"
	"net/url"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// Price 는 종목 현재가.
type Price struct {
	Symbol    string             `json:"symbol"`
	Timestamp *time.Time         `json:"timestamp"` // 체결 시각. 시세가 없으면 null
	LastPrice decimal.Decimal    `json:"lastPrice"`
	Currency  tosstypes.Currency `json:"currency"`
}

// Prices 는 여러 종목의 현재가를 조회한다(GET /api/v1/prices). 최대 200개. 없는 심볼은 결과에서 빠진다.
func (c *Client) Prices(ctx context.Context, symbols ...string) ([]Price, error) {
	joined, err := params.Symbols(symbols)
	if err != nil {
		return nil, err
	}
	return fetch.List[Price](ctx, c.http, "/api/v1/prices", url.Values{"symbols": {joined}})
}
EOF
cat > marketdata/orderbook.go << 'EOF'
package marketdata

import (
	"context"
	"net/url"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// OrderbookEntry 는 호가 한 단계(가격·잔량).
type OrderbookEntry struct {
	Price  decimal.Decimal `json:"price"`
	Volume decimal.Decimal `json:"volume"`
}

// Orderbook 은 매도/매수 호가.
type Orderbook struct {
	Timestamp *time.Time         `json:"timestamp"` // 호가 시각. 호가가 없으면(장외 등) nil
	Currency  tosstypes.Currency `json:"currency"`
	Asks      []OrderbookEntry   `json:"asks"` // 매도 호가(낮은 가격부터)
	Bids      []OrderbookEntry   `json:"bids"` // 매수 호가(높은 가격부터)
}

// Orderbook 은 호가를 조회한다(GET /api/v1/orderbook).
func (c *Client) Orderbook(ctx context.Context, symbol string) (*Orderbook, error) {
	if err := params.Symbol(symbol); err != nil {
		return nil, err
	}
	return fetch.One[Orderbook](ctx, c.http, "/api/v1/orderbook", url.Values{"symbol": {symbol}})
}
EOF
cat > marketdata/trades.go << 'EOF'
package marketdata

import (
	"context"
	"net/url"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// Trade 는 체결 1건.
type Trade struct {
	Price     decimal.Decimal    `json:"price"`
	Volume    decimal.Decimal    `json:"volume"`
	Timestamp time.Time          `json:"timestamp"`
	Currency  tosstypes.Currency `json:"currency"`
}

// Trades 는 최근 체결 내역을 조회한다(GET /api/v1/trades). count 는 최대 50, 0 이면 서버 기본값(50).
func (c *Client) Trades(ctx context.Context, symbol string, count int) ([]Trade, error) {
	if err := params.Symbol(symbol); err != nil {
		return nil, err
	}
	q := url.Values{"symbol": {symbol}}
	params.Int(q, "count", count)
	return fetch.List[Trade](ctx, c.http, "/api/v1/trades", q)
}
EOF
cat > marketdata/price_limits.go << 'EOF'
package marketdata

import (
	"context"
	"net/url"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// PriceLimits 는 당일 상한가·하한가. 해외 종목 등 제한이 없으면 nil.
type PriceLimits struct {
	Timestamp       time.Time          `json:"timestamp"`
	UpperLimitPrice *decimal.Decimal   `json:"upperLimitPrice"`
	LowerLimitPrice *decimal.Decimal   `json:"lowerLimitPrice"`
	Currency        tosstypes.Currency `json:"currency"`
}

// PriceLimits 는 상/하한가를 조회한다(GET /api/v1/price-limits).
func (c *Client) PriceLimits(ctx context.Context, symbol string) (*PriceLimits, error) {
	if err := params.Symbol(symbol); err != nil {
		return nil, err
	}
	return fetch.One[PriceLimits](ctx, c.http, "/api/v1/price-limits", url.Values{"symbol": {symbol}})
}
EOF
cat > marketdata/candles.go << 'EOF'
package marketdata

import (
	"context"
	"net/url"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// Candle 은 OHLCV 봉 1개.
type Candle struct {
	Timestamp  time.Time          `json:"timestamp"` // 봉 시작 시각
	OpenPrice  decimal.Decimal    `json:"openPrice"`
	HighPrice  decimal.Decimal    `json:"highPrice"`
	LowPrice   decimal.Decimal    `json:"lowPrice"`
	ClosePrice decimal.Decimal    `json:"closePrice"`
	Volume     decimal.Decimal    `json:"volume"`
	Currency   tosstypes.Currency `json:"currency"`
}

// CandlePage 는 캔들 한 페이지. NextBefore 를 다음 요청의 Before 로 넘기면 이어서 조회한다.
type CandlePage struct {
	Candles    []Candle   `json:"candles"`
	NextBefore *time.Time `json:"nextBefore"` // 더 없으면 nil
}

// CandlesParams 는 Candles 인자.
type CandlesParams struct {
	Symbol   string             // 필수
	Interval tosstypes.Interval // 필수 (1m, 1d)
	Count    int                // 최대 200, 0 이면 서버 기본값(100)
	Before   *time.Time         // 이 시각 이하(inclusive)의 봉만. nil 이면 최신부터
	Adjusted *bool              // 수정주가 적용 여부. nil 이면 서버 기본값(true)
}

// Candles 는 캔들 차트를 조회한다(GET /api/v1/candles).
func (c *Client) Candles(ctx context.Context, p CandlesParams) (*CandlePage, error) {
	if err := params.Symbol(p.Symbol); err != nil {
		return nil, err
	}
	if err := params.Require("interval", string(p.Interval)); err != nil {
		return nil, err
	}
	q := url.Values{"symbol": {p.Symbol}, "interval": {string(p.Interval)}}
	params.Int(q, "count", p.Count)
	params.Time(q, "before", p.Before)
	params.Bool(q, "adjusted", p.Adjusted)
	return fetch.One[CandlePage](ctx, c.http, "/api/v1/candles", q)
}
EOF
gofmt -l marketdata internal; go vet ./marketdata/ && go test ./marketdata/ -v 2>&1 | tail -14
```
Expected: gofmt 출력 없음, 10 tests PASS.

- [ ] **Step 5: 커밋**

```bash
git add internal/params internal/fetch marketdata testdata && git commit -m "feat(marketdata): 현재가·호가·체결·상하한가·캔들 5 ops + params 헬퍼

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 5: `stockinfo` (8 ops)

**Files:**
- Create: `stockinfo/client.go`, `stockinfo/stocks.go`, `stockinfo/warnings.go`, `stockinfo/trend.go`, `stockinfo/stockinfo_test.go`
- Move: `testdata/captured/{stocks_symbols_005930_AAPL_,stocks_005930_warnings_,stocks_005930_investor_trading_count_1_,stocks_005930_program_trades_count_1_,stocks_005930_short_selling_count_1_}.json` → `stockinfo/testdata/`
- Capture: `stockinfo/testdata/{stocks_all,credit_trades,securities_lending}.json`

- [ ] **Step 1: fixture 이동 + 미보유 fixture 캡처**

```bash
mkdir -p stockinfo/testdata && git mv testdata/captured/stocks_symbols_005930_AAPL_.json stockinfo/testdata/stocks.json && git mv testdata/captured/stocks_005930_warnings_.json stockinfo/testdata/warnings_empty.json && git mv testdata/captured/stocks_005930_investor_trading_count_1_.json stockinfo/testdata/investor_trading.json && git mv testdata/captured/stocks_005930_program_trades_count_1_.json stockinfo/testdata/program_trades.json && git mv testdata/captured/stocks_005930_short_selling_count_1_.json stockinfo/testdata/short_selling.json
eval "$(grep -E '^export TOSS_CLIENT_(ID|SECRET)=' ~/.zshrc)"
TOKEN=$(curl -s --compressed -X POST https://openapi.tossinvest.com/oauth2/token -H 'Content-Type: application/x-www-form-urlencoded' -d grant_type=client_credentials -d "client_id=$TOSS_CLIENT_ID" -d "client_secret=$TOSS_CLIENT_SECRET" | jq -r .access_token)
if [ -n "$TOKEN" ] && [ "$TOKEN" != "null" ]; then echo TOKEN_OK; else echo "TOKEN_UNAVAILABLE → openapi.json 예시로 대체"; fi
J=docs/api/openapi.json; ex() { jq --arg p "$1" --arg n "$2" '.paths[$p].get.responses."200".content."application/json".examples[$n].value' $J; }
B=https://openapi.tossinvest.com/api/v1
if [ -n "$TOKEN" ] && [ "$TOKEN" != "null" ]; then
  curl -s --compressed -H "Authorization: Bearer $TOKEN" "$B/stocks/all?market=KOSPI&securityType=REIT" | jq '.result |= .[:3]' > stockinfo/testdata/stocks_all.json; sleep 1
  curl -s --compressed -H "Authorization: Bearer $TOKEN" "$B/stocks/005930/credit-trades?count=1" | jq . > stockinfo/testdata/credit_trades.json; sleep 0.3
  curl -s --compressed -H "Authorization: Bearer $TOKEN" "$B/stocks/005930/securities-lending?count=1" | jq . > stockinfo/testdata/securities_lending.json
else
  # 허용 IP 미등록 등으로 실호출이 안 되면 openapi.json 의 응답 예시(토스 작성)로 대체 — 구조는 동일
  ex "/api/v1/stocks/all" kospi > stockinfo/testdata/stocks_all.json
  ex "/api/v1/stocks/{symbol}/credit-trades" daily > stockinfo/testdata/credit_trades.json
  ex "/api/v1/stocks/{symbol}/securities-lending" daily > stockinfo/testdata/securities_lending.json
fi
for f in stockinfo/testdata/*.json; do printf "%s: " "$f"; jq -c 'if .result then (.result|type) else . end' "$f"; done
```
Expected: `TOKEN_OK`(또는 대체 안내), 8개 파일 모두 `"array"` 또는 `"object"`(에러 봉투가 아님). `credit_trades.json` 의 `.result.records[0]` 에 `marginLoan`/`stockLoan` 이, `securities_lending.json` 에 `executionQuantity` 등이 있는지 `jq` 로 눈으로 확인한다. 값이 `null` 인 경우(해당 일자 데이터 없음)도 정상이며 테스트는 nil 허용으로 쓴다.

- [ ] **Step 2: 실패 테스트 작성**

```bash
cat > stockinfo/stockinfo_test.go << 'EOF'
package stockinfo

import (
	"context"
	"net/url"
	"testing"

	"github.com/kenshin579/toss-go/internal/testutil"
	"github.com/kenshin579/toss-go/tosstypes"
)

func TestStocks(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/stocks", Query: url.Values{"symbols": {"005930,AAPL"}}}, 200, testutil.Fixture(t, "stocks.json"))
	defer done()
	got, err := New(hc).Stocks(context.Background(), "005930", "AAPL")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	var samsung, apple *Stock
	for i := range got {
		switch got[i].Symbol {
		case "005930":
			samsung = &got[i]
		case "AAPL":
			apple = &got[i]
		}
	}
	if samsung == nil || apple == nil {
		t.Fatalf("symbols = %+v", got)
	}
	if samsung.Name != "삼성전자" || samsung.Market != tosstypes.MarketKOSPI || samsung.SecurityType != tosstypes.SecurityTypeStock || samsung.Status != tosstypes.StockStatusActive || !samsung.IsCommonShare {
		t.Errorf("samsung = %+v", samsung)
	}
	if samsung.ListDate == nil || *samsung.ListDate != "1975-06-11" || samsung.DelistDate != nil || samsung.LeverageFactor != nil {
		t.Errorf("samsung dates/leverage = %v %v %v", samsung.ListDate, samsung.DelistDate, samsung.LeverageFactor)
	}
	if samsung.SharesOutstanding.String() != "5846278608" {
		t.Errorf("SharesOutstanding = %s", samsung.SharesOutstanding)
	}
	if samsung.KoreanMarketDetail == nil || !samsung.KoreanMarketDetail.NXTSupported || samsung.KoreanMarketDetail.NXTTradingSuspended == nil || *samsung.KoreanMarketDetail.NXTTradingSuspended {
		t.Errorf("KoreanMarketDetail = %+v", samsung.KoreanMarketDetail)
	}
	if apple.KoreanMarketDetail != nil || apple.Market != tosstypes.MarketNASDAQ || apple.Currency != tosstypes.CurrencyUSD {
		t.Errorf("apple = %+v", apple)
	}
}

// fixture = openapi 예시(kospi): 요청 파라미터와 무관하게 STOCK+ETF 2건
func TestListStocks(t *testing.T) {
	cs := true
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/stocks/all", Query: url.Values{
		"market": {"KOSPI"}, "securityType": {"REIT"}, "status": {"ACTIVE"}, "commonShare": {"true"},
	}}, 200, testutil.Fixture(t, "stocks_all.json"))
	defer done()
	got, err := New(hc).ListStocks(context.Background(), ListStocksParams{Market: tosstypes.MarketKOSPI, SecurityType: tosstypes.SecurityTypeREIT, Status: tosstypes.StockStatusActive, CommonShare: &cs})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Symbol != "005930" || got[0].Name != "삼성전자" || got[0].SecurityType != tosstypes.SecurityTypeStock || !got[0].IsCommonShare || got[0].ISINCode != "KR7005930003" {
		t.Errorf("got[0] = %+v", got)
	}
	if got[1].Symbol != "069500" || got[1].SecurityType != tosstypes.SecurityTypeETF || got[1].ISINCode != "KR7069500007" {
		t.Errorf("got[1] = %+v", got)
	}
}

func TestListStocks_RequiresMarket(t *testing.T) {
	if _, err := New(nil).ListStocks(context.Background(), ListStocksParams{}); err == nil {
		t.Error("want error")
	}
}

func TestWarnings_Empty(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/stocks/005930/warnings"}, 200, testutil.Fixture(t, "warnings_empty.json"))
	defer done()
	got, err := New(hc).Warnings(context.Background(), "005930")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v", got)
	}
}

func TestWarnings_Decode(t *testing.T) {
	body := []byte(`{"result":[{"warningType":"OVERHEATED","exchange":"KRX","startDate":"2026-09-01","endDate":null}]}`)
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/stocks/000001/warnings"}, 200, body)
	defer done()
	got, err := New(hc).Warnings(context.Background(), "000001")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].WarningType != tosstypes.WarningTypeOverheated || got[0].Exchange == nil || *got[0].Exchange != "KRX" || got[0].StartDate == nil || got[0].EndDate != nil {
		t.Errorf("got = %+v", got)
	}
}

func TestInvestorTrading(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/stocks/005930/investor-trading", Query: url.Values{"count": {"1"}, "until": {"2026-09-03"}}}, 200, testutil.Fixture(t, "investor_trading.json"))
	defer done()
	page, err := New(hc).InvestorTrading(context.Background(), "005930", TrendParams{Count: 1, Until: "2026-09-03"})
	if err != nil {
		t.Fatal(err)
	}
	if page.NextUntil == nil || *page.NextUntil != "2026-09-02" || len(page.Records) != 1 {
		t.Fatalf("page = %+v", page)
	}
	r := page.Records[0]
	if r.Date != "2026-09-03" || r.UpdatedAt.IsZero() {
		t.Errorf("date/updatedAt = %s %v", r.Date, r.UpdatedAt)
	}
	if r.Individual == nil || r.Individual.NetBuyVolume.String() != "-1248264" {
		t.Errorf("Individual = %+v", r.Individual)
	}
	if r.Foreigner.BuyVolume.String() != "3898908" {
		t.Errorf("Foreigner = %+v", r.Foreigner)
	}
	if r.Institution.NetBuyVolume.String() != "-860563" || r.Institution.Breakdown == nil || r.Institution.Breakdown.FinancialInvestment.BuyVolume.String() != "1036782" {
		t.Errorf("Institution = %+v", r.Institution)
	}
	if r.OtherCorporation == nil || r.ForeignerHolding == nil || r.ForeignerHolding.HoldingRate.String() != "0.4671" || r.CFD != nil {
		t.Errorf("other=%v fh=%v cfd=%v", r.OtherCorporation, r.ForeignerHolding, r.CFD)
	}
}

func TestProgramTrades(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/stocks/005930/program-trades", Query: url.Values{"count": {"1"}}}, 200, testutil.Fixture(t, "program_trades.json"))
	defer done()
	page, err := New(hc).ProgramTrades(context.Background(), "005930", TrendParams{Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 1 || page.Records[0].Arbitrage.NetBuyVolume.String() != "3406" || page.Records[0].NonArbitrage.SellVolume.String() != "3201913" {
		t.Errorf("page = %+v", page)
	}
}

func TestShortSelling(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/stocks/005930/short-selling", Query: url.Values{"count": {"1"}}}, 200, testutil.Fixture(t, "short_selling.json"))
	defer done()
	page, err := New(hc).ShortSelling(context.Background(), "005930", TrendParams{Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	r := page.Records[0]
	if r.ShortSellingVolume.String() != "1226909" || r.ShortSellingAmount.String() != "306620169500" || r.ShortSellingVolumeRate == nil || r.ShortSellingVolumeRate.String() != "0.08919" {
		t.Errorf("r = %+v", r)
	}
}

// fixture = openapi 예시(daily): 2건, marginLoan/stockLoan 모두 존재
func TestCreditTrades(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/stocks/005930/credit-trades", Query: url.Values{"count": {"1"}}}, 200, testutil.Fixture(t, "credit_trades.json"))
	defer done()
	page, err := New(hc).CreditTrades(context.Background(), "005930", TrendParams{Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.NextUntil == nil || *page.NextUntil != "2026-07-14" || len(page.Records) != 2 {
		t.Fatalf("page = %+v", page)
	}
	r := page.Records[0]
	if r.Date != "2026-07-16" || r.UpdatedAt.IsZero() || r.MarginLoan == nil || r.StockLoan == nil {
		t.Fatalf("r = %+v", r)
	}
	if r.MarginLoan.NewQuantity.String() != "125300" || r.MarginLoan.BalanceQuantity.String() != "2513400" || r.MarginLoan.BalanceRate.String() != "0.0042" || r.MarginLoan.TradingRate.String() != "0.09" {
		t.Errorf("MarginLoan = %+v", r.MarginLoan)
	}
	if r.StockLoan.BalanceQuantity.String() != "45200" || r.StockLoan.TradingRate.String() != "0.0004" {
		t.Errorf("StockLoan = %+v", r.StockLoan)
	}
}

// fixture = openapi 예시(daily)
func TestSecuritiesLending(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/stocks/005930/securities-lending", Query: url.Values{"count": {"1"}}}, 200, testutil.Fixture(t, "securities_lending.json"))
	defer done()
	page, err := New(hc).SecuritiesLending(context.Background(), "005930", TrendParams{Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.NextUntil == nil || len(page.Records) != 2 {
		t.Fatalf("page = %+v", page)
	}
	r := page.Records[0]
	if r.Date != "2026-07-17" || r.ExecutionQuantity.String() != "210500" || r.RepaymentQuantity.String() != "185300" || r.BalanceQuantity.String() != "15234000" || r.BalanceAmount.String() != "1218720000000" {
		t.Errorf("r = %+v", r)
	}
}

// params.Symbol 이 [A-Za-z0-9.-] 만 허용하므로 PathEscape 는 방어용 — '.' 이 경로에 그대로 남는지만 확인
func TestTrend_DottedSymbolPath(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/stocks/BRK.B/short-selling"}, 200, []byte(`{"result":{"nextUntil":null,"records":[]}}`))
	defer done()
	page, err := New(hc).ShortSelling(context.Background(), "BRK.B", TrendParams{})
	if err != nil {
		t.Fatal(err)
	}
	if page.NextUntil != nil || len(page.Records) != 0 {
		t.Errorf("page = %+v", page)
	}
}

func TestTrend_RequiresSymbol(t *testing.T) {
	c := New(nil) // nil client: 검증이 요청 전에 실패해야 한다
	if _, err := c.InvestorTrading(context.Background(), "", TrendParams{}); err == nil {
		t.Error("InvestorTrading empty symbol")
	}
	if _, err := c.InvestorTrading(context.Background(), " 005930", TrendParams{}); err == nil {
		t.Error("InvestorTrading symbol with whitespace")
	}
	if _, err := c.Warnings(context.Background(), ""); err == nil {
		t.Error("Warnings empty symbol")
	}
}
EOF
go test ./stockinfo/ 2>&1 | head -5
```
Expected: 컴파일 에러.

- [ ] **Step 3: 구현**

```bash
cat > stockinfo/client.go << 'EOF'
// Package stockinfo 는 토스 Open API 종목 정보(Stock Info) 그룹 — 종목 메타·전체 목록·매수 유의사항·
// 투자자별/프로그램/공매도/신용/대차 매매동향. toss.Client.StockInfo 로 접근한다.
// 매매동향 5종은 국내(KR) 종목만 지원한다 — 해외 심볼을 넘기면 400 unsupported-market *APIError 가 반환된다.
package stockinfo

import "github.com/kenshin579/toss-go/internal/httpclient"

// Client 는 종목 정보 sub-client.
type Client struct {
	http *httpclient.Client
}

// New 는 internal 용도 — toss.NewClient 가 호출한다.
func New(hc *httpclient.Client) *Client { return &Client{http: hc} }
EOF
cat > stockinfo/stocks.go << 'EOF'
package stockinfo

import (
	"context"
	"net/url"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// KRMarketDetail 은 국내 종목의 시장 상세. 해외 종목은 nil.
type KRMarketDetail struct {
	LiquidationTrading  bool  `json:"liquidationTrading"`  // 정리매매 여부
	NXTSupported        bool  `json:"nxtSupported"`        // NXT 거래 지원 여부
	KRXTradingSuspended bool  `json:"krxTradingSuspended"` // KRX 거래정지 여부
	NXTTradingSuspended *bool `json:"nxtTradingSuspended"` // NXT 거래정지 여부. NXT 미지원 종목은 nil
}

// Stock 은 종목 기본 정보.
type Stock struct {
	Symbol             string                 `json:"symbol"`
	Name               string                 `json:"name"`
	EnglishName        string                 `json:"englishName"`
	ISINCode           string                 `json:"isinCode"`
	Market             tosstypes.Market       `json:"market"`
	SecurityType       tosstypes.SecurityType `json:"securityType"`
	IsCommonShare      bool                   `json:"isCommonShare"`
	Status             tosstypes.StockStatus  `json:"status"`
	Currency           tosstypes.Currency     `json:"currency"`
	ListDate           *tosstypes.Date        `json:"listDate"`
	DelistDate         *tosstypes.Date        `json:"delistDate"`
	SharesOutstanding  decimal.Decimal        `json:"sharesOutstanding"`
	LeverageFactor     *decimal.Decimal       `json:"leverageFactor"` // 레버리지 ETF/ETN 배수. 해당 없으면 nil
	KoreanMarketDetail *KRMarketDetail        `json:"koreanMarketDetail"`
}

// Stocks 는 여러 종목의 기본 정보를 조회한다(GET /api/v1/stocks). 최대 200개. 없는 심볼은 결과에서 빠진다.
func (c *Client) Stocks(ctx context.Context, symbols ...string) ([]Stock, error) {
	joined, err := params.Symbols(symbols)
	if err != nil {
		return nil, err
	}
	return fetch.List[Stock](ctx, c.http, "/api/v1/stocks", url.Values{"symbols": {joined}})
}

// ListedStock 은 마켓별 전체 종목 목록의 항목.
type ListedStock struct {
	Symbol        string                 `json:"symbol"`
	Name          string                 `json:"name"`
	SecurityType  tosstypes.SecurityType `json:"securityType"`
	IsCommonShare bool                   `json:"isCommonShare"`
	ISINCode      string                 `json:"isinCode"`
}

// ListStocksParams 는 ListStocks 인자.
type ListStocksParams struct {
	Market       tosstypes.Market       // 필수
	Status       tosstypes.StockStatus  // 비우면 서버 기본값(ACTIVE)
	SecurityType tosstypes.SecurityType // 비우면 전체
	CommonShare  *bool                  // nil 이면 전체
}

// ListStocks 는 마켓의 전체 종목을 조회한다(GET /api/v1/stocks/all). Rate limit 그룹 STOCK_ALL(1/s).
func (c *Client) ListStocks(ctx context.Context, p ListStocksParams) ([]ListedStock, error) {
	if err := params.Require("market", string(p.Market)); err != nil {
		return nil, err
	}
	q := url.Values{"market": {string(p.Market)}}
	params.Str(q, "status", string(p.Status))
	params.Str(q, "securityType", string(p.SecurityType))
	params.Bool(q, "commonShare", p.CommonShare)
	return fetch.List[ListedStock](ctx, c.http, "/api/v1/stocks/all", q)
}
EOF
cat > stockinfo/warnings.go << 'EOF'
package stockinfo

import (
	"context"
	"net/url"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// Warning 은 매수 유의사항 1건.
type Warning struct {
	WarningType tosstypes.WarningType `json:"warningType"`
	Exchange    *string               `json:"exchange"`
	StartDate   *tosstypes.Date       `json:"startDate"`
	EndDate     *tosstypes.Date       `json:"endDate"`
}

// Warnings 는 종목의 매수 유의사항을 조회한다(GET /api/v1/stocks/{symbol}/warnings). 없으면 빈 슬라이스.
func (c *Client) Warnings(ctx context.Context, symbol string) ([]Warning, error) {
	if err := params.Symbol(symbol); err != nil {
		return nil, err
	}
	return fetch.List[Warning](ctx, c.http, "/api/v1/stocks/"+url.PathEscape(symbol)+"/warnings", nil)
}
EOF
cat > stockinfo/trend.go << 'EOF'
package stockinfo

import (
	"context"
	"net/url"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/httpclient"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// TrendParams 는 매매동향 5종의 공통 인자. 국내(KR) 종목만 지원한다.
type TrendParams struct {
	Count int            // 최대 100, 0 이면 서버 기본값(10)
	Until tosstypes.Date // 이 날짜 이하(inclusive)의 기록만. 비우면 최신부터
}

// TrendPage 는 매매동향 한 페이지. NextUntil 을 다음 요청의 Until 로 넘기면 이어서 조회한다.
type TrendPage[T any] struct {
	Records   []T             `json:"records"`
	NextUntil *tosstypes.Date `json:"nextUntil"` // 더 없으면 nil
}

// TradingVolume 은 매수/매도/순매수 거래량.
type TradingVolume struct {
	BuyVolume    decimal.Decimal `json:"buyVolume"`
	SellVolume   decimal.Decimal `json:"sellVolume"`
	NetBuyVolume decimal.Decimal `json:"netBuyVolume"`
}

// InstitutionBreakdown 은 기관 세부 7개 분류.
type InstitutionBreakdown struct {
	FinancialInvestment       TradingVolume `json:"financialInvestment"`
	Insurance                 TradingVolume `json:"insurance"`
	Trust                     TradingVolume `json:"trust"`
	PrivateEquityFund         TradingVolume `json:"privateEquityFund"`
	Bank                      TradingVolume `json:"bank"`
	OtherFinancialInstitution TradingVolume `json:"otherFinancialInstitution"`
	PensionFund               TradingVolume `json:"pensionFund"`
}

// InstitutionTradingVolume 은 기관 합계 + 세부 분류(잠정치에는 nil).
type InstitutionTradingVolume struct {
	TradingVolume
	Breakdown *InstitutionBreakdown `json:"breakdown"`
}

// ForeignerHolding 은 외국인 보유 현황.
type ForeignerHolding struct {
	HoldingQuantity decimal.Decimal `json:"holdingQuantity"`
	LimitQuantity   decimal.Decimal `json:"limitQuantity"`
	HoldingRate     decimal.Decimal `json:"holdingRate"`
}

// CFDBalance 는 CFD 잔고 현황(T+1 반영).
type CFDBalance struct {
	BuyBalanceQuantity  decimal.Decimal `json:"buyBalanceQuantity"`
	BuyBalanceRate      decimal.Decimal `json:"buyBalanceRate"`
	SellBalanceQuantity decimal.Decimal `json:"sellBalanceQuantity"`
	SellBalanceRate     decimal.Decimal `json:"sellBalanceRate"`
}

// InvestorTradingRecord 는 투자자별 매매동향 1일. 당일 잠정 기록에는 Individual/OtherCorporation/
// Institution.Breakdown 이 nil 이며 확정치가 반영되는 저녁부터 채워진다.
type InvestorTradingRecord struct {
	Date             tosstypes.Date           `json:"date"`
	UpdatedAt        time.Time                `json:"updatedAt"`
	Individual       *TradingVolume           `json:"individual"`
	Foreigner        TradingVolume            `json:"foreigner"`
	Institution      InstitutionTradingVolume `json:"institution"`
	OtherCorporation *TradingVolume           `json:"otherCorporation"`
	ForeignerHolding *ForeignerHolding        `json:"foreignerHolding"`
	CFD              *CFDBalance              `json:"cfd"`
}

// ProgramTradesRecord 는 프로그램매매 동향 1일.
type ProgramTradesRecord struct {
	Date         tosstypes.Date `json:"date"`
	Arbitrage    TradingVolume  `json:"arbitrage"`    // 차익거래
	NonArbitrage TradingVolume  `json:"nonArbitrage"` // 비차익거래
}

// ShortSellingRecord 는 공매도 동향 1일.
type ShortSellingRecord struct {
	Date                   tosstypes.Date   `json:"date"`
	UpdatedAt              time.Time        `json:"updatedAt"`
	ShortSellingVolume     decimal.Decimal  `json:"shortSellingVolume"`
	ShortSellingAmount     decimal.Decimal  `json:"shortSellingAmount"`
	ShortSellingVolumeRate *decimal.Decimal `json:"shortSellingVolumeRate"` // 거래량 대비 비율
	ShortSellingAmountRate *decimal.Decimal `json:"shortSellingAmountRate"` // 거래대금 대비 비율
}

// CreditTradeDetail 은 신용융자/신용대주 상세.
type CreditTradeDetail struct {
	NewQuantity     decimal.Decimal `json:"newQuantity"`
	ReturnQuantity  decimal.Decimal `json:"returnQuantity"`
	BalanceQuantity decimal.Decimal `json:"balanceQuantity"`
	BalanceRate     decimal.Decimal `json:"balanceRate"`
	TradingRate     decimal.Decimal `json:"tradingRate"`
}

// CreditTradesRecord 는 신용거래 동향 1일. 데이터가 없는 항목은 nil.
type CreditTradesRecord struct {
	Date       tosstypes.Date     `json:"date"`
	UpdatedAt  time.Time          `json:"updatedAt"`
	MarginLoan *CreditTradeDetail `json:"marginLoan"` // 신용융자
	StockLoan  *CreditTradeDetail `json:"stockLoan"`  // 신용대주
}

// SecuritiesLendingRecord 는 대차거래 동향 1일.
type SecuritiesLendingRecord struct {
	Date              tosstypes.Date  `json:"date"`
	UpdatedAt         time.Time       `json:"updatedAt"`
	ExecutionQuantity decimal.Decimal `json:"executionQuantity"`
	RepaymentQuantity decimal.Decimal `json:"repaymentQuantity"`
	BalanceQuantity   decimal.Decimal `json:"balanceQuantity"`
	BalanceAmount     decimal.Decimal `json:"balanceAmount"`
}

// InvestorTradingPage 는 InvestorTrading 의 응답 페이지.
type InvestorTradingPage = TrendPage[InvestorTradingRecord]

// ProgramTradesPage 는 ProgramTrades 의 응답 페이지.
type ProgramTradesPage = TrendPage[ProgramTradesRecord]

// ShortSellingPage 는 ShortSelling 의 응답 페이지.
type ShortSellingPage = TrendPage[ShortSellingRecord]

// CreditTradesPage 는 CreditTrades 의 응답 페이지.
type CreditTradesPage = TrendPage[CreditTradesRecord]

// SecuritiesLendingPage 는 SecuritiesLending 의 응답 페이지.
type SecuritiesLendingPage = TrendPage[SecuritiesLendingRecord]

// InvestorTrading 은 투자자별 매매동향(GET /api/v1/stocks/{symbol}/investor-trading). 국내 종목만 지원(해외 심볼 → 400 unsupported-market).
func (c *Client) InvestorTrading(ctx context.Context, symbol string, p TrendParams) (*InvestorTradingPage, error) {
	return fetchTrend[InvestorTradingRecord](ctx, c.http, symbol, "investor-trading", p)
}

// ProgramTrades 는 프로그램매매 동향(GET /api/v1/stocks/{symbol}/program-trades). 국내 종목만 지원(해외 심볼 → 400 unsupported-market).
func (c *Client) ProgramTrades(ctx context.Context, symbol string, p TrendParams) (*ProgramTradesPage, error) {
	return fetchTrend[ProgramTradesRecord](ctx, c.http, symbol, "program-trades", p)
}

// ShortSelling 은 공매도 동향(GET /api/v1/stocks/{symbol}/short-selling). 국내 종목만 지원(해외 심볼 → 400 unsupported-market).
func (c *Client) ShortSelling(ctx context.Context, symbol string, p TrendParams) (*ShortSellingPage, error) {
	return fetchTrend[ShortSellingRecord](ctx, c.http, symbol, "short-selling", p)
}

// CreditTrades 는 신용거래 동향(GET /api/v1/stocks/{symbol}/credit-trades). 국내 종목만 지원(해외 심볼 → 400 unsupported-market).
func (c *Client) CreditTrades(ctx context.Context, symbol string, p TrendParams) (*CreditTradesPage, error) {
	return fetchTrend[CreditTradesRecord](ctx, c.http, symbol, "credit-trades", p)
}

// SecuritiesLending 은 대차거래 동향(GET /api/v1/stocks/{symbol}/securities-lending). 국내 종목만 지원(해외 심볼 → 400 unsupported-market).
func (c *Client) SecuritiesLending(ctx context.Context, symbol string, p TrendParams) (*SecuritiesLendingPage, error) {
	return fetchTrend[SecuritiesLendingRecord](ctx, c.http, symbol, "securities-lending", p)
}

func fetchTrend[T any](ctx context.Context, hc *httpclient.Client, symbol, segment string, p TrendParams) (*TrendPage[T], error) {
	if err := params.Symbol(symbol); err != nil {
		return nil, err
	}
	q := url.Values{}
	params.Int(q, "count", p.Count)
	params.Date(q, "until", p.Until)
	return fetch.One[TrendPage[T]](ctx, hc, "/api/v1/stocks/"+url.PathEscape(symbol)+"/"+segment, q)
}
EOF
gofmt -l stockinfo; go vet ./stockinfo/ && go test ./stockinfo/ -v 2>&1 | tail -16
```
Expected: gofmt 출력 없음, 12 tests PASS. `TestCreditTrades`/`TestSecuritiesLending` 이 캡처 데이터 때문에 실패하면(예: 레코드 0건) 캡처 명령의 `count` 나 날짜를 바꿔 다시 캡처한다 — 테스트 조건을 완화하지 않는다.

- [ ] **Step 4: 커밋**

```bash
git add stockinfo testdata && git commit -m "feat(stockinfo): 종목 메타·전체 목록·유의사항·매매동향 5종 8 ops

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 6: `marketinfo` (3 ops)

**Files:**
- Create: `marketinfo/client.go`, `marketinfo/exchange_rate.go`, `marketinfo/calendar.go`, `marketinfo/marketinfo_test.go`
- Move: `testdata/captured/{exchange_rate_baseCurrency_USD_quoteCurrency_KRW_,market_calendar_KR_}.json` → `marketinfo/testdata/`
- Capture: `marketinfo/testdata/market_calendar_us.json`

- [ ] **Step 1: fixture 이동 + US 캘린더 캡처**

```bash
mkdir -p marketinfo/testdata && git mv testdata/captured/exchange_rate_baseCurrency_USD_quoteCurrency_KRW_.json marketinfo/testdata/exchange_rate.json && git mv testdata/captured/market_calendar_KR_.json marketinfo/testdata/market_calendar_kr.json
eval "$(grep -E '^export TOSS_CLIENT_(ID|SECRET)=' ~/.zshrc)"
TOKEN=$(curl -s --compressed -X POST https://openapi.tossinvest.com/oauth2/token -H 'Content-Type: application/x-www-form-urlencoded' -d grant_type=client_credentials -d "client_id=$TOSS_CLIENT_ID" -d "client_secret=$TOSS_CLIENT_SECRET" | jq -r .access_token)
if [ -n "$TOKEN" ] && [ "$TOKEN" != "null" ]; then echo TOKEN_OK; else echo "TOKEN_UNAVAILABLE → openapi.json 예시로 대체"; fi
J=docs/api/openapi.json; ex() { jq --arg p "$1" --arg n "$2" '.paths[$p].get.responses."200".content."application/json".examples[$n].value' $J; }
if [ -n "$TOKEN" ] && [ "$TOKEN" != "null" ]; then
  curl -s --compressed -H "Authorization: Bearer $TOKEN" "https://openapi.tossinvest.com/api/v1/market-calendar/US" | jq . > marketinfo/testdata/market_calendar_us.json
else
  ex "/api/v1/market-calendar/US" businessDay > marketinfo/testdata/market_calendar_us.json
fi
jq -c '.result.today | {date, day: (.dayMarket != null), pre: (.preMarket != null), reg: (.regularMarket != null), after: (.afterMarket != null)}' marketinfo/testdata/market_calendar_us.json
```
Expected: `TOKEN_OK`(또는 대체 안내) 와 `{"date":"YYYY-MM-DD","day":...,"pre":...}` 형태. 세션이 휴장이면 `false`(null)일 수 있으며 테스트는 `regularMarket` 만 non-nil 가정하지 않고 날짜만 검사한다.

- [ ] **Step 2: 실패 테스트 작성**

```bash
cat > marketinfo/marketinfo_test.go << 'EOF'
package marketinfo

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/kenshin579/toss-go/internal/testutil"
	"github.com/kenshin579/toss-go/tosstypes"
)

func TestExchangeRate(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/exchange-rate", Query: url.Values{"baseCurrency": {"USD"}, "quoteCurrency": {"KRW"}}}, 200, testutil.Fixture(t, "exchange_rate.json"))
	defer done()
	fx, err := New(hc).ExchangeRate(context.Background(), tosstypes.CurrencyUSD, tosstypes.CurrencyKRW, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fx.BaseCurrency != tosstypes.CurrencyUSD || fx.QuoteCurrency != tosstypes.CurrencyKRW || fx.Rate.String() != "1359.63" || fx.MidRate.String() != "1359.13" || fx.BasisPoint.String() != "4" || fx.RateChangeType != tosstypes.RateChangeTypeDown {
		t.Errorf("fx = %+v", fx)
	}
	if fx.ValidFrom.IsZero() || !fx.ValidUntil.After(fx.ValidFrom) {
		t.Errorf("validity = %v ~ %v", fx.ValidFrom, fx.ValidUntil)
	}
}

func TestExchangeRate_At(t *testing.T) {
	at := time.Date(2026, 9, 1, 9, 0, 0, 0, tosstypes.KST)
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/exchange-rate", Query: url.Values{"baseCurrency": {"USD"}, "quoteCurrency": {"KRW"}, "dateTime": {"2026-09-01T09:00:00+09:00"}}}, 200, testutil.Fixture(t, "exchange_rate.json"))
	defer done()
	if _, err := New(hc).ExchangeRate(context.Background(), tosstypes.CurrencyUSD, tosstypes.CurrencyKRW, &at); err != nil {
		t.Fatal(err)
	}
}

func TestExchangeRate_Validation(t *testing.T) {
	c := New(nil) // nil client: 검증이 요청 전에 실패해야 한다
	if _, err := c.ExchangeRate(context.Background(), "", tosstypes.CurrencyKRW, nil); err == nil {
		t.Error("want error for empty base")
	}
	if _, err := c.ExchangeRate(context.Background(), tosstypes.CurrencyUSD, "", nil); err == nil {
		t.Error("want error for empty quote")
	}
}

func TestKRMarketCalendar(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/market-calendar/KR", Query: url.Values{"date": {"2026-09-04"}}}, 200, testutil.Fixture(t, "market_calendar_kr.json"))
	defer done()
	cal, err := New(hc).KRMarketCalendar(context.Background(), "2026-09-04")
	if err != nil {
		t.Fatal(err)
	}
	if cal.Today.Date != "2026-09-04" || cal.PreviousBusinessDay.Date == "" || cal.NextBusinessDay.Date == "" {
		t.Errorf("cal = %+v", cal)
	}
	ih := cal.Today.Integrated
	if ih == nil || ih.PreMarket == nil || ih.RegularMarket == nil || ih.AfterMarket == nil {
		t.Fatalf("Integrated = %+v", ih)
	}
	if ih.RegularMarket.StartTime.Hour() != 9 || ih.RegularMarket.EndTime.Hour() != 15 || ih.RegularMarket.SinglePriceAuctionStartTime == nil || ih.RegularMarket.SinglePriceAuctionStartTime.Minute() != 20 {
		t.Errorf("RegularMarket = %+v", ih.RegularMarket)
	}
	if ih.AfterMarket.SinglePriceAuctionEndTime == nil || ih.AfterMarket.SinglePriceAuctionEndTime.Minute() != 40 {
		t.Errorf("AfterMarket = %+v", ih.AfterMarket)
	}
}

func TestKRMarketCalendar_NoDate(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/market-calendar/KR"}, 200, testutil.Fixture(t, "market_calendar_kr.json"))
	defer done()
	if _, err := New(hc).KRMarketCalendar(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
}

func TestKRMarketCalendar_Holiday(t *testing.T) {
	body := []byte(`{"result":{"today":{"date":"2026-10-03","integrated":null},"previousBusinessDay":{"date":"2026-10-02","integrated":{"preMarket":null,"regularMarket":{"startTime":"2026-10-02T09:00:00+09:00","singlePriceAuctionStartTime":null,"endTime":"2026-10-02T15:30:00+09:00"},"afterMarket":null}},"nextBusinessDay":{"date":"2026-10-05","integrated":null}}}`)
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/market-calendar/KR"}, 200, body)
	defer done()
	cal, err := New(hc).KRMarketCalendar(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if cal.Today.Integrated != nil || cal.PreviousBusinessDay.Integrated == nil || cal.PreviousBusinessDay.Integrated.PreMarket != nil || cal.PreviousBusinessDay.Integrated.RegularMarket.SinglePriceAuctionStartTime != nil {
		t.Errorf("cal = %+v", cal)
	}
}

func TestUSMarketCalendar(t *testing.T) {
	// fixture = openapi 예시(businessDay): 2026-03-25, 4개 세션 모두 존재, 정규장은 KST 자정을 넘김
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/market-calendar/US"}, 200, testutil.Fixture(t, "market_calendar_us.json"))
	defer done()
	cal, err := New(hc).USMarketCalendar(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	td := cal.Today
	if td.Date != "2026-03-25" || cal.PreviousBusinessDay.Date == "" || cal.NextBusinessDay.Date == "" {
		t.Errorf("dates = %s %s %s", td.Date, cal.PreviousBusinessDay.Date, cal.NextBusinessDay.Date)
	}
	if td.DayMarket == nil || td.DayMarket.StartTime.Hour() != 9 || td.DayMarket.EndTime.Hour() != 16 || td.DayMarket.EndTime.Minute() != 50 {
		t.Errorf("DayMarket = %+v", td.DayMarket)
	}
	if td.PreMarket == nil || td.PreMarket.StartTime.Hour() != 17 || td.PreMarket.EndTime.Hour() != 22 || td.PreMarket.EndTime.Minute() != 30 {
		t.Errorf("PreMarket = %+v", td.PreMarket)
	}
	if td.RegularMarket == nil || td.RegularMarket.StartTime.Hour() != 22 || td.RegularMarket.EndTime.Day() != 26 || td.RegularMarket.EndTime.Hour() != 5 {
		t.Errorf("RegularMarket = %+v", td.RegularMarket)
	}
	if td.AfterMarket == nil || td.AfterMarket.StartTime.Hour() != 5 || td.AfterMarket.EndTime.Hour() != 7 {
		t.Errorf("AfterMarket = %+v", td.AfterMarket)
	}
}

func TestUSMarketCalendar_Holiday(t *testing.T) {
	// openapi 예시(holidayToday): 오늘은 4개 세션 모두 null
	body := []byte(`{"result":{"today":{"date":"2026-07-03","dayMarket":null,"preMarket":null,"regularMarket":null,"afterMarket":null},"previousBusinessDay":{"date":"2026-07-02","dayMarket":{"startTime":"2026-07-02T09:00:00+09:00","endTime":"2026-07-02T16:50:00+09:00"},"preMarket":{"startTime":"2026-07-02T17:00:00+09:00","endTime":"2026-07-02T22:30:00+09:00"},"regularMarket":{"startTime":"2026-07-02T22:30:00+09:00","endTime":"2026-07-03T05:00:00+09:00"},"afterMarket":{"startTime":"2026-07-03T05:00:00+09:00","endTime":"2026-07-03T07:00:00+09:00"}},"nextBusinessDay":{"date":"2026-07-06","dayMarket":{"startTime":"2026-07-06T09:00:00+09:00","endTime":"2026-07-06T16:50:00+09:00"},"preMarket":{"startTime":"2026-07-06T17:00:00+09:00","endTime":"2026-07-06T22:30:00+09:00"},"regularMarket":{"startTime":"2026-07-06T22:30:00+09:00","endTime":"2026-07-07T05:00:00+09:00"},"afterMarket":{"startTime":"2026-07-07T05:00:00+09:00","endTime":"2026-07-07T07:00:00+09:00"}}}}`)
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/market-calendar/US"}, 200, body)
	defer done()
	cal, err := New(hc).USMarketCalendar(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	td := cal.Today
	if td.Date == "" || td.DayMarket != nil || td.PreMarket != nil || td.RegularMarket != nil || td.AfterMarket != nil {
		t.Errorf("today = %+v", td)
	}
	if cal.NextBusinessDay.RegularMarket == nil {
		t.Errorf("next business day must have sessions: %+v", cal.NextBusinessDay)
	}
}
EOF
go test ./marketinfo/ 2>&1 | head -5
```
Expected: 컴파일 에러.

- [ ] **Step 3: 구현**

```bash
cat > marketinfo/client.go << 'EOF'
// Package marketinfo 는 토스 Open API 시장 정보(Market Info) 그룹 — 환율·국내/해외 장 운영 정보.
// toss.Client.MarketInfo 로 접근한다.
package marketinfo

import "github.com/kenshin579/toss-go/internal/httpclient"

// Client 는 시장 정보 sub-client.
type Client struct {
	http *httpclient.Client
}

// New 는 internal 용도 — toss.NewClient 가 호출한다.
func New(hc *httpclient.Client) *Client { return &Client{http: hc} }
EOF
cat > marketinfo/exchange_rate.go << 'EOF'
package marketinfo

import (
	"context"
	"net/url"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// ExchangeRate 는 환율 고시.
type ExchangeRate struct {
	BaseCurrency   tosstypes.Currency       `json:"baseCurrency"`
	QuoteCurrency  tosstypes.Currency       `json:"quoteCurrency"`
	Rate           decimal.Decimal          `json:"rate"`       // 매수 환율(1 base = Rate quote). 참고용 표시 환율로 실제 체결 환율과 다를 수 있음
	MidRate        decimal.Decimal          `json:"midRate"`    // 매매기준율
	BasisPoint     decimal.Decimal          `json:"basisPoint"` // (rate - midRate) / midRate * 10000
	RateChangeType tosstypes.RateChangeType `json:"rateChangeType"`
	ValidFrom      time.Time                `json:"validFrom"`  // 고시 유효 시작
	ValidUntil     time.Time                `json:"validUntil"` // 고시 유효 종료(보통 1분 주기 갱신) — 캐시 TTL 힌트로 사용
}

// ExchangeRate 는 환율을 조회한다(GET /api/v1/exchange-rate). at 이 nil 이면 현재 고시. 해당 통화쌍/시각의 고시가 없으면 404 exchange-rate-not-found. KRW/USD 만 지원하며 base == quote 는 400 invalid-request.
func (c *Client) ExchangeRate(ctx context.Context, base, quote tosstypes.Currency, at *time.Time) (*ExchangeRate, error) {
	if err := params.Require("baseCurrency", string(base)); err != nil {
		return nil, err
	}
	if err := params.Require("quoteCurrency", string(quote)); err != nil {
		return nil, err
	}
	q := url.Values{"baseCurrency": {string(base)}, "quoteCurrency": {string(quote)}}
	params.Time(q, "dateTime", at)
	return fetch.One[ExchangeRate](ctx, c.http, "/api/v1/exchange-rate", q)
}
EOF
cat > marketinfo/calendar.go << 'EOF'
package marketinfo

import (
	"context"
	"net/url"
	"time"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// KRSession 은 국내 프리마켓/정규장 세션.
type KRSession struct {
	StartTime                   time.Time  `json:"startTime"`
	SinglePriceAuctionStartTime *time.Time `json:"singlePriceAuctionStartTime"` // 단일가 구간 시작. 슬롯별 의미는 KRIntegratedHours 필드 주석 참고. nil 이면 결손/휴장
	EndTime                     time.Time  `json:"endTime"`
}

// KRAfterMarketSession 은 국내 애프터마켓 세션.
type KRAfterMarketSession struct {
	StartTime                 time.Time  `json:"startTime"`
	SinglePriceAuctionEndTime *time.Time `json:"singlePriceAuctionEndTime"` // 단일가 구간 종료. 결손 시 nil
	EndTime                   time.Time  `json:"endTime"`
}

// KRIntegratedHours 는 통합(KRX+NXT) 거래 가능 시간. 휴장 세션은 nil.
type KRIntegratedHours struct {
	PreMarket     *KRSession            `json:"preMarket"`     // NXT 프리마켓(접속매매). SinglePriceAuctionStartTime = 시가단일가 시작(결손 시 nil). 휴장이면 nil
	RegularMarket *KRSession            `json:"regularMarket"` // KRX·NXT 정규장 합집합(가장 이른 시작~가장 늦은 종료). SinglePriceAuctionStartTime = 종가단일가 시작(KRX 기준, KRX 휴장 시 nil). 휴장이면 nil
	AfterMarket   *KRAfterMarketSession `json:"afterMarket"`   // 애프터마켓. 휴장이면 nil
}

// KRMarketDay 는 국내 하루 장 운영 정보. 휴장일이면 Integrated 가 nil.
type KRMarketDay struct {
	Date       tosstypes.Date     `json:"date"` // 영업일(KST 기준)
	Integrated *KRIntegratedHours `json:"integrated"`
}

// KRMarketCalendar 는 국내 장 운영 정보(오늘·직전·다음 영업일).
type KRMarketCalendar struct {
	Today               KRMarketDay `json:"today"`
	PreviousBusinessDay KRMarketDay `json:"previousBusinessDay"`
	NextBusinessDay     KRMarketDay `json:"nextBusinessDay"`
}

// USSession 은 해외 세션(시작·종료).
type USSession struct {
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
}

// USMarketDay 는 해외 하루 장 운영 정보. 휴장 세션은 nil.
type USMarketDay struct {
	Date          tosstypes.Date `json:"date"`      // 영업일(미국 현지 기준). 세션 시각은 모두 KST 이며 RegularMarket/AfterMarket 은 KST 자정을 넘어간다(예: 22:30 → 익일 05:00)
	DayMarket     *USSession     `json:"dayMarket"` // 데이마켓(토스증권)
	PreMarket     *USSession     `json:"preMarket"`
	RegularMarket *USSession     `json:"regularMarket"`
	AfterMarket   *USSession     `json:"afterMarket"`
}

// USMarketCalendar 는 해외 장 운영 정보(오늘·직전·다음 영업일).
type USMarketCalendar struct {
	Today               USMarketDay `json:"today"`
	PreviousBusinessDay USMarketDay `json:"previousBusinessDay"`
	NextBusinessDay     USMarketDay `json:"nextBusinessDay"`
}

// KRMarketCalendar 는 국내 장 운영 정보를 조회한다(GET /api/v1/market-calendar/KR). date 는 KST 기준, 비면 오늘. 지원 범위 밖 날짜는 400 unsupported-date.
func (c *Client) KRMarketCalendar(ctx context.Context, date tosstypes.Date) (*KRMarketCalendar, error) {
	q := url.Values{}
	params.Date(q, "date", date)
	return fetch.One[KRMarketCalendar](ctx, c.http, "/api/v1/market-calendar/KR", q)
}

// USMarketCalendar 는 해외 장 운영 정보를 조회한다(GET /api/v1/market-calendar/US). date 는 미국 현지 날짜(비면 오늘). tosstypes.NewDate 는 KST 변환이므로 미국 날짜는 Date(t.In(loc).Format("2006-01-02")) 로 직접 만든다.
func (c *Client) USMarketCalendar(ctx context.Context, date tosstypes.Date) (*USMarketCalendar, error) {
	q := url.Values{}
	params.Date(q, "date", date)
	return fetch.One[USMarketCalendar](ctx, c.http, "/api/v1/market-calendar/US", q)
}
EOF
gofmt -l marketinfo; go vet ./marketinfo/ && go test ./marketinfo/ -v 2>&1 | tail -10
```
Expected: gofmt 출력 없음, 8 tests PASS.

- [ ] **Step 4: 커밋**

```bash
git add marketinfo testdata && git commit -m "feat(marketinfo): 환율·국내/해외 장 운영 정보 3 ops

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 7: `ranking` (1 op) + `indicators` (3 ops)

**Files:**
- Create: `ranking/client.go`, `ranking/rankings.go`, `ranking/ranking_test.go`
- Create: `indicators/client.go`, `indicators/prices.go`, `indicators/candles.go`, `indicators/investor_trading.go`, `indicators/indicators_test.go`
- Move: `testdata/captured/rankings_type_TOP_GAINERS_marketCountry_KR_duration_1d_count_2_.json` → `ranking/testdata/rankings.json`, `testdata/captured/market_indicators_prices_symbols_KOSPI_.json` → `indicators/testdata/prices.json`
- Capture: `indicators/testdata/{candles,investor_trading}.json`
- Delete: 비어 있는 `testdata/captured/`

- [ ] **Step 1: fixture 이동 + 캡처**

```bash
mkdir -p ranking/testdata indicators/testdata && git mv testdata/captured/rankings_type_TOP_GAINERS_marketCountry_KR_duration_1d_count_2_.json ranking/testdata/rankings.json && git mv testdata/captured/market_indicators_prices_symbols_KOSPI_.json indicators/testdata/prices.json
eval "$(grep -E '^export TOSS_CLIENT_(ID|SECRET)=' ~/.zshrc)"
TOKEN=$(curl -s --compressed -X POST https://openapi.tossinvest.com/oauth2/token -H 'Content-Type: application/x-www-form-urlencoded' -d grant_type=client_credentials -d "client_id=$TOSS_CLIENT_ID" -d "client_secret=$TOSS_CLIENT_SECRET" | jq -r .access_token)
if [ -n "$TOKEN" ] && [ "$TOKEN" != "null" ]; then echo TOKEN_OK; else echo "TOKEN_UNAVAILABLE → openapi.json 예시로 대체"; fi
J=docs/api/openapi.json; ex() { jq --arg p "$1" --arg n "$2" '.paths[$p].get.responses."200".content."application/json".examples[$n].value' $J; }
B=https://openapi.tossinvest.com/api/v1
if [ -n "$TOKEN" ] && [ "$TOKEN" != "null" ]; then
  curl -s --compressed -H "Authorization: Bearer $TOKEN" "$B/market-indicators/KOSPI/candles?interval=1d&count=2" | jq . > indicators/testdata/candles.json; sleep 0.3
  curl -s --compressed -H "Authorization: Bearer $TOKEN" "$B/market-indicators/KOSPI/investor-trading?interval=1d&count=1" | jq . > indicators/testdata/investor_trading.json
else
  ex "/api/v1/market-indicators/{symbol}/candles" dailyCandles > indicators/testdata/candles.json
  ex "/api/v1/market-indicators/{symbol}/investor-trading" daily > indicators/testdata/investor_trading.json
fi
jq -c '.result | {n: (.candles|length), next: .nextBefore}' indicators/testdata/candles.json; jq -c '.result | {n: (.records|length), keys: (.records[0]|keys)}' indicators/testdata/investor_trading.json
rmdir testdata/captured testdata && echo CAPTURED_DIR_REMOVED
```
Expected: `TOKEN_OK`(또는 대체 안내), candles `n: 2`, investor_trading `n: 1`(예시 대체 시 2) 에 keys `date, updatedAt, individual, foreigner, institution, otherCorporation`, `CAPTURED_DIR_REMOVED`(모든 fixture 가 이동됐으므로 비어 있어야 함. 남은 파일이 있으면 어느 태스크가 빠뜨렸는지 확인).

- [ ] **Step 2: 실패 테스트 작성**

```bash
cat > ranking/ranking_test.go << 'EOF'
package ranking

import (
	"context"
	"net/url"
	"testing"

	"github.com/kenshin579/toss-go/internal/testutil"
	"github.com/kenshin579/toss-go/tosstypes"
)

func TestRankings(t *testing.T) {
	ex := true
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/rankings", Query: url.Values{
		"type": {"TOP_GAINERS"}, "marketCountry": {"KR"}, "duration": {"1d"}, "count": {"2"}, "excludeInvestmentCaution": {"true"},
	}}, 200, testutil.Fixture(t, "rankings.json"))
	defer done()
	r, err := New(hc).Rankings(context.Background(), RankingsParams{Type: tosstypes.RankingTypeTopGainers, MarketCountry: tosstypes.MarketCountryKR, Duration: tosstypes.RankingDuration1d, Count: 2, ExcludeInvestmentCaution: &ex})
	if err != nil {
		t.Fatal(err)
	}
	if r.RankedAt == nil || len(r.Rankings) != 2 {
		t.Fatalf("r = %+v", r)
	}
	it := r.Rankings[0]
	if it.Rank != 1 || it.Symbol != "459550" || it.Currency != tosstypes.CurrencyKRW || it.Price.LastPrice.String() != "2570" || it.Price.BasePrice.String() != "1979" || it.Price.ChangeRate == nil || it.Price.ChangeRate.String() != "0.2986" {
		t.Errorf("it = %+v", it)
	}
	if it.TradingVolume.String() != "13640212" || it.TradingAmount.String() != "31835155684" {
		t.Errorf("volume/amount = %s %s", it.TradingVolume, it.TradingAmount)
	}
}

func TestRankings_Validation(t *testing.T) {
	c := New(nil) // nil client: 검증이 요청 전에 실패해야 한다
	cases := []RankingsParams{
		{MarketCountry: tosstypes.MarketCountryKR, Duration: tosstypes.RankingDuration1d},
		{Type: tosstypes.RankingTypeTopGainers, Duration: tosstypes.RankingDuration1d},
		{Type: tosstypes.RankingTypeTopGainers, MarketCountry: tosstypes.MarketCountryKR},
	}
	for i, p := range cases {
		if _, err := c.Rankings(context.Background(), p); err == nil {
			t.Errorf("case %d: want error", i)
		}
	}
}
EOF
cat > indicators/indicators_test.go << 'EOF'
package indicators

import (
	"context"
	"net/url"
	"testing"

	"github.com/kenshin579/toss-go/internal/testutil"
	"github.com/kenshin579/toss-go/tosstypes"
)

func TestPrices(t *testing.T) {
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/market-indicators/prices", Query: url.Values{"symbols": {"KOSPI"}}}, 200, testutil.Fixture(t, "prices.json"))
	defer done()
	got, err := New(hc).Prices(context.Background(), "KOSPI")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Symbol != "KOSPI" || got[0].LastPrice.String() != "6579.48" || got[0].Timestamp != nil {
		t.Errorf("got = %+v", got)
	}
}

func TestPrices_NoSymbols(t *testing.T) {
	if _, err := New(nil).Prices(context.Background()); err == nil { // nil client: 검증이 요청 전에 실패해야 한다
		t.Error("want error")
	}
}

func TestCandles(t *testing.T) {
	// fixture = openapi 예시(dailyCandles)
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/market-indicators/KOSPI/candles", Query: url.Values{"interval": {"1d"}, "count": {"2"}}}, 200, testutil.Fixture(t, "candles.json"))
	defer done()
	page, err := New(hc).Candles(context.Background(), "KOSPI", CandlesParams{Interval: tosstypes.Interval1d, Count: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Candles) != 2 {
		t.Fatalf("page = %+v", page)
	}
	c0 := page.Candles[0]
	if c0.Timestamp.Year() != 2026 || c0.Timestamp.Month() != 6 || c0.Timestamp.Day() != 11 {
		t.Errorf("Timestamp = %v", c0.Timestamp)
	}
	if c0.OpenPrice.String() != "2798.32" || c0.HighPrice.String() != "2820.15" || c0.LowPrice.String() != "2790.1" || c0.ClosePrice.String() != "2812.45" || c0.Volume.String() != "542000000" {
		t.Errorf("candle = %+v", c0)
	}
	if page.NextBefore == nil || page.NextBefore.Day() != 10 {
		t.Errorf("NextBefore = %v", page.NextBefore)
	}
}

func TestCandles_Validation(t *testing.T) {
	c := New(nil) // nil client: 검증이 요청 전에 실패해야 한다
	if _, err := c.Candles(context.Background(), "", CandlesParams{Interval: tosstypes.Interval1d}); err == nil {
		t.Error("want error for empty symbol")
	}
	if _, err := c.Candles(context.Background(), "삼성", CandlesParams{Interval: tosstypes.Interval1d}); err == nil {
		t.Error("want error for non-ascii symbol")
	}
	if _, err := c.Candles(context.Background(), "KOSPI", CandlesParams{}); err == nil {
		t.Error("want error for empty interval")
	}
}

func TestInvestorTrading(t *testing.T) {
	// fixture = openapi 예시(daily)
	hc, done := testutil.NewServer(t, testutil.Expect{Path: "/api/v1/market-indicators/KOSPI/investor-trading", Query: url.Values{"interval": {"1d"}, "count": {"1"}, "until": {"2026-09-03"}}}, 200, testutil.Fixture(t, "investor_trading.json"))
	defer done()
	page, err := New(hc).InvestorTrading(context.Background(), "KOSPI", InvestorTradingParams{Interval: tosstypes.IndicatorInterval1d, Count: 1, Until: "2026-09-03"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 2 {
		t.Fatalf("page = %+v", page)
	}
	r := page.Records[0]
	if r.Date != "2026-06-11" || r.Institution.Breakdown.PensionFund.BuyAmount.String() != "500000000000" || r.Institution.Breakdown.PensionFund.SellAmount.String() != "490000000000" {
		t.Errorf("r = %+v", r)
	}
}

func TestInvestorTrading_Validation(t *testing.T) {
	c := New(nil) // nil client: 검증이 요청 전에 실패해야 한다
	if _, err := c.InvestorTrading(context.Background(), "", InvestorTradingParams{Interval: tosstypes.IndicatorInterval1d}); err == nil {
		t.Error("want error for empty symbol")
	}
	if _, err := c.InvestorTrading(context.Background(), "KOSPI", InvestorTradingParams{}); err == nil {
		t.Error("want error for empty interval")
	}
}
EOF
go test ./ranking/ ./indicators/ 2>&1 | head -5
```
Expected: 컴파일 에러.

- [ ] **Step 3: 구현**

```bash
cat > ranking/client.go << 'EOF'
// Package ranking 은 토스 Open API 주식 랭킹(Ranking) 그룹. toss.Client.Ranking 으로 접근한다.
package ranking

import "github.com/kenshin579/toss-go/internal/httpclient"

// Client 는 랭킹 sub-client.
type Client struct {
	http *httpclient.Client
}

// New 는 internal 용도 — toss.NewClient 가 호출한다.
func New(hc *httpclient.Client) *Client { return &Client{http: hc} }
EOF
cat > ranking/rankings.go << 'EOF'
package ranking

import (
	"context"
	"net/url"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// RankingPrice 는 랭킹 항목의 가격 정보.
type RankingPrice struct {
	LastPrice  decimal.Decimal  `json:"lastPrice"`
	BasePrice  decimal.Decimal  `json:"basePrice"`  // 기준가(전일 종가)
	ChangeRate *decimal.Decimal `json:"changeRate"` // 등락률(소수). 없으면 nil
}

// RankingItem 은 랭킹 1건.
type RankingItem struct {
	Rank          int                `json:"rank"`
	Symbol        string             `json:"symbol"`
	Currency      tosstypes.Currency `json:"currency"`
	Price         RankingPrice       `json:"price"`
	TradingVolume decimal.Decimal    `json:"tradingVolume"`
	TradingAmount decimal.Decimal    `json:"tradingAmount"`
}

// Rankings 는 랭킹 결과.
type Rankings struct {
	RankedAt *time.Time    `json:"rankedAt"`
	Rankings []RankingItem `json:"rankings"`
}

// RankingsParams 는 Rankings 인자. Type/MarketCountry/Duration 필수.
type RankingsParams struct {
	Type                     tosstypes.RankingType
	MarketCountry            tosstypes.MarketCountry
	Duration                 tosstypes.RankingDuration // TOP_GAINERS/TOP_LOSERS 는 realtime 미지원
	ExcludeInvestmentCaution *bool                     // 투자주의 종목 제외. nil 이면 서버 기본값
	Count                    int                       // 최대 100, 0 이면 서버 기본값(100)
}

// Rankings 는 주식 랭킹을 조회한다(GET /api/v1/rankings).
func (c *Client) Rankings(ctx context.Context, p RankingsParams) (*Rankings, error) {
	if err := params.Require("type", string(p.Type)); err != nil {
		return nil, err
	}
	if err := params.Require("marketCountry", string(p.MarketCountry)); err != nil {
		return nil, err
	}
	if err := params.Require("duration", string(p.Duration)); err != nil {
		return nil, err
	}
	q := url.Values{"type": {string(p.Type)}, "marketCountry": {string(p.MarketCountry)}, "duration": {string(p.Duration)}}
	params.Bool(q, "excludeInvestmentCaution", p.ExcludeInvestmentCaution)
	params.Int(q, "count", p.Count)
	return fetch.One[Rankings](ctx, c.http, "/api/v1/rankings", q)
}
EOF
cat > indicators/client.go << 'EOF'
// Package indicators 는 토스 Open API 시장 지표(Market Indicators) 그룹 — 지수 현재가·캔들·투자자별 매매대금.
// toss.Client.MarketIndicators 로 접근한다.
package indicators

import "github.com/kenshin579/toss-go/internal/httpclient"

// Client 는 시장 지표 sub-client.
type Client struct {
	http *httpclient.Client
}

// New 는 internal 용도 — toss.NewClient 가 호출한다.
func New(hc *httpclient.Client) *Client { return &Client{http: hc} }
EOF
cat > indicators/prices.go << 'EOF'
package indicators

import (
	"context"
	"net/url"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
)

// Price 는 시장 지표(지수) 현재가.
type Price struct {
	Symbol    string          `json:"symbol"`
	Timestamp *time.Time      `json:"timestamp"` // 없으면 nil
	LastPrice decimal.Decimal `json:"lastPrice"`
}

// Prices 는 시장 지표 현재가를 조회한다(GET /api/v1/market-indicators/prices). 최대 200개. 예: KOSPI, KOSDAQ.
// 지원하지 않는 심볼은 400 unsupported-symbol, 잘못된 요청은 400 invalid-request.
func (c *Client) Prices(ctx context.Context, symbols ...string) ([]Price, error) {
	joined, err := params.Symbols(symbols)
	if err != nil {
		return nil, err
	}
	return fetch.List[Price](ctx, c.http, "/api/v1/market-indicators/prices", url.Values{"symbols": {joined}})
}
EOF
cat > indicators/candles.go << 'EOF'
package indicators

import (
	"context"
	"net/url"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// Candle 은 시장 지표 OHLCV 봉 1개.
type Candle struct {
	Timestamp  time.Time       `json:"timestamp"`
	OpenPrice  decimal.Decimal `json:"openPrice"`
	HighPrice  decimal.Decimal `json:"highPrice"`
	LowPrice   decimal.Decimal `json:"lowPrice"`
	ClosePrice decimal.Decimal `json:"closePrice"`
	Volume     decimal.Decimal `json:"volume"`
}

// CandlePage 는 캔들 한 페이지. NextBefore 를 다음 요청의 Before 로 넘기면 이어서 조회한다.
type CandlePage struct {
	Candles    []Candle   `json:"candles"`
	NextBefore *time.Time `json:"nextBefore"`
}

// CandlesParams 는 Candles 인자.
type CandlesParams struct {
	Interval tosstypes.Interval // 필수 (1m, 1d)
	Count    int                // 최대 200, 0 이면 서버 기본값(100)
	Before   *time.Time         // 이 시각 이하의 봉만. nil 이면 최신부터
}

// Candles 는 시장 지표 캔들을 조회한다(GET /api/v1/market-indicators/{symbol}/candles).
// 지원하지 않는 심볼은 400 unsupported-symbol, 잘못된 요청은 400 invalid-request.
func (c *Client) Candles(ctx context.Context, symbol string, p CandlesParams) (*CandlePage, error) {
	if err := params.Symbol(symbol); err != nil {
		return nil, err
	}
	if err := params.Require("interval", string(p.Interval)); err != nil {
		return nil, err
	}
	q := url.Values{"interval": {string(p.Interval)}}
	params.Int(q, "count", p.Count)
	params.Time(q, "before", p.Before)
	return fetch.One[CandlePage](ctx, c.http, "/api/v1/market-indicators/"+url.PathEscape(symbol)+"/candles", q)
}
EOF
cat > indicators/investor_trading.go << 'EOF'
package indicators

import (
	"context"
	"net/url"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/internal/fetch"
	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// TradingAmount 는 매수/매도 대금.
type TradingAmount struct {
	BuyAmount  decimal.Decimal `json:"buyAmount"`
	SellAmount decimal.Decimal `json:"sellAmount"`
}

// InstitutionBreakdown 은 기관 세부 7개 분류.
type InstitutionBreakdown struct {
	FinancialInvestment       TradingAmount `json:"financialInvestment"`
	Insurance                 TradingAmount `json:"insurance"`
	Trust                     TradingAmount `json:"trust"`
	PrivateEquityFund         TradingAmount `json:"privateEquityFund"`
	Bank                      TradingAmount `json:"bank"`
	OtherFinancialInstitution TradingAmount `json:"otherFinancialInstitution"`
	PensionFund               TradingAmount `json:"pensionFund"`
}

// InstitutionTradingAmount 는 기관 합계 + 세부 분류.
type InstitutionTradingAmount struct {
	TradingAmount
	Breakdown InstitutionBreakdown `json:"breakdown"`
}

// InvestorTradingRecord 는 투자자별 매매대금 1구간.
type InvestorTradingRecord struct {
	Date             tosstypes.Date           `json:"date"`
	UpdatedAt        time.Time                `json:"updatedAt"`
	Individual       TradingAmount            `json:"individual"`
	Foreigner        TradingAmount            `json:"foreigner"`
	Institution      InstitutionTradingAmount `json:"institution"`
	OtherCorporation TradingAmount            `json:"otherCorporation"`
}

// InvestorTradingPage 는 매매대금 한 페이지. NextUntil 을 다음 요청의 Until 로 넘기면 이어서 조회한다.
type InvestorTradingPage struct {
	Records   []InvestorTradingRecord `json:"records"`
	NextUntil *tosstypes.Date         `json:"nextUntil"`
}

// InvestorTradingParams 는 InvestorTrading 인자.
type InvestorTradingParams struct {
	Interval tosstypes.IndicatorInterval // 필수 (1d, 1w, 1mo, 1y)
	Count    int                         // 최대 100, 0 이면 서버 기본값(10)
	Until    tosstypes.Date              // 이 날짜(KST) 이하의 기록만. 비우면 최신부터
}

// InvestorTrading 은 투자자별 매매대금을 조회한다(GET /api/v1/market-indicators/{symbol}/investor-trading). Until 은 KST 기준 날짜.
// KOSPI, KOSDAQ 만 지원한다. 지원하지 않는 심볼은 400 unsupported-symbol, 잘못된 요청은 400 invalid-request.
func (c *Client) InvestorTrading(ctx context.Context, symbol string, p InvestorTradingParams) (*InvestorTradingPage, error) {
	if err := params.Symbol(symbol); err != nil {
		return nil, err
	}
	if err := params.Require("interval", string(p.Interval)); err != nil {
		return nil, err
	}
	q := url.Values{"interval": {string(p.Interval)}}
	params.Int(q, "count", p.Count)
	params.Date(q, "until", p.Until)
	return fetch.One[InvestorTradingPage](ctx, c.http, "/api/v1/market-indicators/"+url.PathEscape(symbol)+"/investor-trading", q)
}
EOF
gofmt -l ranking indicators; go vet ./ranking/ ./indicators/ && go test ./ranking/ ./indicators/ -v 2>&1 | grep -E '^(=== RUN|--- (PASS|FAIL)|ok|FAIL)' | grep -c -- '--- PASS'
```
Expected: gofmt 출력 없음, `8` (PASS 8건: ranking 2 + indicators 6).

- [ ] **Step 4: 커밋**

```bash
git add ranking indicators testdata && git commit -m "feat(ranking,indicators): 주식 랭킹 1 op + 시장 지표 3 ops

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 8: 루트 `toss` 패키지 — Client / Option / env / errors

**Files:**
- Create: `client.go`, `config.go`, `from_env.go`, `errors.go`, `client_test.go`

- [ ] **Step 1: 실패 테스트 작성**

```bash
cat > client_test.go << 'EOF'
package toss

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kenshin579/toss-go/internal/httpclient"
)

func TestNewClient_RequiresCredentials(t *testing.T) {
	if _, err := NewClient("", "s"); err == nil {
		t.Error("want error for empty clientID")
	}
	if _, err := NewClient("i", ""); err == nil {
		t.Error("want error for empty clientSecret")
	}
}

func TestNewClient_WiresGroups(t *testing.T) {
	c, err := NewClient("i", "s")
	if err != nil {
		t.Fatal(err)
	}
	if c.MarketData == nil || c.StockInfo == nil || c.MarketInfo == nil || c.Ranking == nil || c.MarketIndicators == nil {
		t.Errorf("group clients not wired: %+v", c)
	}
}

func TestNewClientFromEnv(t *testing.T) {
	t.Setenv("TOSS_CLIENT_ID", "")
	t.Setenv("TOSS_CLIENT_SECRET", "")
	if _, err := NewClientFromEnv(); err == nil {
		t.Error("want error when env missing")
	}
	t.Setenv("TOSS_CLIENT_ID", "id")
	t.Setenv("TOSS_CLIENT_SECRET", "sec")
	if _, err := NewClientFromEnv(); err != nil {
		t.Errorf("NewClientFromEnv: %v", err)
	}
}

// 토큰 발급 + 첫 API 호출까지 루트에서 end-to-end 로 확인한다.
func TestClient_EndToEnd(t *testing.T) {
	var tokenCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth2/token":
			atomic.AddInt32(&tokenCalls, 1)
			_, _ = w.Write([]byte(`{"access_token":"T","token_type":"Bearer","expires_in":3600}`))
		case "/api/v1/prices":
			if r.Header.Get("Authorization") != "Bearer T" {
				t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
			}
			_, _ = w.Write([]byte(`{"result":[{"symbol":"005930","timestamp":null,"lastPrice":"1","currency":"KRW"}]}`))
		case "/api/v1/stocks/X/warnings":
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"error":{"requestId":"r","code":"stock-not-found","message":"no"}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c, err := NewClient("i", "s", WithBaseURL(srv.URL), WithTimeout(5*time.Second), WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tok, err := c.AccessToken(ctx)
	if err != nil || tok != "T" {
		t.Fatalf("AccessToken = %q, %v", tok, err)
	}
	ps, err := c.MarketData.Prices(ctx, "005930")
	if err != nil || len(ps) != 1 {
		t.Fatalf("Prices = %+v, %v", ps, err)
	}
	if tokenCalls != 1 {
		t.Errorf("token issued %d times, want 1 (cached)", tokenCalls)
	}

	_, err = c.StockInfo.Warnings(ctx, "X")
	var ae *APIError
	if !errors.As(err, &ae) || ae.StatusCode != 404 {
		t.Fatalf("want *APIError 404, got %v", err)
	}
	if !IsCode(err, "stock-not-found") || IsCode(err, "other") || IsCode(nil, "stock-not-found") {
		t.Error("IsCode mismatch")
	}
}

func TestAPIErrorAlias(t *testing.T) {
	var e error = &httpclient.APIError{StatusCode: 429}
	var ae *APIError
	if !errors.As(e, &ae) {
		t.Error("APIError must alias httpclient.APIError")
	}
}
EOF
go test . 2>&1 | head -5
```
Expected: 컴파일 에러.

- [ ] **Step 2: 구현**

```bash
cat > config.go << 'EOF'
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
EOF
cat > errors.go << 'EOF'
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
EOF
cat > from_env.go << 'EOF'
package toss

import (
	"errors"
	"os"
)

// NewClientFromEnv 는 TOSS_CLIENT_ID / TOSS_CLIENT_SECRET 환경변수로 Client 를 만든다.
func NewClientFromEnv(opts ...Option) (*Client, error) {
	id, secret := os.Getenv("TOSS_CLIENT_ID"), os.Getenv("TOSS_CLIENT_SECRET")
	if id == "" || secret == "" {
		return nil, errors.New("toss: TOSS_CLIENT_ID and TOSS_CLIENT_SECRET must be set")
	}
	return NewClient(id, secret, opts...)
}
EOF
cat > client.go << 'EOF'
// Package toss 는 토스증권 Open API(https://developers.tossinvest.com)의 Go 클라이언트다.
//
// 인증은 OAuth2 Client Credentials 로, 첫 호출 때 토큰을 발급해 만료 전까지 재사용한다.
// 수치는 shopspring/decimal, 시각은 time.Time(KST 오프셋), 날짜는 tosstypes.Date 로 표현한다.
//
//	c, _ := toss.NewClientFromEnv() // TOSS_CLIENT_ID / TOSS_CLIENT_SECRET
//	ps, err := c.MarketData.Prices(ctx, "005930", "AAPL")
package toss

import (
	"context"
	"errors"
	"net/http"

	"github.com/kenshin579/toss-go/indicators"
	"github.com/kenshin579/toss-go/internal/auth"
	"github.com/kenshin579/toss-go/internal/httpclient"
	"github.com/kenshin579/toss-go/marketdata"
	"github.com/kenshin579/toss-go/marketinfo"
	"github.com/kenshin579/toss-go/ranking"
	"github.com/kenshin579/toss-go/stockinfo"
)

// Client 는 toss-go 의 단일 진입점. 그룹별 sub-client 를 필드로 노출한다.
type Client struct {
	http   *httpclient.Client
	tokens *auth.TokenSource

	MarketData       *marketdata.Client // 시세: 현재가·호가·체결·상하한가·캔들
	StockInfo        *stockinfo.Client  // 종목: 메타·전체 목록·유의사항·매매동향 5종
	MarketInfo       *marketinfo.Client // 시장 정보: 환율·장 운영 정보
	Ranking          *ranking.Client    // 주식 랭킹
	MarketIndicators *indicators.Client // 시장 지표: 지수 현재가·캔들·투자자별 매매대금
}

// NewClient 는 client credentials 로 Client 를 만든다. 생성 시 네트워크 호출은 없다(토큰은 lazy).
func NewClient(clientID, clientSecret string, opts ...Option) (*Client, error) {
	if clientID == "" || clientSecret == "" {
		return nil, errors.New("toss: clientID and clientSecret are required")
	}
	cfg := clientOptions{baseURL: httpclient.DefaultBaseURL, timeout: httpclient.DefaultTimeout}
	for _, opt := range opts {
		opt(&cfg)
	}
	hc := cfg.httpClient
	if hc == nil {
		hc = &http.Client{Timeout: cfg.timeout}
	}
	tokens := auth.New(clientID, clientSecret, cfg.baseURL, hc)
	h := httpclient.New(httpclient.Config{BaseURL: cfg.baseURL, HTTPClient: hc, Tokens: tokens})

	return &Client{
		http:             h,
		tokens:           tokens,
		MarketData:       marketdata.New(h),
		StockInfo:        stockinfo.New(h),
		MarketInfo:       marketinfo.New(h),
		Ranking:          ranking.New(h),
		MarketIndicators: indicators.New(h),
	}, nil
}

// AccessToken 은 유효한 access token 을 돌려준다(필요 시 발급/갱신). 웹소켓 연결 등 외부 용도.
func (c *Client) AccessToken(ctx context.Context) (string, error) {
	return c.tokens.Token(ctx)
}
EOF
gofmt -l .; go vet ./... && go test ./... -race 2>&1 | tail -12
```
Expected: gofmt 출력 없음, 전 패키지 `ok`.

- [ ] **Step 3: 커밋**

```bash
git add client.go config.go from_env.go errors.go client_test.go && git commit -m "feat: toss.Client 진입점 — Option, env, AccessToken, APIError/AuthError/IsCode

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 9: 예시 · integration 테스트 · release.sh · README · 워크스페이스 CLAUDE.md

**Files:**
- Create: `examples/basic/main.go`, `integration_test.go`, `scripts/release.sh`, `LICENSE`(fmp-go 복사), `README.md`(덮어쓰기, 현재 빈 파일)
- Modify: `../CLAUDE.md` (워크스페이스 루트, git 저장소 아님) — toss-go 항목 2곳

- [ ] **Step 1: 예시**

```bash
mkdir -p examples/basic && cat > examples/basic/main.go << 'EOF'
// 시세·캔들 조회 예시. 실행: TOSS_CLIENT_ID=... TOSS_CLIENT_SECRET=... go run ./examples/basic
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	toss "github.com/kenshin579/toss-go"
	"github.com/kenshin579/toss-go/marketdata"
	"github.com/kenshin579/toss-go/tosstypes"
)

func main() {
	c, err := toss.NewClientFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	prices, err := c.MarketData.Prices(ctx, "005930", "AAPL")
	if err != nil {
		log.Fatal(err)
	}
	for _, p := range prices {
		fmt.Printf("%s %s %s\n", p.Symbol, p.LastPrice, p.Currency)
	}

	page, err := c.MarketData.Candles(ctx, marketdata.CandlesParams{Symbol: "005930", Interval: tosstypes.Interval1d, Count: 5})
	if err != nil {
		log.Fatal(err)
	}
	for _, k := range page.Candles {
		fmt.Printf("%s O=%s H=%s L=%s C=%s V=%s\n", k.Timestamp.Format("2006-01-02"), k.OpenPrice, k.HighPrice, k.LowPrice, k.ClosePrice, k.Volume)
	}
	if page.NextBefore != nil {
		fmt.Println("next before:", page.NextBefore.Format(time.RFC3339))
	}

	// 에러 처리: 토스 에러 코드로 분기
	if _, err := c.StockInfo.Warnings(ctx, "000000"); err != nil {
		var ae *toss.APIError
		if errors.As(err, &ae) {
			fmt.Printf("api error: status=%d code=%s requestId=%s\n", ae.StatusCode, ae.Code, ae.RequestID)
		}
		if toss.IsCode(err, "stock-not-found") {
			fmt.Println("존재하지 않는 종목")
		}
	}
}
EOF
go vet ./examples/... && echo VET_OK
```
Expected: `VET_OK`.

- [ ] **Step 2: integration 테스트** (읽기 전용, 자격 증명 없으면 skip)

```bash
cat > integration_test.go << 'EOF'
//go:build integration

package toss_test

import (
	"context"
	"os"
	"testing"
	"time"

	toss "github.com/kenshin579/toss-go"
	"github.com/kenshin579/toss-go/marketdata"
	"github.com/kenshin579/toss-go/stockinfo"
	"github.com/kenshin579/toss-go/tosstypes"
)

// 실행: TOSS_CLIENT_ID=... TOSS_CLIENT_SECRET=... go test -tags integration -run TestIntegration ./
// 허용 IP 가 등록된 머신에서만 성공한다. 그룹별 rate limit 을 넘지 않도록 호출 사이에 짧게 쉰다.
func newIntegrationClient(t *testing.T) *toss.Client {
	t.Helper()
	if os.Getenv("TOSS_CLIENT_ID") == "" || os.Getenv("TOSS_CLIENT_SECRET") == "" {
		t.Skip("TOSS_CLIENT_ID / TOSS_CLIENT_SECRET not set")
	}
	c, err := toss.NewClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestIntegration_MarketData(t *testing.T) {
	c := newIntegrationClient(t)
	ctx := context.Background()

	ps, err := c.MarketData.Prices(ctx, "005930", "AAPL")
	if err != nil || len(ps) != 2 {
		t.Fatalf("Prices: %+v %v", ps, err)
	}
	for _, p := range ps {
		if p.LastPrice.IsZero() {
			t.Errorf("zero price: %+v", p)
		}
	}
	time.Sleep(200 * time.Millisecond)

	page, err := c.MarketData.Candles(ctx, marketdata.CandlesParams{Symbol: "005930", Interval: tosstypes.Interval1d, Count: 3})
	if err != nil || len(page.Candles) != 3 {
		t.Fatalf("Candles: %+v %v", page, err)
	}
	if page.NextBefore == nil {
		t.Error("NextBefore nil")
	}
	time.Sleep(200 * time.Millisecond)

	if _, err := c.MarketData.Orderbook(ctx, "005930"); err != nil {
		t.Errorf("Orderbook: %v", err)
	}
}

func TestIntegration_StockInfo(t *testing.T) {
	c := newIntegrationClient(t)
	ctx := context.Background()

	ss, err := c.StockInfo.Stocks(ctx, "005930")
	if err != nil || len(ss) != 1 || ss[0].Name != "삼성전자" {
		t.Fatalf("Stocks: %+v %v", ss, err)
	}
	time.Sleep(300 * time.Millisecond)

	page, err := c.StockInfo.InvestorTrading(ctx, "005930", stockinfo.TrendParams{Count: 2})
	if err != nil || len(page.Records) == 0 {
		t.Fatalf("InvestorTrading: %+v %v", page, err)
	}
	time.Sleep(300 * time.Millisecond)

	// 없는 종목의 유의사항: 404 stock-not-found 또는 빈 결과 — 둘 다 허용하되 다른 에러는 실패
	if _, err := c.StockInfo.Warnings(ctx, "000000"); err != nil && !toss.IsCode(err, "stock-not-found") {
		t.Errorf("Warnings(000000): %v", err)
	}
}

func TestIntegration_MarketInfo(t *testing.T) {
	c := newIntegrationClient(t)
	ctx := context.Background()

	fx, err := c.MarketInfo.ExchangeRate(ctx, tosstypes.CurrencyUSD, tosstypes.CurrencyKRW, nil)
	if err != nil || fx.Rate.IsZero() {
		t.Fatalf("ExchangeRate: %+v %v", fx, err)
	}
	time.Sleep(500 * time.Millisecond) // MARKET_INFO 3/s

	cal, err := c.MarketInfo.KRMarketCalendar(ctx, "")
	if err != nil || cal.Today.Date == "" {
		t.Fatalf("KRMarketCalendar: %+v %v", cal, err)
	}
}

func TestIntegration_AccessToken(t *testing.T) {
	c := newIntegrationClient(t)
	tok, err := c.AccessToken(context.Background())
	if err != nil || len(tok) < 100 {
		t.Fatalf("AccessToken: len=%d %v", len(tok), err)
	}
}
EOF
gofmt -l .; go vet -tags integration . && echo VET_OK
eval "$(grep -E '^export TOSS_CLIENT_(ID|SECRET)=' ~/.zshrc)" && go test -tags integration -run TestIntegration -v . 2>&1 | grep -E '^(--- |ok|FAIL)'
```
Expected: `VET_OK`, 그리고 4개 `--- PASS`. (IP 미등록 환경이면 토큰 403 `AuthError` 로 실패한다 — 그 경우 `t.Skip` 이 아니라 실패가 맞으며, 등록된 머신에서 다시 돌린다.)

- [ ] **Step 3: release.sh** — fmp-go 것을 복사하고 이름만 바꾼다.

```bash
cp ../fmp-go/scripts/release.sh scripts/release.sh && sed -i '' -e 's/fmp-go/toss-go/g' scripts/release.sh && chmod +x scripts/release.sh && grep -n 'toss-go' scripts/release.sh && bash -n scripts/release.sh && echo SYNTAX_OK
```
Expected: `toss-go` 가 3곳(헤더 주석, go.mod 주석, `go mod init toss-go-release-check`)에 보이고 `SYNTAX_OK`.

- [ ] **Step 4: LICENSE + README**

```bash
cp ../fmp-go/LICENSE LICENSE && head -3 LICENSE
cat > README.md << 'EOF'
# toss-go

토스증권 Open API(https://developers.tossinvest.com) 의 Go 클라이언트.

- OAuth2 Client Credentials 토큰 자동 발급·캐시(만료 60초 전 갱신, 401 토큰 오류 시 1회 재발급)
- 수치는 [`shopspring/decimal`](https://github.com/shopspring/decimal), 시각은 `time.Time`(KST 오프셋), 날짜는 `tosstypes.Date`
- 토스 에러 봉투를 `*toss.APIError`(StatusCode / Code / RequestID / Data / RetryAfter)로 매핑. `toss.IsCode(err, "stock-not-found")`
- 재시도·스로틀링 없음 — 429 는 `APIError.RetryAfter` 로 전달되며 속도 조절은 호출자 책임

## 설치

```bash
go get github.com/kenshin579/toss-go@latest
```

Go 1.25+.

## 인증

1. 토스증권 WTS 설정 > Open API 에서 client ID / secret 을 발급받고 **허용 IP** 를 등록한다
   (미등록 IP 는 토큰 발급이 `403 access_denied: IP address not allowed`).
2. 환경변수 `TOSS_CLIENT_ID`, `TOSS_CLIENT_SECRET` 를 설정한다.

```go
c, err := toss.NewClientFromEnv()
// 또는
c, err := toss.NewClient(clientID, clientSecret, toss.WithTimeout(10*time.Second))
```

## 사용

```go
ctx := context.Background()

// 현재가 (국내 6자리 코드, 해외 티커 혼용 가능)
prices, err := c.MarketData.Prices(ctx, "005930", "AAPL")

// 일봉 캔들 + 페이지네이션
page, err := c.MarketData.Candles(ctx, marketdata.CandlesParams{Symbol: "005930", Interval: tosstypes.Interval1d, Count: 200})
for page.NextBefore != nil {
    page, err = c.MarketData.Candles(ctx, marketdata.CandlesParams{Symbol: "005930", Interval: tosstypes.Interval1d, Count: 200, Before: page.NextBefore})
}

// 투자자별 매매동향 (국내 종목)
trend, err := c.StockInfo.InvestorTrading(ctx, "005930", stockinfo.TrendParams{Count: 20})

// 에러 처리
if err != nil {
    var ae *toss.APIError
    if errors.As(err, &ae) && ae.StatusCode == 429 {
        time.Sleep(ae.RetryAfter)
    }
    if toss.IsCode(err, "stock-not-found") { /* ... */ }
}
```

실행 가능한 예시: `examples/basic`.

## 커버리지

| 그룹 | 필드 | 메서드 |
| --- | --- | --- |
| Market Data | `MarketData` | `Prices` `Orderbook` `Trades` `PriceLimits` `Candles` |
| Stock Info | `StockInfo` | `Stocks` `ListStocks` `Warnings` `InvestorTrading` `ProgramTrades` `ShortSelling` `CreditTrades` `SecuritiesLending` |
| Market Info | `MarketInfo` | `ExchangeRate` `KRMarketCalendar` `USMarketCalendar` |
| Ranking | `Ranking` | `Rankings` |
| Market Indicators | `MarketIndicators` | `Prices` `Candles` `InvestorTrading` |

조회 21 ops (v0.1.0). 계좌·자산·주문·조건주문(15 ops)과 실시간 웹소켓은 후속 버전.

## 문서

토스 공식 문서 원본(`openapi.json`, `asyncapi.json`, `overview.md`, `api-reference.md`)을 [`docs/api/`](docs/api/) 에 보관한다. 갱신은 `./scripts/fetch-docs.sh`.

## 개발

```bash
go build ./... && go vet ./... && go test ./... -race
go test -tags integration ./            # 실호출 (TOSS_CLIENT_ID/SECRET + 허용 IP 필요)
./scripts/release.sh vX.Y.Z              # 태그 + GitHub Release
```

## License

MIT
EOF
file -I README.md
```
Expected: LICENSE 첫 줄 `MIT License`, README `charset=utf-8`.

- [ ] **Step 5: 워크스페이스 CLAUDE.md 갱신** — 두 곳을 Edit 도구로 바꾼다.

프로젝트 표의 행:
```
| `toss-go/` | 토스증권 Open API 문서 카탈로그 (`docs/api/` 에 공식 문서 원본 보관, 라이브러리 구현 전) | Markdown, Shell |
```
→
```
| `toss-go/` | 토스증권 Open API wrapper (Go 라이브러리, `github.com/kenshin579/toss-go`) — 조회 21 ops, 주문·WS 예정 | Go 1.25, shopspring/decimal |
```

`### toss-go` 섹션 전체:
```
### toss-go

```bash
./scripts/fetch-docs.sh    # 토스 Open API 공식 문서 원본(openapi.json, asyncapi.json 등) 갱신
```

토스증권 Open API 문서 카탈로그 단계. `docs/api/` 에 overview/api-reference/openapi/asyncapi 보관, Go 라이브러리 구현은 아직 없음. 자체 `CLAUDE.md` 없음.
```
→
```
### toss-go

```bash
go build ./... && go vet ./... && go test ./... -race
go test -tags integration ./            # 실호출 (TOSS_CLIENT_ID/SECRET + 허용 IP 등록 필요)
go run ./examples/basic
./scripts/fetch-docs.sh                  # 토스 공식 문서 원본(docs/api/) 갱신
./scripts/release.sh vX.Y.Z
```

**Module**: `github.com/kenshin579/toss-go` — 토스증권 Open API wrapper. Auth 는 OAuth2 client credentials(`TOSS_CLIENT_ID`/`TOSS_CLIENT_SECRET`, `toss.NewClientFromEnv()`), 토큰 자동 캐시. 그룹: `MarketData`/`StockInfo`/`MarketInfo`/`Ranking`/`MarketIndicators` 조회 21 ops. 수치 `shopspring/decimal`. 계좌·주문·WebSocket 은 후속. 문서 정본은 `docs/api/openapi.json`. 자체 `CLAUDE.md` 없음.
```

```bash
grep -n 'toss-go' ../CLAUDE.md | head
```
Expected: 표 행과 섹션이 새 문구로 보인다. (워크스페이스 루트는 git 저장소가 아니므로 커밋 없음.)

- [ ] **Step 6: 전체 검증 + 커밋**

```bash
gofmt -l . ; go build ./... && go vet ./... && go vet -tags integration ./... && go test ./... -race -count=1 2>&1 | tail -12 && go mod tidy && git status --short
```
Expected: gofmt 출력 없음, 전 패키지 `ok`, `go mod tidy` 후 go.mod/go.sum 변경 없음(있으면 함께 커밋).

```bash
git add examples integration_test.go scripts/release.sh LICENSE README.md go.mod go.sum && git commit -m "docs: README·예시·integration 테스트·release.sh

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 10: PR 생성

- [ ] **Step 1: 푸시 + PR (gh + HEREDOC, 리뷰어 지정 금지)**

```bash
git push -u origin feature/sdk-foundation && gh pr create --title "feat: toss-go SDK 기반 + 조회 21 ops (v0.1.0)" --body "$(cat <<'EOF'
## Summary
- `toss.Client` 진입점 + `internal/auth`(OAuth2 client credentials 토큰 발급·메모리 캐시·401 시 1회 재발급) + `internal/httpclient`(Bearer 주입, `{result}` 봉투 해제, `{error}` → `APIError`, 429 `RetryAfter`)
- 조회 21 ops: `marketdata`(5) `stockinfo`(8) `marketinfo`(3) `ranking`(1) `indicators`(3). 수치 `shopspring/decimal`, null 은 포인터, 날짜 `tosstypes.Date`
- 2026-09-04 실응답 fixture 기반 단위 테스트 + `-tags integration` 실호출 테스트, `examples/basic`, `scripts/release.sh`, README
- 설계 `docs/superpowers/specs/2026-09-04-sdk-foundation-design.md`, 계획 `docs/superpowers/plans/2026-09-04-sdk-foundation.md`

## Test plan
- [x] `go build ./... && go vet ./... && go test ./... -race` 전부 통과
- [x] `go test -tags integration ./` 허용 IP 등록 머신에서 4개 PASS(시세·종목·시장정보·토큰)
- [x] 토큰 캐시: 동시 100 goroutine 발급 1회, 만료 60초 전 갱신, 401 expired-token 1회 재시도
- [ ] 머지 후 `./scripts/release.sh v0.1.0`

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```
Expected: PR URL (`https://github.com/kenshin579/toss-go/pull/2`).

---

## 머지 후 (사용자 머지 뒤 실행)

```bash
git checkout main && git pull origin main && ./scripts/release.sh v0.1.0
```
`go list -m -versions github.com/kenshin579/toss-go` 로 proxy 인덱싱을 확인한다. 이후 메모리(`toss_go_library.md`)를 "v0.1.0 릴리스" 로 갱신하고, 다음 스펙(계좌·주문 그룹 또는 WebSocket)을 브레인스토밍한다.
