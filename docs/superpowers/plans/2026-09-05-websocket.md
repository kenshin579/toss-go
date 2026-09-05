# toss-go WebSocket 실시간 스트림 (v0.3.0) Implementation Plan

> 내부 개발 문서(설계/실행 계획). 라이브러리 사용법은 [README](../../../README.md) 를 보세요.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 실시간 체결·호가·본인 주문 이벤트를 채널로 받는 `stream` 패키지를 만들고 v0.3.0 으로 릴리스한다.

**Architecture:** 연결 하나 위에서 **선언형 full-replace** 구독을 SDK 가 대신 관리한다. `stream.Stream` 이 구독 집합을 소유하고, `Subscribe`/`Unsubscribe`/`Declare` 는 집합을 갱신한 뒤 전체 배열을 다시 선언한다. 읽기 루프가 프레임을 `type` 으로 디스패치해 시세는 **가득 차면 버리는** 채널로, 주문은 **버리지 않는** 채널로 보낸다. 끊기면 기존 연결을 먼저 닫고 지수 백오프로 재연결한 뒤 저장된 집합을 재선언하며, 그 사실을 `Reconnects()` 로 알린다.

**핵심 주의(전 태스크 공통):**
- **PING 은 JSON 이 아니라 순수 텍스트 `PING`** 이다. 서버는 **클라이언트 송신이 180초 없으면** 끊는다(수신 데이터는 타이머를 리셋하지 않음).
- **재연결 전에 기존 연결을 반드시 닫는다.** 계정당 연결 2개 한도라 안 닫으면 자기 자신을 밀어내 끊김이 반복된다.
- **`personal:order` 의 codes 는 종목이 아니라 accountSeq** 다.
- **재연결 구간의 주문 이벤트는 복구 불가** — 사용자가 REST 로 재동기화하도록 알리는 것이 SDK 의 책임이다.

**Tech Stack:** Go 1.25, `github.com/coder/websocket` v1.8.14(신규), `shopspring/decimal`.

**Spec:** `docs/superpowers/specs/2026-09-05-websocket-design.md`
**Branch:** `feature/websocket` (스펙 커밋 완료: ada8ee1)
**Base:** v0.2.0 (main 6b2e91c). 기존 관례를 따른다 — 파라미터는 값 인자, 검증은 요청 전에, 수치는 `decimal.Decimal`, nullable 은 포인터, 열거값은 문자열 타입(unknown 허용).

---

## 파일 구조

| 경로 | 책임 |
|---|---|
| `stream/events.go` | 공개 이벤트 타입 — `TradeEvent`, `OrderbookEvent`, `OrderEvent`, `Reconnect`, 에러 타입 |
| `stream/subscription.go` (+`_test.go`) | `Subscription` + 생성자(`Trade`/`Orderbook`/`Order`), 집합 관리·full-replace 직렬화·상한 검사 |
| `stream/frames.go` (+`_test.go`) | 수신 프레임 디코딩(`subscriptions`/`message`/`error`/`pong`), topic 파싱 |
| `stream/options.go` | `Option` — 버퍼 크기, PING 주기, 백오프, 자동 재연결 |
| `stream/conn.go` | 핸드셰이크·PING 루프·읽기 루프·재연결 |
| `stream/stream.go` | 공개 `Stream` — 채널 접근자, Subscribe/Unsubscribe/Declare/Close |
| `stream/testserver_test.go` | 실제 WebSocket 업그레이드 스텁 서버 |
| `client.go` | (수정) `Client.Stream(ctx, opts...)` |
| `examples/stream/main.go` | 시세 구독 예시 |
| `integration_test.go` | (수정) 시세 구독 ack 까지만 검증 |

커밋 메시지는 항상 아래 트레일러로 끝낸다:
```
Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>
```

---

### Task 1: 이벤트 타입 + 구독 집합

**Files:**
- Create: `stream/events.go`, `stream/subscription.go`, `stream/subscription_test.go`
- Modify: `go.mod` (coder/websocket 추가는 Task 3 — 이 태스크는 표준 라이브러리만)

- [ ] **Step 1: 실패 테스트 작성**

```bash
git branch --show-current && mkdir -p stream && cat > stream/subscription_test.go << 'EOF'
package stream

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/kenshin579/toss-go/tosstypes"
)

func TestSubscriptionConstructors(t *testing.T) {
	if got := Trade(tosstypes.MarketCountryKR, "005930"); got.Type != "trade:kr" || len(got.Codes) != 1 || got.Codes[0] != "005930" {
		t.Errorf("Trade(KR) = %+v", got)
	}
	if got := Trade(tosstypes.MarketCountryUS, "AAPL", "TSLA"); got.Type != "trade:us" || len(got.Codes) != 2 {
		t.Errorf("Trade(US) = %+v", got)
	}
	if got := Orderbook(tosstypes.MarketCountryKR, "005930"); got.Type != "orderbook:kr" {
		t.Errorf("Orderbook = %+v", got)
	}
	// personal:order 의 codes 는 종목이 아니라 accountSeq 문자열이다
	if got := PersonalOrder(3, 7); got.Type != "personal:order" || got.Codes[0] != "3" || got.Codes[1] != "7" {
		t.Errorf("PersonalOrder = %+v", got)
	}
}

func TestSubscriptionSet_AddRemoveDeclare(t *testing.T) {
	s := newSubscriptionSet()
	if err := s.add(Trade(tosstypes.MarketCountryKR, "005930"), Orderbook(tosstypes.MarketCountryKR, "005930")); err != nil {
		t.Fatal(err)
	}
	if err := s.add(Trade(tosstypes.MarketCountryKR, "000660")); err != nil {
		t.Fatal(err)
	}
	if n := s.count(); n != 3 {
		t.Errorf("count = %d, want 3 (채널×종목 조합 기준)", n)
	}
	// 같은 type 은 하나의 배열 원소로 합쳐진다
	body, err := s.declaration("d-1")
	if err != nil {
		t.Fatal(err)
	}
	var arr []map[string]any
	if err := json.Unmarshal(body, &arr); err != nil {
		t.Fatalf("declaration is not a JSON array: %s", body)
	}
	if len(arr) != 3 { // id 1개 + trade:kr 1개 + orderbook:kr 1개
		t.Fatalf("declaration = %s", body)
	}
	if arr[0]["id"] != "d-1" {
		t.Errorf("first element must be the request id: %s", body)
	}
	seen := map[string]int{}
	for _, e := range arr[1:] {
		typ, _ := e["type"].(string)
		codes, _ := e["codes"].([]any)
		seen[typ] = len(codes)
	}
	if seen["trade:kr"] != 2 || seen["orderbook:kr"] != 1 {
		t.Errorf("merged codes = %v (%s)", seen, body)
	}

	if err := s.remove(Trade(tosstypes.MarketCountryKR, "005930")); err != nil {
		t.Fatal(err)
	}
	if n := s.count(); n != 2 {
		t.Errorf("after remove count = %d", n)
	}
	// 마지막 code 를 지우면 type 자체가 사라진다
	if err := s.remove(Orderbook(tosstypes.MarketCountryKR, "005930")); err != nil {
		t.Fatal(err)
	}
	body, _ = s.declaration("d-2")
	_ = json.Unmarshal(body, &arr)
	for _, e := range arr[1:] {
		if e["type"] == "orderbook:kr" {
			t.Errorf("empty type must be dropped: %s", body)
		}
	}
}

func TestSubscriptionSet_EmptyDeclarationIsArray(t *testing.T) {
	s := newSubscriptionSet()
	body, err := s.declaration("")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `[]` {
		t.Errorf("empty declaration = %s, want []", body)
	}
	// 전체 해제는 id 유무와 무관하게 항상 []여야 한다 — 토스는 []만 전체 구독 해제로 해석한다.
	if body, err := s.declaration("d-1"); err != nil || string(body) != `[]` {
		t.Errorf("빈 집합은 id 가 있어도 [] 여야 한다: %s %v", body, err)
	}
}

func TestSubscriptionSet_Validation(t *testing.T) {
	s := newSubscriptionSet()
	if err := s.add(Subscription{Type: "trade:kr", Codes: []string{"삼성"}}); err == nil {
		t.Error("잘못된 심볼은 거부해야 한다")
	}
	if err := s.add(Subscription{Type: "personal:order", Codes: []string{"0"}}); err == nil {
		t.Error("accountSeq 0 은 거부해야 한다")
	}
	if err := s.add(Subscription{Type: "personal:order", Codes: []string{"abc"}}); err == nil {
		t.Error("accountSeq 는 숫자여야 한다")
	}
	if err := s.add(Subscription{Type: "bogus", Codes: []string{"x"}}); err == nil {
		t.Error("알 수 없는 type 은 거부해야 한다")
	}
	if err := s.add(Subscription{Type: "trade:kr"}); err == nil {
		t.Error("빈 codes 는 거부해야 한다")
	}
}

func TestSubscriptionSet_MaxTopics(t *testing.T) {
	s := newSubscriptionSet()
	codes := make([]string, MaxTopics)
	for i := range codes {
		// 6자리 고유 코드(플랜 원문의 "00000"+i%10 생성식은 10종만 나와 중복되므로, 정확히
		// MaxTopics 개의 서로 다른 코드를 만들도록 %06d 로 바꿨다).
		codes[i] = fmt.Sprintf("%06d", i)
	}
	if err := s.add(Subscription{Type: "trade:kr", Codes: codes}); err != nil {
		t.Fatalf("정확히 %d 건은 허용해야 한다: %v", MaxTopics, err)
	}
	if err := s.add(Trade(tosstypes.MarketCountryKR, "999999")); err == nil || !strings.Contains(err.Error(), "too many") {
		t.Errorf("%d 건 초과는 거부해야 한다: %v", MaxTopics, err)
	}
}

func TestSubscriptionSet_RejectRemoves(t *testing.T) {
	s := newSubscriptionSet()
	_ = s.add(Trade(tosstypes.MarketCountryUS, "AAPL", "NOPE"))
	// ack 의 rejected[].target 은 full key 다
	s.reject("trade:us:NOPE")
	body, _ := s.declaration("")
	if strings.Contains(string(body), "NOPE") {
		t.Errorf("거부된 항목은 집합에서 빠져야 한다: %s", body)
	}
	if !strings.Contains(string(body), "AAPL") {
		t.Errorf("나머지는 유지돼야 한다: %s", body)
	}
}

func TestSubscriptionSet_Snapshot(t *testing.T) {
	s := newSubscriptionSet()
	_ = s.add(Trade(tosstypes.MarketCountryKR, "005930"), PersonalOrder(3))
	got := s.snapshot()
	if len(got) != 2 {
		t.Fatalf("snapshot = %+v", got)
	}
	// 스냅샷을 바꿔도 내부 집합은 영향받지 않는다
	got[0].Codes[0] = "999999"
	body, _ := s.declaration("")
	if strings.Contains(string(body), "999999") {
		t.Error("snapshot 은 복사본이어야 한다")
	}
}

func TestSubscriptionSet_Replace(t *testing.T) {
	s := newSubscriptionSet()
	if err := s.add(Trade(tosstypes.MarketCountryKR, "005930")); err != nil {
		t.Fatal(err)
	}
	// replace 는 집합을 통째로 바꾼다 — 기존 구독은 새 목록에 없으면 사라진다.
	if err := s.replace(Orderbook(tosstypes.MarketCountryUS, "AAPL"), PersonalOrder(3)); err != nil {
		t.Fatal(err)
	}
	body, _ := s.declaration("")
	if strings.Contains(string(body), "005930") {
		t.Errorf("replace 후 이전 구독이 남아있으면 안 된다: %s", body)
	}
	if !strings.Contains(string(body), "AAPL") || !strings.Contains(string(body), "orderbook:us") {
		t.Errorf("replace 로 넣은 구독이 있어야 한다: %s", body)
	}
	if n := s.count(); n != 2 {
		t.Errorf("count = %d, want 2", n)
	}

	// 상한을 넘는 replace 는 기존 집합을 그대로 둔다.
	codes := make([]string, MaxTopics+1)
	for i := range codes {
		codes[i] = fmt.Sprintf("%06d", i)
	}
	if err := s.replace(Subscription{Type: "trade:kr", Codes: codes}); err == nil || !strings.Contains(err.Error(), "too many") {
		t.Errorf("상한 초과 replace 는 거부해야 한다: %v", err)
	}
	if n := s.count(); n != 2 {
		t.Errorf("실패한 replace 는 기존 집합을 보존해야 한다: count = %d, want 2", n)
	}
}
EOF
go test ./stream/ 2>&1 | head -5
```
Expected: 컴파일 에러(`undefined: Trade`, `newSubscriptionSet` 등).

- [ ] **Step 2: 이벤트 타입 작성**

```bash
cat > stream/events.go << 'EOF'
// Package stream 은 토스증권 Open API 의 실시간 웹소켓 스트림이다 — 체결·호가·본인 주문 이벤트.
//
// toss.Client.Stream 으로 만든다. 연결 하나 위에서 선언형(full-replace) 구독으로 동작하며,
// SDK 가 현재 구독 집합을 들고 있다가 Subscribe/Unsubscribe 때마다 전체를 다시 선언한다.
//
//	s, _ := c.Stream(ctx)
//	defer s.Close()
//	s.Subscribe(ctx, stream.Trade(tosstypes.MarketCountryKR, "005930"))
//	for t := range s.Trades() { ... }
//
// # 전달 보장
//
// 시세(Trades·Orderbooks)는 LOSSY 다 — 소비가 밀리면 중간 이벤트가 버려지고 최신이 우선한다.
// 주문(Orders)은 연결 세션 안에서 LOSSLESS 지만, **끊긴 구간의 이벤트는 재전송되지 않는다.**
// 따라서 Reconnects() 신호를 받으면 REST(Account(seq).Order.List)로 주문 상태를 재동기화해야 한다.
//
// # 한도
//
// 계정당 동시 연결 2개, 연결당 구독 100건(채널×종목 조합), 선언 5회/초.
// keepalive PING 은 SDK 가 알아서 보낸다(기본 60초).
package stream

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/order"
	"github.com/kenshin579/toss-go/tosstypes"
)

// TradeEvent 는 실시간 체결 1건.
type TradeEvent struct {
	Market    tosstypes.MarketCountry // 이 이벤트가 속한 시장
	Symbol    string                  // 종목 심볼
	Price     decimal.Decimal         // 체결가
	Volume    decimal.Decimal         // 체결 수량
	Timestamp time.Time               // 체결 시각
	Currency  tosstypes.Currency
}

// Level 은 호가 한 단계.
type Level struct {
	Price  decimal.Decimal `json:"price"`
	Volume decimal.Decimal `json:"volume"`
}

// OrderbookEvent 는 실시간 호가 스냅샷.
type OrderbookEvent struct {
	Market    tosstypes.MarketCountry
	Symbol    string
	Timestamp *time.Time // 데이터 미제공 시 nil
	Currency  tosstypes.Currency
	Asks      []Level // 매도호가(낮은 가격순)
	Bids      []Level // 매수호가(높은 가격순)
}

// OrderEventType 은 주문 이벤트 종류. unknown 값도 그대로 보존된다.
type OrderEventType string

const (
	OrderEventPending         OrderEventType = "PENDING"          // 접수
	OrderEventPartialFill     OrderEventType = "PARTIAL_FILL"     // 부분 체결
	OrderEventFill            OrderEventType = "FILL"             // 전량 체결
	OrderEventCanceling       OrderEventType = "CANCELING"        // 취소 중
	OrderEventCanceled        OrderEventType = "CANCELED"         // 취소됨
	OrderEventReplacing       OrderEventType = "REPLACING"        // 정정 중
	OrderEventReplaced        OrderEventType = "REPLACED"         // 정정됨
	OrderEventRejected        OrderEventType = "REJECTED"         // 주문 거부
	OrderEventCancelRejected  OrderEventType = "CANCEL_REJECTED"  // 취소 거부
	OrderEventReplaceRejected OrderEventType = "REPLACE_REJECTED" // 정정 거부
)

// OrderEvent 는 본인 계좌의 주문 이벤트.
type OrderEvent struct {
	Event      OrderEventType
	AccountSeq int64 // 구독 시 넣은 accountSeq(topic 에서 파싱)

	// Order 는 REST 주문 조회와 동일한 스냅샷이다.
	// 단 스트림에는 Execution.FilledAt 이 내려오지 않아 항상 nil 이다 — 체결 시각이 필요하면
	// Account(seq).Order.Get 으로 다시 조회한다.
	Order order.Order
}

// ReconnectCause 는 재연결이 일어난 이유.
type ReconnectCause string

const (
	// ReconnectServerShutdown 은 서버 배포로 인한 종료(server-shutdown 프레임 수신).
	ReconnectServerShutdown ReconnectCause = "server-shutdown"
	// ReconnectBackpressure 는 Orders() 를 소비하지 않아 SDK 가 연결을 끊은 경우.
	// 주문 이벤트가 유실됐을 수 있으니 REST 로 재동기화한다.
	ReconnectBackpressure ReconnectCause = "backpressure"
	// ReconnectReadError 는 읽기 실패·비정상 종료.
	ReconnectReadError ReconnectCause = "read-error"
)

// Reconnect 는 재연결이 성공했음을 알린다.
//
// 끊긴 구간의 주문 이벤트는 재전송되지 않으므로, 이 신호를 받으면 REST 로 주문 상태를 재동기화해야 한다.
type Reconnect struct {
	Attempt int            // 몇 번째 시도에서 성공했는지(1부터)
	Cause   ReconnectCause // 직전 연결이 끊긴 이유
	At      time.Time
}

// ConnectError 는 웹소켓 핸드셰이크 실패. 401 토큰 문제, 403 허용 IP 미등록, 503 서버 오류 등.
type ConnectError struct {
	StatusCode int
	Err        error
}

func (e *ConnectError) Error() string {
	return fmt.Sprintf("toss: websocket handshake failed (status %d): %v", e.StatusCode, e.Err)
}

func (e *ConnectError) Unwrap() error { return e.Err }

// DeclareError 는 구독 선언 전체가 실패했을 때의 error 프레임.
// 기존 구독은 유지된다. Code 는 wrong-format·no-type·invalid-type·no-codes·too-many-topics·
// too-many·rate-limit-exceeded·internal-error·server-shutdown 중 하나이며 unknown 값도 올 수 있다.
// 단 server-shutdown 은 SDK 가 에러로 취급하지 않고 재연결 사유(ReconnectServerShutdown)로 처리한다.
type DeclareError struct {
	ID      string // 요청에 id 를 보냈을 때만 채워진다
	Code    string
	Message string
}

func (e *DeclareError) Error() string {
	return fmt.Sprintf("toss: declare failed: %s: %s", e.Code, e.Message)
}

// RejectedError 는 구독 선언의 일부 항목이 거부됐을 때다. 나머지 항목은 정상 구독된다.
// 거부된 항목은 SDK 가 구독 집합에서 자동으로 제거한다 — 원인을 고치기 전에 재선언해도 다시 거부되기 때문이다.
// Code 는 stock-not-found·symbol-market-mismatch·account-not-found 중 하나.
type RejectedError struct {
	Target  string // full key. 예: trade:us:NOPE
	Code    string
	Message string
}

func (e *RejectedError) Error() string {
	return fmt.Sprintf("toss: subscription rejected %s: %s: %s", e.Target, e.Code, e.Message)
}
EOF
```

- [ ] **Step 3: 구독 집합 구현**

```bash
cat > stream/subscription.go << 'EOF'
package stream

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/kenshin579/toss-go/internal/params"
	"github.com/kenshin579/toss-go/tosstypes"
)

// MaxTopics 는 연결 하나가 가질 수 있는 최대 구독 수(채널×종목 조합 기준).
const MaxTopics = 100

// 구독 type 문자열.
const (
	typeTradeKR       = "trade:kr"
	typeTradeUS       = "trade:us"
	typeOrderbookKR   = "orderbook:kr"
	typeOrderbookUS   = "orderbook:us"
	typePersonalOrder = "personal:order"
)

// Subscription 은 구독 선언의 원소 하나다. 생성자(Trade·Orderbook·PersonalOrder)를 쓴다.
type Subscription struct {
	Type  string   // 예: trade:kr, orderbook:us, personal:order
	Codes []string // 시세는 종목 symbol, personal:order 는 accountSeq 문자열
}

// Trade 는 실시간 체결 구독을 만든다. 국내는 통합 시세(KRX+NXT)다.
func Trade(market tosstypes.MarketCountry, symbols ...string) Subscription {
	return Subscription{Type: marketType("trade", market), Codes: append([]string(nil), symbols...)}
}

// Orderbook 은 실시간 호가 구독을 만든다. 국내는 통합 시세(KRX+NXT)다.
func Orderbook(market tosstypes.MarketCountry, symbols ...string) Subscription {
	return Subscription{Type: marketType("orderbook", market), Codes: append([]string(nil), symbols...)}
}

// PersonalOrder 는 본인 계좌의 주문 이벤트 구독을 만든다(와이어 type: personal:order).
// codes 에 들어가는 값은 종목이 아니라 계좌 accountSeq 다(Client.Accounts 로 조회).
func PersonalOrder(accountSeqs ...int64) Subscription {
	codes := make([]string, len(accountSeqs))
	for i, seq := range accountSeqs {
		codes[i] = strconv.FormatInt(seq, 10)
	}
	return Subscription{Type: typePersonalOrder, Codes: codes}
}

func marketType(prefix string, market tosstypes.MarketCountry) string {
	return prefix + ":" + strings.ToLower(string(market))
}

// validate 는 구독 원소의 형식을 검사한다. 시세는 종목 심볼 규칙, 주문은 양의 정수 accountSeq.
func (s Subscription) validate() error {
	switch s.Type {
	case typeTradeKR, typeTradeUS, typeOrderbookKR, typeOrderbookUS:
		if len(s.Codes) == 0 {
			return fmt.Errorf("toss: %s: codes must not be empty", s.Type)
		}
		for _, c := range s.Codes {
			if err := params.Symbol(c); err != nil {
				return err
			}
		}
	case typePersonalOrder:
		if len(s.Codes) == 0 {
			return fmt.Errorf("toss: %s: codes must not be empty", s.Type)
		}
		for _, c := range s.Codes {
			seq, err := strconv.ParseInt(c, 10, 64)
			if err != nil || seq <= 0 {
				return fmt.Errorf("toss: %s: codes must be positive accountSeq (got %q)", s.Type, c)
			}
		}
	default:
		return fmt.Errorf("toss: unknown subscription type %q", s.Type)
	}
	return nil
}

// subscriptionSet 은 현재 구독 전체를 들고 있다. 프로토콜이 full-replace 라 부분 전송이 없다.
// 같은 code 를 두 번 넣어도 1건이다(프로토콜이 topic 집합이라 참조 카운트가 없다) — 두 번
// 구독한 뒤 한 번 해제하면 완전히 빠진다.
type subscriptionSet struct {
	mu sync.Mutex
	m  map[string]map[string]struct{} // type -> code set
}

func newSubscriptionSet() *subscriptionSet {
	return &subscriptionSet{m: map[string]map[string]struct{}{}}
}

// add 는 구독을 집합에 넣는다. 상한을 넘으면 아무것도 바꾸지 않고 에러를 낸다.
func (s *subscriptionSet) add(subs ...Subscription) error {
	for _, sub := range subs {
		if err := sub.validate(); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// 먼저 결과 크기를 계산해 상한을 검사한다(부분 적용 방지).
	next := s.cloneLocked()
	mergeInto(next, subs)
	if n := countTopics(next); n > MaxTopics {
		return fmt.Errorf("toss: too many topics: %d (max %d)", n, MaxTopics)
	}
	s.m = next
	return nil
}

// remove 는 구독을 집합에서 뺀다. 마지막 code 가 빠지면 type 자체를 지운다.
// 오타 난 Unsubscribe 가 조용히 무시되지 않도록 형식도 검증한다.
func (s *subscriptionSet) remove(subs ...Subscription) error {
	for _, sub := range subs {
		if err := sub.validate(); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sub := range subs {
		codes := s.m[sub.Type]
		if codes == nil {
			continue
		}
		for _, c := range sub.Codes {
			delete(codes, c)
		}
		if len(codes) == 0 {
			delete(s.m, sub.Type)
		}
	}
	return nil
}

// replace 는 집합을 통째로 바꾼다(Declare). 상한을 넘으면 기존 집합을 그대로 둔다.
func (s *subscriptionSet) replace(subs ...Subscription) error {
	for _, sub := range subs {
		if err := sub.validate(); err != nil {
			return err
		}
	}
	next := map[string]map[string]struct{}{}
	mergeInto(next, subs)
	if n := countTopics(next); n > MaxTopics {
		return fmt.Errorf("toss: too many topics: %d (max %d)", n, MaxTopics)
	}
	s.mu.Lock()
	s.m = next
	s.mu.Unlock()
	return nil
}

// reject 는 ack 의 rejected[].target(full key)을 집합에서 제거한다.
// 거부된 항목은 원인을 고치기 전에는 재선언해도 다시 거부되므로 자동으로 뺀다.
func (s *subscriptionSet) reject(target string) {
	typ, code, ok := splitTopic(target)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if codes := s.m[typ]; codes != nil {
		delete(codes, code)
		if len(codes) == 0 {
			delete(s.m, typ)
		}
	}
}

// count 는 구독 수(채널×종목 조합)를 센다.
func (s *subscriptionSet) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return countTopics(s.m)
}

// snapshot 은 현재 집합의 복사본을 돌려준다.
func (s *subscriptionSet) snapshot() []Subscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Subscription, 0, len(s.m))
	for _, typ := range sortedKeys(s.m) {
		out = append(out, Subscription{Type: typ, Codes: sortedCodes(s.m[typ])})
	}
	return out
}

// declaration 은 현재 집합 전체를 선언 배열(JSON)로 만든다.
// 집합이 비어 있으면 id 와 무관하게 `[]` 를 돌려준다 — 토스는 빈 배열만 전체 구독 해제로 해석하며,
// id 만 담긴 배열의 의미는 정의돼 있지 않다(대신 그 선언은 ack 를 짝지을 수 없다).
// id 가 비어 있지 않고 구독이 있으면 첫 원소로 넣어 ack·error 프레임에서 echo 받는다.
func (s *subscriptionSet) declaration(id string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.m) == 0 {
		return []byte("[]"), nil
	}
	arr := make([]any, 0, len(s.m)+1)
	if id != "" {
		arr = append(arr, map[string]string{"id": id})
	}
	for _, typ := range sortedKeys(s.m) {
		arr = append(arr, map[string]any{"type": typ, "codes": sortedCodes(s.m[typ])})
	}
	b, err := json.Marshal(arr)
	if err != nil {
		return nil, fmt.Errorf("toss: encode declaration: %w", err)
	}
	return b, nil
}

func (s *subscriptionSet) cloneLocked() map[string]map[string]struct{} {
	out := make(map[string]map[string]struct{}, len(s.m))
	for typ, codes := range s.m {
		c := make(map[string]struct{}, len(codes))
		for k := range codes {
			c[k] = struct{}{}
		}
		out[typ] = c
	}
	return out
}

// mergeInto 는 구독들을 type→code 집합에 병합한다.
func mergeInto(dst map[string]map[string]struct{}, subs []Subscription) {
	for _, sub := range subs {
		codes := dst[sub.Type]
		if codes == nil {
			codes = map[string]struct{}{}
			dst[sub.Type] = codes
		}
		for _, c := range sub.Codes {
			codes[c] = struct{}{}
		}
	}
}

// countTopics 는 잠금과 무관한 순수 함수로, 구독 수(채널×종목 조합)를 센다.
func countTopics(m map[string]map[string]struct{}) int {
	n := 0
	for _, codes := range m {
		n += len(codes)
	}
	return n
}

func sortedKeys(m map[string]map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedCodes(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// splitTopic 은 full key(trade:us:AAPL, personal:order:3)를 type 과 code 로 나눈다.
// type 자체가 콜론을 포함하므로 마지막 콜론을 기준으로 자른다.
func splitTopic(topic string) (typ, code string, ok bool) {
	i := strings.LastIndex(topic, ":")
	if i <= 0 || i == len(topic)-1 {
		return "", "", false
	}
	return topic[:i], topic[i+1:], true
}
EOF
gofmt -w stream && go vet ./stream/ && go test ./stream/ -race -v 2>&1 | grep -cE '^--- PASS'
```
Expected: `8`.

- [ ] **Step 4: 커밋**

```bash
git add stream && git commit -m "feat(stream): 이벤트 타입 + 선언형 구독 집합

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 2: 프레임 디코딩 + 디스패치

**Files:**
- Create: `stream/frames.go`, `stream/frames_test.go`

- [ ] **Step 1: 실패 테스트 작성**

```bash
cat > stream/frames_test.go << 'EOF'
package stream

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kenshin579/toss-go/tosstypes"
)

func TestSplitTopic(t *testing.T) {
	cases := map[string][2]string{
		"trade:us:AAPL":       {"trade:us", "AAPL"},
		"orderbook:kr:005930": {"orderbook:kr", "005930"},
		"personal:order:3":    {"personal:order", "3"},
		"trade:us:BRK.B":      {"trade:us", "BRK.B"},
	}
	for in, want := range cases {
		typ, code, ok := splitTopic(in)
		if !ok || typ != want[0] || code != want[1] {
			t.Errorf("splitTopic(%q) = %q,%q,%v", in, typ, code, ok)
		}
	}
	for _, bad := range []string{"", "trade", ":", "trade:"} {
		if _, _, ok := splitTopic(bad); ok {
			t.Errorf("splitTopic(%q) must fail", bad)
		}
	}
}

func TestDecodeFrame_Trade(t *testing.T) {
	raw := []byte(`{"type":"message","topic":"trade:us:AAPL","data":{"price":"185.25","volume":"3","timestamp":"2026-03-25T09:30:42.000+09:00","currency":"USD"}}`)
	f, err := decodeFrame(raw)
	if err != nil {
		t.Fatal(err)
	}
	if f.kind != frameMessage {
		t.Fatalf("kind = %v", f.kind)
	}
	ev, err := f.tradeEvent()
	if err != nil {
		t.Fatal(err)
	}
	if ev.Market != tosstypes.MarketCountryUS || ev.Symbol != "AAPL" {
		t.Errorf("topic parse = %+v", ev)
	}
	if ev.Price.String() != "185.25" || ev.Volume.String() != "3" || ev.Currency != tosstypes.CurrencyUSD {
		t.Errorf("data = %+v", ev)
	}
	want, _ := time.Parse(time.RFC3339, "2026-03-25T09:30:42.000+09:00")
	if !ev.Timestamp.Equal(want) {
		t.Errorf("Timestamp = %v, want %v", ev.Timestamp, want)
	}
}

func TestDecodeFrame_Orderbook(t *testing.T) {
	raw := []byte(`{"type":"message","topic":"orderbook:kr:005930","data":{"timestamp":null,"currency":"KRW","asks":[{"price":"72100","volume":"8500"},{"price":"72200","volume":"100"}],"bids":[{"price":"72000","volume":"500"}]}}`)
	f, err := decodeFrame(raw)
	if err != nil {
		t.Fatal(err)
	}
	ev, err := f.orderbookEvent()
	if err != nil {
		t.Fatal(err)
	}
	if ev.Market != tosstypes.MarketCountryKR || ev.Symbol != "005930" || ev.Currency != tosstypes.CurrencyKRW {
		t.Errorf("ev = %+v", ev)
	}
	if ev.Timestamp != nil {
		t.Errorf("null timestamp must decode to nil: %v", ev.Timestamp)
	}
	if len(ev.Asks) != 2 || ev.Asks[0].Price.String() != "72100" || ev.Asks[0].Volume.String() != "8500" {
		t.Errorf("asks = %+v", ev.Asks)
	}
	if len(ev.Bids) != 1 || ev.Bids[0].Price.String() != "72000" {
		t.Errorf("bids = %+v", ev.Bids)
	}
}

func TestDecodeFrame_Orderbook_WithTimestamp(t *testing.T) {
	raw := []byte(`{"type":"message","topic":"orderbook:kr:005930","data":{"timestamp":"2026-06-18T23:30:00.000+09:00","currency":"KRW","asks":[{"price":"71500","volume":"5"}],"bids":[{"price":"71400","volume":"10"}]}}`)
	f, err := decodeFrame(raw)
	if err != nil {
		t.Fatal(err)
	}
	ev, err := f.orderbookEvent()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := time.Parse(time.RFC3339, "2026-06-18T23:30:00.000+09:00")
	if ev.Timestamp == nil || !ev.Timestamp.Equal(want) {
		t.Errorf("non-nil timestamp = %v, want %v", ev.Timestamp, want)
	}
}

func TestDecodeFrame_Order(t *testing.T) {
	raw := []byte(`{"type":"message","topic":"personal:order:3","data":{"event":"FILL","accountSeq":"3","order":{"orderId":"o-1","symbol":"005930","side":"BUY","orderType":"LIMIT","timeInForce":"DAY","status":"FILLED","price":"70000","quantity":"10","orderAmount":null,"currency":"KRW","orderedAt":"2026-03-28T09:30:00+09:00","canceledAt":null,"execution":{"filledQuantity":"10","averageFilledPrice":"70000","filledAmount":"700000","commission":"1400","tax":"0","settlementDate":"2026-03-30"}}}}`)
	f, err := decodeFrame(raw)
	if err != nil {
		t.Fatal(err)
	}
	ev, err := f.orderEvent()
	if err != nil {
		t.Fatal(err)
	}
	if ev.Event != OrderEventFill || ev.AccountSeq != 3 {
		t.Errorf("ev = %+v", ev)
	}
	if ev.Order.OrderID != "o-1" || ev.Order.Status != "FILLED" || ev.Order.Quantity.String() != "10" {
		t.Errorf("order = %+v", ev.Order)
	}
	if ev.Order.Execution.AverageFilledPrice == nil || ev.Order.Execution.AverageFilledPrice.String() != "70000" {
		t.Errorf("execution = %+v", ev.Order.Execution)
	}
	// 스트림에는 filledAt 이 없다
	if ev.Order.Execution.FilledAt != nil {
		t.Errorf("스트림 주문 스냅샷에는 filledAt 이 없어야 한다: %v", ev.Order.Execution.FilledAt)
	}
	if ev.Order.Execution.SettlementDate == nil || *ev.Order.Execution.SettlementDate != "2026-03-30" {
		t.Errorf("settlementDate = %v", ev.Order.Execution.SettlementDate)
	}
}

func TestDecodeFrame_SubscriptionsAck(t *testing.T) {
	raw := []byte(`{"type":"subscriptions","id":"d-1","subscribed":["trade:us:AAPL"],"rejected":[{"target":"trade:us:NOPE","code":"stock-not-found","message":"없음"}]}`)
	f, err := decodeFrame(raw)
	if err != nil {
		t.Fatal(err)
	}
	if f.kind != frameSubscriptions || f.id != "d-1" {
		t.Fatalf("f = %+v", f)
	}
	if len(f.subscribed) != 1 || f.subscribed[0] != "trade:us:AAPL" {
		t.Errorf("subscribed = %v", f.subscribed)
	}
	if len(f.rejected) != 1 || f.rejected[0].Target != "trade:us:NOPE" || f.rejected[0].Code != "stock-not-found" {
		t.Errorf("rejected = %+v", f.rejected)
	}
}

func TestDecodeFrame_ErrorAndPong(t *testing.T) {
	f, err := decodeFrame([]byte(`{"type":"error","id":"d-2","error":{"code":"rate-limit-exceeded","message":"too fast"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if f.kind != frameError || f.errCode != "rate-limit-exceeded" || f.id != "d-2" {
		t.Errorf("f = %+v", f)
	}
	f, err = decodeFrame([]byte(`{"type":"pong"}`))
	if err != nil {
		t.Fatal(err)
	}
	if f.kind != framePong {
		t.Errorf("f = %+v", f)
	}
}

func TestDecodeFrame_UnknownTypeIsIgnored(t *testing.T) {
	f, err := decodeFrame([]byte(`{"type":"brand-new-frame","x":1}`))
	if err != nil {
		t.Fatalf("알 수 없는 프레임은 에러가 아니라 무시 대상이어야 한다: %v", err)
	}
	if f.kind != frameUnknown {
		t.Errorf("kind = %v", f.kind)
	}
}

func TestDecodeFrame_Malformed(t *testing.T) {
	if _, err := decodeFrame([]byte(`not json`)); err == nil {
		t.Error("깨진 JSON 은 에러여야 한다")
	}
}

func TestFrame_TopicMismatch(t *testing.T) {
	f, _ := decodeFrame([]byte(`{"type":"message","topic":"orderbook:kr:005930","data":{}}`))
	if _, err := f.tradeEvent(); err == nil {
		t.Error("호가 topic 을 체결로 해석하면 에러여야 한다")
	}
}

func TestDecodeFrame_MessageStructure(t *testing.T) {
	for name, raw := range map[string]string{
		"no topic":    `{"type":"message","data":{"price":"1"}}`,
		"empty topic": `{"type":"message","topic":"","data":{"price":"1"}}`,
		"null data":   `{"type":"message","topic":"trade:kr:005930","data":null}`,
		"no data":     `{"type":"message","topic":"trade:kr:005930"}`,
	} {
		if _, err := decodeFrame([]byte(raw)); err == nil {
			t.Errorf("%s: 구조가 깨진 message 프레임은 에러여야 한다(가격 0 이벤트 방지)", name)
		}
	}
}

func TestFrame_ConverterErrors(t *testing.T) {
	// 값이 깨진 payload 는 zero value 가 아니라 에러여야 한다
	bad := map[string]string{
		"non-numeric price": `{"type":"message","topic":"trade:kr:005930","data":{"price":"abc","volume":"1","timestamp":"2026-03-25T09:30:42+09:00","currency":"KRW"}}`,
		"empty price":       `{"type":"message","topic":"trade:kr:005930","data":{"price":"","volume":"1","timestamp":"2026-03-25T09:30:42+09:00","currency":"KRW"}}`,
		"bad timestamp":     `{"type":"message","topic":"trade:kr:005930","data":{"price":"1","volume":"1","timestamp":"nope","currency":"KRW"}}`,
	}
	for name, raw := range bad {
		f, err := decodeFrame([]byte(raw))
		if err != nil {
			t.Fatalf("%s: decode = %v", name, err)
		}
		if _, err := f.tradeEvent(); err == nil {
			t.Errorf("%s: 값 오류는 에러여야 한다", name)
		}
	}
	// accountSeq 가 숫자가 아닌 topic
	f, _ := decodeFrame([]byte(`{"type":"message","topic":"personal:order:abc","data":{"event":"FILL","accountSeq":"abc","order":{}}}`))
	if _, err := f.orderEvent(); err == nil {
		t.Error("숫자가 아닌 accountSeq 는 에러여야 한다")
	}
	// 변환기 × 잘못된 topic 조합
	mismatch := []struct {
		topic string
		conv  func(frame) error
	}{
		{"orderbook:kr:005930", func(f frame) error { _, err := f.tradeEvent(); return err }},
		{"personal:order:3", func(f frame) error { _, err := f.tradeEvent(); return err }},
		{"trade:kr:005930", func(f frame) error { _, err := f.orderbookEvent(); return err }},
		{"personal:order:3", func(f frame) error { _, err := f.orderbookEvent(); return err }},
		{"trade:kr:005930", func(f frame) error { _, err := f.orderEvent(); return err }},
		{"orderbook:kr:005930", func(f frame) error { _, err := f.orderEvent(); return err }},
	}
	for _, m := range mismatch {
		f, err := decodeFrame([]byte(`{"type":"message","topic":"` + m.topic + `","data":{}}`))
		if err != nil {
			t.Fatalf("decode(%s) = %v", m.topic, err)
		}
		if err := m.conv(f); err == nil {
			t.Errorf("%s: 다른 채널 변환기는 에러여야 한다", m.topic)
		}
	}
}

// TestDecodeFrame_AsyncAPIExamples 는 docs/api/asyncapi.json 의 실제 예시 payload(stream/testdata/frames/*.json)로
// decodeFrame + 해당 변환기가 성공하는지 확인한다. 손으로 적은 테스트 리터럴이 스펙과 어긋나는 것을 막는 회귀 테스트다.
func TestDecodeFrame_AsyncAPIExamples(t *testing.T) {
	cases := []struct {
		file string
		conv func(frame) error
	}{
		{"trade.json", func(f frame) error { _, err := f.tradeEvent(); return err }},
		{"orderbook.json", func(f frame) error { _, err := f.orderbookEvent(); return err }},
		{"order.json", func(f frame) error { _, err := f.orderEvent(); return err }},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", "frames", c.file))
			if err != nil {
				t.Fatal(err)
			}
			f, err := decodeFrame(raw)
			if err != nil {
				t.Fatalf("decodeFrame: %v", err)
			}
			if f.kind != frameMessage {
				t.Fatalf("kind = %v", f.kind)
			}
			if err := c.conv(f); err != nil {
				t.Errorf("converter: %v", err)
			}
		})
	}
}

// FuzzDecodeFrame 은 임의의 입력에도 decodeFrame·변환기가 panic 하지 않음을 확인한다.
func FuzzDecodeFrame(f *testing.F) {
	f.Add(`{"type":"message","topic":"trade:kr:005930","data":{"price":"1","volume":"1","timestamp":"2026-03-25T09:30:42+09:00","currency":"KRW"}}`)
	f.Add(`{"type":"subscriptions","subscribed":["trade:kr:005930"],"rejected":[]}`)
	f.Add(`{"type":"error","error":{"code":"x","message":"y"}}`)
	f.Add(`{"type":"pong"}`)
	f.Fuzz(func(t *testing.T, raw string) {
		fr, err := decodeFrame([]byte(raw))
		if err != nil {
			return
		}
		if fr.kind != frameMessage {
			return
		}
		// 어떤 입력에도 panic 하지 않아야 한다
		_, _ = fr.tradeEvent()
		_, _ = fr.orderbookEvent()
		_, _ = fr.orderEvent()
	})
}

func TestFrame_EmptyPayloadIsRejected(t *testing.T) {
	// data:{} 는 구조상 유효하지만 값이 비어 있다 — zero value(가격 0 체결, 빈 주문)를 내보내면 안 된다.
	for name, raw := range map[string]string{
		"empty trade":          `{"type":"message","topic":"trade:kr:005930","data":{}}`,
		"trade missing volume": `{"type":"message","topic":"trade:kr:005930","data":{"price":"72000","timestamp":"2026-03-25T09:30:42+09:00","currency":"KRW"}}`,
		"zero price":           `{"type":"message","topic":"trade:kr:005930","data":{"price":"0","volume":"1","timestamp":"2026-03-25T09:30:42+09:00","currency":"KRW"}}`,
	} {
		f, err := decodeFrame([]byte(raw))
		if err != nil {
			t.Fatalf("%s: decode = %v", name, err)
		}
		if ev, err := f.tradeEvent(); err == nil {
			t.Errorf("%s: 값이 빈 체결은 에러여야 한다, got %+v", name, ev)
		}
	}
	for name, raw := range map[string]string{
		"empty order": `{"type":"message","topic":"personal:order:3","data":{}}`,
		"no orderId":  `{"type":"message","topic":"personal:order:3","data":{"event":"FILL","accountSeq":"3","order":{}}}`,
		"no event":    `{"type":"message","topic":"personal:order:3","data":{"accountSeq":"3","order":{"orderId":"o-1"}}}`,
	} {
		f, err := decodeFrame([]byte(raw))
		if err != nil {
			t.Fatalf("%s: decode = %v", name, err)
		}
		if ev, err := f.orderEvent(); err == nil {
			t.Errorf("%s: 값이 빈 주문 이벤트는 에러여야 한다, got %+v", name, ev)
		}
	}
	// 호가는 장 시작 전처럼 비어 있는 상태가 실제로 존재하므로 관용적으로 통과시킨다.
	f, err := decodeFrame([]byte(`{"type":"message","topic":"orderbook:kr:005930","data":{"currency":"KRW","asks":[],"bids":[]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.orderbookEvent(); err != nil {
		t.Errorf("빈 호가는 정상 상태다: %v", err)
	}
}

func TestDecodeFrame_EmptyErrorObject(t *testing.T) {
	f, err := decodeFrame([]byte(`{"type":"error","error":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	if f.kind != frameError || f.errCode != "unknown" {
		t.Errorf("빈 error 객체도 진단 가능한 코드를 가져야 한다: %+v", f)
	}
}
EOF
go test ./stream/ 2>&1 | head -5
```
Expected: 컴파일 에러(`undefined: decodeFrame` 등).

- [ ] **Step 2: 구현**

```bash
cat > stream/frames.go << 'EOF'
package stream

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/kenshin579/toss-go/order"
	"github.com/kenshin579/toss-go/tosstypes"
)

// frameKind 는 수신 프레임 종류. 서버는 top-level "type" 으로 구분한다.
type frameKind int

const (
	frameUnknown frameKind = iota // 알 수 없는 type — 무시한다(서버가 프레임을 추가할 수 있다)
	frameSubscriptions
	frameMessage
	frameError
	framePong
)

// rejectedItem 은 ack 의 rejected[] 원소.
type rejectedItem struct {
	Target  string `json:"target"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// frame 은 디코딩된 수신 프레임.
type frame struct {
	kind  frameKind
	id    string // subscriptions/error 프레임에서 요청 id echo
	topic string // message 프레임
	data  json.RawMessage

	subscribed []string
	rejected   []rejectedItem

	errCode    string
	errMessage string
}

type wireFrame struct {
	Type       string          `json:"type"`
	ID         string          `json:"id"`
	Topic      string          `json:"topic"`
	Data       json.RawMessage `json:"data"`
	Subscribed []string        `json:"subscribed"`
	Rejected   []rejectedItem  `json:"rejected"`
	Error      *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// decodeFrame 은 텍스트 프레임을 해석한다. 알 수 없는 type 은 frameUnknown 이며 에러가 아니다.
func decodeFrame(raw []byte) (frame, error) {
	var w wireFrame
	if err := json.Unmarshal(raw, &w); err != nil {
		return frame{}, fmt.Errorf("toss: decode frame: %w", err)
	}
	f := frame{id: w.ID, topic: w.Topic, data: w.Data, subscribed: w.Subscribed, rejected: w.Rejected}
	switch w.Type {
	case "subscriptions":
		f.kind = frameSubscriptions
	case "message":
		// 구조 검증은 여기서 한다 — 모든 수신 프레임이 지나는 단일 지점이고,
		// data 가 null/부재면 payload 언마샬이 에러 없이 zero value 를 만들어(가격 0 인 체결 등)
		// 사용자 채널로 흘러들어간다.
		if w.Topic == "" {
			return frame{}, fmt.Errorf("toss: message frame has no topic")
		}
		if len(w.Data) == 0 || string(w.Data) == "null" {
			return frame{}, fmt.Errorf("toss: message frame %q has no data", w.Topic)
		}
		f.kind = frameMessage
	case "error":
		f.kind = frameError
		if w.Error != nil && w.Error.Code != "" {
			f.errCode, f.errMessage = w.Error.Code, w.Error.Message
		} else {
			if w.Error != nil {
				f.errMessage = w.Error.Message
			}
			f.errCode = "unknown" // 서버가 새 에러 형태를 보내도 스트림을 죽이지 않되, 메시지는 진단 가능하게
		}
	case "pong":
		f.kind = framePong
	default:
		f.kind = frameUnknown
	}
	return f, nil
}

// topicKind 는 message 프레임의 topic 이 어느 채널인지 돌려준다.
func (f frame) topicKind() string {
	typ, _, ok := splitTopic(f.topic)
	if !ok {
		return ""
	}
	return typ
}

// prefix 는 subscription.go 의 typeTrade*/typeOrderbook* 와 짝을 이룬다. 시장 세그먼트는
// 검증하지 않는다 — 토스가 시장을 추가해도 디코딩이 깨지지 않도록 관용적으로 둔다.
func (f frame) marketSymbol(prefix string) (tosstypes.MarketCountry, string, error) {
	typ, code, ok := splitTopic(f.topic)
	if !ok || !strings.HasPrefix(typ, prefix+":") {
		return "", "", fmt.Errorf("toss: unexpected topic %q for %s event", f.topic, prefix)
	}
	market := tosstypes.MarketCountry(strings.ToUpper(strings.TrimPrefix(typ, prefix+":")))
	return market, code, nil
}

type wireTrade struct {
	Price     decimal.Decimal    `json:"price"`
	Volume    decimal.Decimal    `json:"volume"`
	Timestamp time.Time          `json:"timestamp"`
	Currency  tosstypes.Currency `json:"currency"`
}

// tradeEvent 는 message 프레임을 체결 이벤트로 해석한다.
func (f frame) tradeEvent() (TradeEvent, error) {
	market, symbol, err := f.marketSymbol("trade")
	if err != nil {
		return TradeEvent{}, err
	}
	var w wireTrade
	if err := json.Unmarshal(f.data, &w); err != nil {
		return TradeEvent{}, fmt.Errorf("toss: decode trade data: %w", err)
	}
	// 값 검증 — 빈 객체나 일부 필드만 담긴 payload 는 언마샬이 통과해 zero value(가격 0 체결)를 만든다.
	// 체결에 시각·가격·수량이 없는 경우는 없으므로 이벤트로 내보내지 않고 에러로 돌린다.
	if w.Timestamp.IsZero() || !w.Price.IsPositive() || !w.Volume.IsPositive() {
		return TradeEvent{}, fmt.Errorf("toss: incomplete trade data for %q (price=%s volume=%s)", f.topic, w.Price, w.Volume)
	}
	return TradeEvent{
		Market: market, Symbol: symbol,
		Price: w.Price, Volume: w.Volume, Timestamp: w.Timestamp, Currency: w.Currency,
	}, nil
}

type wireOrderbook struct {
	Timestamp *time.Time         `json:"timestamp"`
	Currency  tosstypes.Currency `json:"currency"`
	Asks      []Level            `json:"asks"`
	Bids      []Level            `json:"bids"`
}

// orderbookEvent 는 message 프레임을 호가 이벤트로 해석한다.
// 체결·주문과 달리 값 검증을 하지 않는다 — 장 시작 전처럼 호가가 비어 있는 상태가 실제로 존재한다.
func (f frame) orderbookEvent() (OrderbookEvent, error) {
	market, symbol, err := f.marketSymbol("orderbook")
	if err != nil {
		return OrderbookEvent{}, err
	}
	var w wireOrderbook
	if err := json.Unmarshal(f.data, &w); err != nil {
		return OrderbookEvent{}, fmt.Errorf("toss: decode orderbook data: %w", err)
	}
	return OrderbookEvent{
		Market: market, Symbol: symbol,
		Timestamp: w.Timestamp, Currency: w.Currency, Asks: w.Asks, Bids: w.Bids,
	}, nil
}

type wireOrder struct {
	Event OrderEventType `json:"event"`
	// AccountSeq 는 와이어 문서화용. 실제 값은 topic 에서 파싱한다(스펙상 동일).
	// 불일치해도 에러로 만들지 않는다 — 무손실 채널의 이벤트를 버리는 대가가 더 크다.
	AccountSeq string      `json:"accountSeq"`
	Order      order.Order `json:"order"`
}

// orderEvent 는 message 프레임을 주문 이벤트로 해석한다.
// 스트림의 주문 스냅샷에는 execution.filledAt 이 없어 Order.Execution.FilledAt 은 항상 nil 이다.
func (f frame) orderEvent() (OrderEvent, error) {
	typ, code, ok := splitTopic(f.topic)
	if !ok || typ != typePersonalOrder {
		return OrderEvent{}, fmt.Errorf("toss: unexpected topic %q for order event", f.topic)
	}
	var w wireOrder
	if err := json.Unmarshal(f.data, &w); err != nil {
		return OrderEvent{}, fmt.Errorf("toss: decode order data: %w", err)
	}
	seq, err := strconv.ParseInt(code, 10, 64)
	if err != nil {
		return OrderEvent{}, fmt.Errorf("toss: invalid accountSeq in topic %q: %w", f.topic, err)
	}
	// 값 검증 — 빈 payload 는 언마샬이 통과한다. 무손실 채널이라 빈 이벤트를 흘리면 소비자가 잘못 판단한다.
	if w.Event == "" || w.Order.OrderID == "" {
		return OrderEvent{}, fmt.Errorf("toss: incomplete order data for %q (event=%q orderId=%q)", f.topic, w.Event, w.Order.OrderID)
	}
	return OrderEvent{Event: w.Event, AccountSeq: seq, Order: w.Order}, nil
}
EOF
gofmt -w stream && go vet ./stream/ && go test ./stream/ -race -v 2>&1 | grep -cE '^--- PASS'
```
Expected: `22` (Task 1 의 8 + Task 2 의 14).

- [ ] **Step 3: 커밋**

```bash
git add stream && git commit -m "feat(stream): 수신 프레임 디코딩·topic 파싱

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 3: 연결·PING·재연결 + 공개 Stream

**Files:**
- Create: `stream/options.go`, `stream/conn.go`, `stream/stream.go`, `stream/testserver_test.go`, `stream/stream_test.go`
- Modify: `go.mod`, `go.sum`, `client.go`

- [ ] **Step 1: 의존성 추가**

```bash
go get github.com/coder/websocket@v1.8.14 && grep coder go.mod
```
Expected: `github.com/coder/websocket v1.8.14`.

- [ ] **Step 2: 옵션 작성**

```bash
cat > stream/options.go << 'EOF'
package stream

import "time"

// 기본값.
const (
	DefaultPingInterval  = 60 * time.Second // 서버는 클라이언트 송신이 180초 없으면 끊는다
	DefaultTradeBuffer   = 1024
	DefaultOrderBuffer   = 256
	defaultDiagBuffer    = 16
	defaultBackoffMin    = time.Second
	defaultBackoffMax    = 30 * time.Second
	defaultCoalesceDelay = 100 * time.Millisecond // 선언 5회/초 한도 대응
)

type config struct {
	pingInterval  time.Duration
	tradeBuffer   int
	orderBuffer   int
	backoffMin    time.Duration
	backoffMax    time.Duration
	coalesceDelay time.Duration
	autoReconnect bool
	baseURL       string // 테스트용 ws:// 오버라이드
}

func defaultConfig() config {
	return config{
		pingInterval:  DefaultPingInterval,
		tradeBuffer:   DefaultTradeBuffer,
		orderBuffer:   DefaultOrderBuffer,
		backoffMin:    defaultBackoffMin,
		backoffMax:    defaultBackoffMax,
		coalesceDelay: defaultCoalesceDelay,
		autoReconnect: true,
	}
}

// Option 은 Stream 생성 옵션.
type Option func(*config)

// WithPingInterval 은 keepalive PING 주기를 바꾼다(기본 60초).
// 서버는 클라이언트로부터의 수신이 180초 없으면 연결을 끊으므로 그보다 짧아야 한다.
func WithPingInterval(d time.Duration) Option { return func(c *config) { c.pingInterval = d } }

// WithTradeBuffer 는 시세 채널(Trades·Orderbooks) 버퍼 크기를 바꾼다(기본 1024).
// 가득 차면 가장 오래된 이벤트를 버린다 — 시세는 LOSSY 규약이다.
func WithTradeBuffer(n int) Option { return func(c *config) { c.tradeBuffer = n } }

// WithOrderBuffer 는 주문 채널 버퍼 크기를 바꾼다(기본 256).
// 주문 이벤트는 버리지 않는다 — 가득 차면 연결을 끊고 재연결하며 Reconnects() 로 알린다.
func WithOrderBuffer(n int) Option { return func(c *config) { c.orderBuffer = n } }

// WithBackoff 는 재연결 지수 백오프의 시작·상한을 바꾼다(기본 1초·30초).
func WithBackoff(min, max time.Duration) Option {
	return func(c *config) { c.backoffMin, c.backoffMax = min, max }
}

// WithoutAutoReconnect 는 자동 재연결을 끈다. 연결이 끊기면 모든 채널이 닫힌다.
func WithoutAutoReconnect() Option { return func(c *config) { c.autoReconnect = false } }

// WithBaseURL 은 웹소켓 URL 을 바꾼다(테스트용). 기본 wss://openapi-ws.tossinvest.com/ws/v1.
func WithBaseURL(u string) Option { return func(c *config) { c.baseURL = u } }

// WithCoalesceDelay 는 연속된 구독 변경을 하나의 선언으로 묶는 대기 시간을 바꾼다(기본 100ms).
// 토스는 선언을 초당 5회로 제한한다.
func WithCoalesceDelay(d time.Duration) Option { return func(c *config) { c.coalesceDelay = d } }
EOF
```

- [ ] **Step 3: 스텁 서버 작성**

```bash
cat > stream/testserver_test.go << 'EOF'
package stream

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// testServer 는 실제 웹소켓 업그레이드를 하는 스텁이다.
// 수신한 텍스트 프레임을 기록하고, 테스트가 원하는 프레임을 밀어 넣을 수 있다.
type testServer struct {
	srv *httptest.Server
	url string

	mu       sync.Mutex
	received []string      // 클라이언트가 보낸 텍스트 프레임(선언·PING)
	conns    int           // 총 연결 수(재연결 검증용)
	authHdr  []string      // 연결별 Authorization 헤더
	pushCh   chan string   // 서버 → 클라이언트로 밀어 넣을 프레임
	closeCh  chan struct{} // 현재 연결을 강제 종료하라는 신호
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	ts := &testServer{pushCh: make(chan string, 64), closeCh: make(chan struct{}, 8)}
	ts.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		ts.mu.Lock()
		ts.conns++
		ts.authHdr = append(ts.authHdr, r.Header.Get("Authorization"))
		ts.mu.Unlock()

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		go func() { // 읽기 루프: 수신 프레임 기록 + PING 에 pong 응답
			for {
				_, data, err := c.Read(ctx)
				if err != nil {
					cancel()
					return
				}
				s := string(data)
				ts.mu.Lock()
				ts.received = append(ts.received, s)
				ts.mu.Unlock()
				if s == "PING" {
					_ = c.Write(ctx, websocket.MessageText, []byte(`{"type":"pong"}`))
				}
			}
		}()
		for {
			select {
			case <-ctx.Done():
				_ = c.CloseNow()
				return
			case <-ts.closeCh:
				_ = c.CloseNow()
				return
			case msg := <-ts.pushCh:
				if err := c.Write(ctx, websocket.MessageText, []byte(msg)); err != nil {
					return
				}
			}
		}
	}))
	ts.url = "ws" + strings.TrimPrefix(ts.srv.URL, "http")
	t.Cleanup(ts.srv.Close)
	return ts
}

func (ts *testServer) push(msg string) { ts.pushCh <- msg }

func (ts *testServer) dropConn() { ts.closeCh <- struct{}{} }

func (ts *testServer) connCount() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.conns
}

func (ts *testServer) frames() []string {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return append([]string(nil), ts.received...)
}

func (ts *testServer) authHeaders() []string {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return append([]string(nil), ts.authHdr...)
}

// waitFor 는 조건이 참이 될 때까지 최대 2초 기다린다.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

// staticToken 은 고정 토큰을 돌려주는 TokenFunc.
func staticToken(tok string) func(context.Context) (string, error) {
	return func(context.Context) (string, error) { return tok, nil }
}
EOF
```

- [ ] **Step 4: 공개 Stream 테스트 작성**

```bash
cat > stream/stream_test.go << 'EOF'
package stream

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kenshin579/toss-go/tosstypes"
)

func newTestStream(t *testing.T, ts *testServer, opts ...Option) *Stream {
	t.Helper()
	opts = append([]Option{WithBaseURL(ts.url), WithPingInterval(30 * time.Millisecond),
		WithBackoff(5*time.Millisecond, 20*time.Millisecond), WithCoalesceDelay(time.Millisecond)}, opts...)
	s, err := New(context.Background(), staticToken("tok"), opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestStream_HandshakeSendsBearer(t *testing.T) {
	ts := newTestServer(t)
	newTestStream(t, ts)
	waitFor(t, "connection", func() bool { return ts.connCount() >= 1 })
	if h := ts.authHeaders(); len(h) == 0 || h[0] != "Bearer tok" {
		t.Errorf("Authorization = %v", h)
	}
}

func TestStream_SubscribeDeclaresFullSet(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts)
	if err := s.Subscribe(context.Background(), Trade(tosstypes.MarketCountryKR, "005930")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "first declare", func() bool { return len(declares(ts)) >= 1 })
	if err := s.Subscribe(context.Background(), Orderbook(tosstypes.MarketCountryKR, "005930")); err != nil {
		t.Fatal(err)
	}
	// 두 번째 선언에는 첫 구독도 함께 들어 있어야 한다(full-replace)
	waitFor(t, "second declare", func() bool {
		d := declares(ts)
		return len(d) >= 2 && strings.Contains(d[len(d)-1], "trade:kr") && strings.Contains(d[len(d)-1], "orderbook:kr")
	})
}

func TestStream_UnsubscribeAndDeclare(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts)
	ctx := context.Background()
	_ = s.Subscribe(ctx, Trade(tosstypes.MarketCountryKR, "005930", "000660"))
	waitFor(t, "declare", func() bool { return len(declares(ts)) >= 1 })
	_ = s.Unsubscribe(ctx, Trade(tosstypes.MarketCountryKR, "000660"))
	waitFor(t, "unsubscribe declare", func() bool {
		d := declares(ts)
		return len(d) >= 2 && !strings.Contains(d[len(d)-1], "000660")
	})
	// Declare 는 집합을 통째로 바꾼다
	_ = s.Declare(ctx, Orderbook(tosstypes.MarketCountryUS, "AAPL"))
	waitFor(t, "replace declare", func() bool {
		d := declares(ts)
		last := d[len(d)-1]
		return len(d) >= 3 && strings.Contains(last, "AAPL") && !strings.Contains(last, "005930")
	})
	if got := s.Subscriptions(); len(got) != 1 || got[0].Type != "orderbook:us" {
		t.Errorf("Subscriptions = %+v", got)
	}
}

func TestStream_EmptyDeclareUnsubscribesAll(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts)
	ctx := context.Background()
	_ = s.Subscribe(ctx, Trade(tosstypes.MarketCountryKR, "005930"))
	waitFor(t, "declare", func() bool { return len(declares(ts)) >= 1 })
	_ = s.Declare(ctx)
	waitFor(t, "empty declare", func() bool {
		d := declares(ts)
		return strings.TrimSpace(d[len(d)-1]) == "[]"
	})
}

func TestStream_PingIsRawText(t *testing.T) {
	ts := newTestServer(t)
	newTestStream(t, ts)
	waitFor(t, "ping", func() bool {
		for _, f := range ts.frames() {
			if f == "PING" { // JSON 이 아니라 순수 텍스트여야 한다
				return true
			}
			if strings.Contains(f, `"PING"`) {
				t.Errorf("PING 은 JSON 이 아니라 순수 텍스트여야 한다: %s", f)
			}
		}
		return false
	})
}

func TestStream_DispatchesEvents(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts)
	waitFor(t, "connection", func() bool { return ts.connCount() >= 1 })

	ts.push(`{"type":"message","topic":"trade:kr:005930","data":{"price":"72000","volume":"5","timestamp":"2026-03-25T09:30:42.000+09:00","currency":"KRW"}}`)
	select {
	case ev := <-s.Trades():
		if ev.Symbol != "005930" || ev.Price.String() != "72000" {
			t.Errorf("trade = %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("trade event 미수신")
	}

	ts.push(`{"type":"message","topic":"orderbook:kr:005930","data":{"timestamp":null,"currency":"KRW","asks":[{"price":"72100","volume":"10"}],"bids":[]}}`)
	select {
	case ev := <-s.Orderbooks():
		if len(ev.Asks) != 1 || ev.Timestamp != nil {
			t.Errorf("orderbook = %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("orderbook event 미수신")
	}
}

func TestStream_AckRejectRemovesFromSet(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts)
	ctx := context.Background()
	_ = s.Subscribe(ctx, Trade(tosstypes.MarketCountryUS, "AAPL", "NOPE"))
	waitFor(t, "declare", func() bool { return len(declares(ts)) >= 1 })
	ts.push(`{"type":"subscriptions","subscribed":["trade:us:AAPL"],"rejected":[{"target":"trade:us:NOPE","code":"stock-not-found","message":"없음"}]}`)

	select {
	case err := <-s.Errors():
		var re *RejectedError
		if !asRejected(err, &re) || re.Target != "trade:us:NOPE" {
			t.Errorf("err = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rejected 알림 미수신")
	}
	// 거부된 항목은 집합에서 빠져 다음 선언에 포함되지 않는다
	waitFor(t, "set without NOPE", func() bool {
		for _, sub := range s.Subscriptions() {
			for _, c := range sub.Codes {
				if c == "NOPE" {
					return false
				}
			}
		}
		return true
	})
}

func TestStream_ErrorFrame(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts)
	waitFor(t, "connection", func() bool { return ts.connCount() >= 1 })
	ts.push(`{"type":"error","error":{"code":"rate-limit-exceeded","message":"too fast"}}`)
	select {
	case err := <-s.Errors():
		var de *DeclareError
		if !asDeclare(err, &de) || de.Code != "rate-limit-exceeded" {
			t.Errorf("err = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("error frame 미수신")
	}
}

func TestStream_ReconnectsAndRedeclares(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts)
	ctx := context.Background()
	_ = s.Subscribe(ctx, Trade(tosstypes.MarketCountryKR, "005930"))
	waitFor(t, "first declare", func() bool { return len(declares(ts)) >= 1 })

	ts.dropConn()
	select {
	case r := <-s.Reconnects():
		if r.Attempt < 1 {
			t.Errorf("reconnect = %+v", r)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("reconnect 알림 미수신")
	}
	// 재연결 후 저장된 구독이 다시 선언돼야 한다
	waitFor(t, "re-declare", func() bool {
		if ts.connCount() < 2 {
			return false
		}
		d := declares(ts)
		return len(d) >= 2 && strings.Contains(d[len(d)-1], "005930")
	})
}

func TestStream_ServerShutdownCause(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts)
	waitFor(t, "connection", func() bool { return ts.connCount() >= 1 })
	ts.push(`{"type":"error","error":{"code":"server-shutdown","message":"배포"}}`)
	ts.dropConn()
	select {
	case r := <-s.Reconnects():
		if r.Cause != ReconnectServerShutdown {
			t.Errorf("cause = %q, want server-shutdown", r.Cause)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("reconnect 미수신")
	}
}

func TestStream_WithoutAutoReconnectClosesChannels(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts, WithoutAutoReconnect())
	waitFor(t, "connection", func() bool { return ts.connCount() >= 1 })
	ts.dropConn()
	select {
	case _, ok := <-s.Trades():
		if ok {
			t.Error("자동 재연결이 꺼져 있으면 채널이 닫혀야 한다")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("채널이 닫히지 않았다")
	}
}

func TestStream_TradeBufferDropsOldest(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts, WithTradeBuffer(2))
	waitFor(t, "connection", func() bool { return ts.connCount() >= 1 })
	for _, p := range []string{"1", "2", "3", "4"} {
		ts.push(`{"type":"message","topic":"trade:kr:005930","data":{"price":"` + p + `","volume":"1","timestamp":"2026-03-25T09:30:42.000+09:00","currency":"KRW"}}`)
	}
	// 버퍼가 2 이므로 오래된 것이 버려지고 최신 2개가 남는다
	waitFor(t, "buffered trades", func() bool { return len(s.Trades()) == 2 })
	first := <-s.Trades()
	if first.Price.String() == "1" {
		t.Errorf("가득 찬 시세 버퍼는 오래된 것을 버려야 한다: %s", first.Price)
	}
}

func TestStream_OrderBackpressureReconnects(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts, WithOrderBuffer(1))
	waitFor(t, "connection", func() bool { return ts.connCount() >= 1 })
	ev := `{"type":"message","topic":"personal:order:3","data":{"event":"PENDING","accountSeq":"3","order":{"orderId":"o-1","symbol":"005930","side":"BUY","orderType":"LIMIT","timeInForce":"DAY","status":"PENDING","quantity":"1","currency":"KRW","orderedAt":"2026-03-28T09:30:00+09:00","execution":{"filledQuantity":"0","averageFilledPrice":null,"filledAmount":null,"commission":null,"tax":null,"settlementDate":null}}}}`
	for i := 0; i < 5; i++ { // 소비하지 않고 버퍼를 넘긴다
		ts.push(ev)
	}
	select {
	case r := <-s.Reconnects():
		if r.Cause != ReconnectBackpressure {
			t.Errorf("cause = %q, want backpressure", r.Cause)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("backpressure 재연결 미수신")
	}
}

func TestStream_MaxTopicsRejectedBeforeSend(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts)
	codes := make([]string, MaxTopics+1)
	for i := range codes {
		codes[i] = fmt.Sprintf("%06d", i) // 고유해야 상한에 걸린다(중복 code 는 집합에서 합쳐진다)
	}
	err := s.Subscribe(context.Background(), Subscription{Type: "trade:kr", Codes: codes})
	if err == nil || !strings.Contains(err.Error(), "too many") {
		t.Errorf("상한 초과는 요청 전에 거부돼야 한다: %v", err)
	}
}

func TestStream_CloseIsIdempotent(t *testing.T) {
	ts := newTestServer(t)
	s := newTestStream(t, ts)
	waitFor(t, "connection", func() bool { return ts.connCount() >= 1 })
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("두 번째 Close 도 안전해야 한다: %v", err)
	}
	if err := s.Subscribe(context.Background(), Trade(tosstypes.MarketCountryKR, "005930")); err == nil {
		t.Error("닫힌 스트림에 Subscribe 하면 에러여야 한다")
	}
}

func asRejected(err error, target **RejectedError) bool { return errors.As(err, target) }

func asDeclare(err error, target **DeclareError) bool { return errors.As(err, target) }

// declares 는 스텁이 받은 프레임 중 선언(JSON 배열)만 고른다.
func declares(ts *testServer) []string {
	var out []string
	for _, f := range ts.frames() {
		if strings.HasPrefix(strings.TrimSpace(f), "[") {
			out = append(out, f)
		}
	}
	return out
}
EOF
```

```bash
go test ./stream/ 2>&1 | head -5
```
Expected: 컴파일 에러(`undefined: New`, `undefined: Stream` 등) — `WithCoalesceDelay` 는 이미 Step 2 의 `options.go` 에 포함돼 있다.

- [ ] **Step 5: 구현**

```bash
cat > stream/conn.go << 'EOF'
package stream

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

// DefaultBaseURL 은 토스 실시간 웹소켓 엔드포인트.
const DefaultBaseURL = "wss://openapi-ws.tossinvest.com/ws/v1"

// TokenFunc 는 연결마다 유효한 access token 을 돌려준다(toss.Client.AccessToken).
type TokenFunc func(ctx context.Context) (string, error)

// dial 은 핸드셰이크를 수행한다. 인증은 이 시점 1회뿐이며, 이후 토큰이 만료돼도 연결은 끊기지 않는다.
func dial(ctx context.Context, url string, token TokenFunc) (*websocket.Conn, error) {
	tok, err := token(ctx)
	if err != nil {
		return nil, err
	}
	c, resp, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + tok}},
	})
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		return nil, &ConnectError{StatusCode: status, Err: err}
	}
	// 주문 이벤트 스냅샷이 커질 수 있어 넉넉히 잡는다.
	c.SetReadLimit(1 << 20)
	return c, nil
}

// backoff 는 지수 백오프 대기 시간을 계산한다(±20% jitter).
func backoff(attempt int, min, max time.Duration) time.Duration {
	d := min
	for i := 1; i < attempt && d < max; i++ {
		d *= 2
	}
	if d > max {
		d = max
	}
	jitter := 1 + (rand.Float64()*0.4 - 0.2) //nolint:gosec // 재시도 분산용, 암호학적 강도 불필요
	return time.Duration(float64(d) * jitter)
}

// pingLoop 는 주기적으로 순수 텍스트 PING 을 보낸다.
// 서버는 클라이언트로부터의 수신이 180초 없으면 끊으며, 서버가 보내는 데이터는 이 타이머를 리셋하지 않는다.
func pingLoop(ctx context.Context, c *websocket.Conn, every time.Duration) error {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if err := c.Write(ctx, websocket.MessageText, []byte("PING")); err != nil {
				return fmt.Errorf("toss: ping: %w", err)
			}
		}
	}
}

// closeConn 은 연결을 확실히 닫는다. 재연결 전에 반드시 호출한다 —
// 계정당 연결 2개 한도가 있어, 닫지 않으면 새 연결이 이전 연결을 밀어내 끊김이 반복된다.
func closeConn(c *websocket.Conn) {
	if c == nil {
		return
	}
	_ = c.Close(websocket.StatusNormalClosure, "")
	_ = c.CloseNow()
}

var errStreamClosed = errors.New("toss: stream is closed")
EOF
cat > stream/stream.go << 'EOF'
package stream

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// Stream 은 실시간 웹소켓 스트림이다. toss.Client.Stream 으로 만든다.
// 여러 goroutine 에서 동시에 사용해도 안전하다.
type Stream struct {
	cfg   config
	token TokenFunc
	url   string
	subs  *subscriptionSet

	trades     chan TradeEvent
	orderbooks chan OrderbookEvent
	orders     chan OrderEvent
	reconnects chan Reconnect
	errs       chan error

	declareCh chan struct{} // 선언 요청 신호(코얼레싱)
	seq       atomic.Int64  // 선언 id 생성

	cancel context.CancelFunc
	done   chan struct{}
	closed atomic.Bool

	mu   sync.Mutex
	conn *websocket.Conn // 현재 연결. 쓰기 전에 잠근다
}

// New 는 스트림을 만들고 연결한다. 보통은 toss.Client.Stream 을 쓴다.
func New(ctx context.Context, token TokenFunc, opts ...Option) (*Stream, error) {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}
	url := cfg.baseURL
	if url == "" {
		url = DefaultBaseURL
	}
	s := &Stream{
		cfg: cfg, token: token, url: url, subs: newSubscriptionSet(),
		trades:     make(chan TradeEvent, cfg.tradeBuffer),
		orderbooks: make(chan OrderbookEvent, cfg.tradeBuffer),
		orders:     make(chan OrderEvent, cfg.orderBuffer),
		reconnects: make(chan Reconnect, defaultDiagBuffer),
		errs:       make(chan error, defaultDiagBuffer),
		declareCh:  make(chan struct{}, 1),
		done:       make(chan struct{}),
	}
	conn, err := dial(ctx, url, token)
	if err != nil {
		return nil, err
	}
	s.conn = conn

	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	s.cancel = cancel
	go s.run(runCtx, conn)
	return s, nil
}

// Trades 는 실시간 체결 채널이다. LOSSY — 소비가 밀리면 오래된 이벤트가 버려진다.
func (s *Stream) Trades() <-chan TradeEvent { return s.trades }

// Orderbooks 는 실시간 호가 채널이다. LOSSY — 소비가 밀리면 오래된 이벤트가 버려진다.
func (s *Stream) Orderbooks() <-chan OrderbookEvent { return s.orderbooks }

// Orders 는 본인 주문 이벤트 채널이다. 이벤트를 버리지 않는 대신, 소비가 밀려 버퍼가 차면
// 연결을 끊고 재연결하며 Reconnects() 로 알린다.
func (s *Stream) Orders() <-chan OrderEvent { return s.orders }

// Reconnects 는 재연결 알림 채널이다.
//
// **끊긴 구간의 주문 이벤트는 재전송되지 않는다.** 이 신호를 받으면 REST(Account(seq).Order.List)로
// 주문 상태를 재동기화해야 한다.
func (s *Stream) Reconnects() <-chan Reconnect { return s.reconnects }

// Errors 는 구독 거부·선언 실패·디코딩 오류 채널이다.
func (s *Stream) Errors() <-chan error { return s.errs }

// Subscriptions 는 현재 구독 집합의 스냅샷이다.
func (s *Stream) Subscriptions() []Subscription { return s.subs.snapshot() }

// Subscribe 는 구독을 추가하고 전체 집합을 다시 선언한다(프로토콜이 full-replace 다).
func (s *Stream) Subscribe(ctx context.Context, subs ...Subscription) error {
	if s.closed.Load() {
		return errStreamClosed
	}
	if err := s.subs.add(subs...); err != nil {
		return err
	}
	s.requestDeclare()
	return nil
}

// Unsubscribe 는 구독을 빼고 전체 집합을 다시 선언한다.
func (s *Stream) Unsubscribe(ctx context.Context, subs ...Subscription) error {
	if s.closed.Load() {
		return errStreamClosed
	}
	if err := s.subs.remove(subs...); err != nil {
		return err
	}
	s.requestDeclare()
	return nil
}

// Declare 는 구독 집합을 통째로 바꾼다. 인자가 없으면 전체 구독을 해제한다.
func (s *Stream) Declare(ctx context.Context, subs ...Subscription) error {
	if s.closed.Load() {
		return errStreamClosed
	}
	if err := s.subs.replace(subs...); err != nil {
		return err
	}
	s.requestDeclare()
	return nil
}

// Close 는 재연결을 멈추고 연결을 닫으며 모든 채널을 닫는다. 여러 번 호출해도 안전하다.
func (s *Stream) Close() error {
	if s.closed.Swap(true) {
		return nil
	}
	s.cancel()
	s.mu.Lock()
	closeConn(s.conn)
	s.conn = nil
	s.mu.Unlock()
	<-s.done
	return nil
}

func (s *Stream) requestDeclare() {
	select {
	case s.declareCh <- struct{}{}:
	default: // 이미 예약돼 있으면 합친다(선언 5회/초 한도 대응)
	}
}

// run 은 연결 수명 주기를 관리한다: 읽기·PING·선언 루프를 돌리고, 끊기면 재연결한다.
func (s *Stream) run(ctx context.Context, conn *websocket.Conn) {
	defer close(s.done)
	defer s.closeChannels()

	attempt := 0
	cause := ReconnectReadError
	for {
		connCtx, connCancel := context.WithCancel(ctx)
		causeCh := make(chan ReconnectCause, 1)
		var wg sync.WaitGroup
		wg.Add(3)
		go func() { defer wg.Done(); s.readLoop(connCtx, conn, causeCh); connCancel() }()
		go func() { defer wg.Done(); _ = pingLoop(connCtx, conn, s.cfg.pingInterval); connCancel() }()
		go func() { defer wg.Done(); s.declareLoop(connCtx, conn) }()
		wg.Wait()

		// 재연결 전에 반드시 기존 연결을 닫는다(계정당 2개 한도).
		s.mu.Lock()
		closeConn(s.conn)
		s.conn = nil
		s.mu.Unlock()
		connCancel()

		select {
		case c := <-causeCh:
			cause = c
		default:
		}

		if ctx.Err() != nil || !s.cfg.autoReconnect {
			return
		}

		attempt++
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff(attempt, s.cfg.backoffMin, s.cfg.backoffMax)):
		}
		next, err := dial(ctx, s.url, s.token)
		if err != nil {
			s.emitErr(err)
			continue
		}
		s.mu.Lock()
		s.conn = next
		s.mu.Unlock()
		conn = next
		s.emitReconnect(Reconnect{Attempt: attempt, Cause: cause, At: time.Now()})
		s.requestDeclare() // 저장된 집합을 다시 선언한다
		attempt = 0
		cause = ReconnectReadError
	}
}

// readLoop 는 프레임을 읽어 채널로 디스패치한다.
func (s *Stream) readLoop(ctx context.Context, conn *websocket.Conn, causeCh chan<- ReconnectCause) {
	shutdown := false
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			c := ReconnectReadError
			if shutdown {
				c = ReconnectServerShutdown
			}
			select {
			case causeCh <- c:
			default:
			}
			return
		}
		if typ != websocket.MessageText {
			continue
		}
		f, err := decodeFrame(data)
		if err != nil {
			s.emitErr(err)
			continue
		}
		switch f.kind {
		case frameSubscriptions:
			for _, r := range f.rejected {
				s.subs.reject(r.Target)
				s.emitErr(&RejectedError{Target: r.Target, Code: r.Code, Message: r.Message})
			}
		case frameError:
			if f.errCode == "server-shutdown" {
				shutdown = true
				continue // 곧 연결이 끊긴다 — 에러가 아니라 재연결 사유다
			}
			s.emitErr(&DeclareError{ID: f.id, Code: f.errCode, Message: f.errMessage})
		case frameMessage:
			if !s.dispatch(f) {
				select {
				case causeCh <- ReconnectBackpressure:
				default:
				}
				return // 주문 채널 포화 — 연결을 끊고 재연결한다
			}
		case framePong, frameUnknown:
			// pong 은 소비만 하고, 알 수 없는 프레임은 무시한다
		}
	}
}

// dispatch 는 message 프레임을 해당 채널로 보낸다. 주문 채널이 가득 차면 false 를 돌려준다.
func (s *Stream) dispatch(f frame) bool {
	switch f.topicKind() {
	case typeTradeKR, typeTradeUS:
		ev, err := f.tradeEvent()
		if err != nil {
			s.emitErr(err)
			return true
		}
		pushLossy(s.trades, ev)
	case typeOrderbookKR, typeOrderbookUS:
		ev, err := f.orderbookEvent()
		if err != nil {
			s.emitErr(err)
			return true
		}
		pushLossy(s.orderbooks, ev)
	case typePersonalOrder:
		ev, err := f.orderEvent()
		if err != nil {
			s.emitErr(err)
			return true
		}
		select {
		case s.orders <- ev: // 주문 이벤트는 버리지 않는다
		default:
			return false
		}
	}
	return true
}

// pushLossy 는 가득 차면 가장 오래된 것을 버리고 새 이벤트를 넣는다(시세 LOSSY 규약).
func pushLossy[T any](ch chan T, ev T) {
	for {
		select {
		case ch <- ev:
			return
		default:
			select {
			case <-ch: // 가장 오래된 것을 버린다
			default:
			}
		}
	}
}

// declareLoop 는 코얼레싱된 선언 요청을 처리한다.
func (s *Stream) declareLoop(ctx context.Context, conn *websocket.Conn) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.declareCh:
			if s.cfg.coalesceDelay > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(s.cfg.coalesceDelay):
				}
				// 대기 중 쌓인 요청을 흡수한다
				select {
				case <-s.declareCh:
				default:
				}
			}
			id := "d-" + strconv.FormatInt(s.seq.Add(1), 10)
			body, err := s.subs.declaration(id)
			if err != nil {
				s.emitErr(err)
				continue
			}
			s.mu.Lock()
			c := s.conn
			s.mu.Unlock()
			if c == nil {
				continue
			}
			if err := c.Write(ctx, websocket.MessageText, body); err != nil {
				s.emitErr(fmt.Errorf("toss: declare: %w", err))
				return
			}
		}
	}
}

func (s *Stream) emitErr(err error) {
	select {
	case s.errs <- err:
	default: // 진단 채널이 밀리면 버린다
	}
}

func (s *Stream) emitReconnect(r Reconnect) {
	select {
	case s.reconnects <- r:
	default:
	}
}

func (s *Stream) closeChannels() {
	close(s.trades)
	close(s.orderbooks)
	close(s.orders)
	close(s.reconnects)
	close(s.errs)
}
EOF
```

`client.go` 에 진입점을 추가한다(import 에 `"github.com/kenshin579/toss-go/stream"` 추가):
```go
// Stream 은 실시간 웹소켓 스트림을 연다(체결·호가·본인 주문 이벤트).
// 연결마다 이 클라이언트의 access token 을 쓰므로 별도 인증이 필요 없다.
//
//	s, err := c.Stream(ctx)
//	defer s.Close()
//	s.Subscribe(ctx, stream.Trade(tosstypes.MarketCountryKR, "005930"))
func (c *Client) Stream(ctx context.Context, opts ...stream.Option) (*stream.Stream, error) {
	return stream.New(ctx, c.AccessToken, opts...)
}
```

```bash
gofmt -w . && go vet ./... && go test ./stream/ -race -count=1 -v 2>&1 | grep -cE '^--- PASS'
```
Expected: `39` (Task 1 의 8 + Task 2 의 16 + 이번 15 — `grep -cE '^--- PASS'` 는 앞에 공백이 붙는 서브테스트 줄을 세지 않는다). 실행 결과로 확인하고 모두 PASS 인지만 본다.

- [ ] **Step 6: 커밋**

```bash
go mod tidy && git add stream client.go go.mod go.sum && git commit -m "feat(stream): 연결·PING·재연결·백프레셔 + 공개 Stream API

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 4: 예시 · integration · README · 워크스페이스 CLAUDE.md

**Files:**
- Create: `examples/stream/main.go`
- Modify: `integration_test.go`, `README.md`, `/Users/user/src/workspace_moneyflow/CLAUDE.md`

- [ ] **Step 1: 예시**

```bash
mkdir -p examples/stream && cat > examples/stream/main.go << 'EOF'
// 실시간 시세 구독 예시. 실행: TOSS_CLIENT_ID=... TOSS_CLIENT_SECRET=... go run ./examples/stream
//
// 장 시간이 아니면 이벤트가 오지 않을 수 있다 — 구독 ack 만 확인하고 종료해도 정상이다.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/signal"
	"syscall"
	"time"

	toss "github.com/kenshin579/toss-go"
	"github.com/kenshin579/toss-go/stream"
	"github.com/kenshin579/toss-go/tosstypes"
)

func main() {
	c, err := toss.NewClientFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	s, err := c.Stream(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()

	if err := s.Subscribe(ctx,
		stream.Trade(tosstypes.MarketCountryKR, "005930"),
		stream.Orderbook(tosstypes.MarketCountryKR, "005930"),
	); err != nil {
		log.Fatal(err)
	}
	fmt.Println("구독:", s.Subscriptions())

	// 본인 주문 이벤트를 함께 받으려면 계좌 accountSeq 로 구독한다(부작용 없는 조회성 구독이다).
	//
	//	accts, _ := c.Accounts(ctx)
	//	s.Subscribe(ctx, stream.PersonalOrder(accts[0].AccountSeq))

	timeout := time.After(30 * time.Second)
	for {
		select {
		case t := <-s.Trades():
			fmt.Printf("체결 %s %s %s주 @ %s\n", t.Market, t.Symbol, t.Volume, t.Price)
		case ob := <-s.Orderbooks():
			if len(ob.Asks) > 0 && len(ob.Bids) > 0 {
				fmt.Printf("호가 %s 매도 %s / 매수 %s\n", ob.Symbol, ob.Asks[0].Price, ob.Bids[0].Price)
			}
		case ev := <-s.Orders():
			fmt.Printf("주문 %s %s %s\n", ev.Event, ev.Order.OrderID, ev.Order.Status)
		case r := <-s.Reconnects():
			// 끊긴 구간의 주문 이벤트는 재전송되지 않는다 — REST 로 주문 상태를 다시 맞춘다.
			fmt.Printf("재연결됨(%s, %d번째) — 주문 상태 재동기화 필요\n", r.Cause, r.Attempt)
		case err := <-s.Errors():
			var re *stream.RejectedError
			if errors.As(err, &re) {
				fmt.Printf("구독 거부 %s: %s\n", re.Target, re.Code)
				continue
			}
			fmt.Println("에러:", err)
		case <-timeout:
			fmt.Println("30초 경과, 종료")
			return
		case <-ctx.Done():
			return
		}
	}
}
EOF
go vet ./examples/... && echo VET_OK
```
Expected: `VET_OK`.

- [ ] **Step 2: integration 테스트 — 시세 구독 ack 까지만**

`integration_test.go` 끝에 추가한다. **주문 이벤트는 주문을 내야 발생하므로 검증하지 않는다.**

```bash
cat >> integration_test.go << 'EOF'

// TestIntegration_Stream 은 실시간 스트림 연결과 구독 ack 까지만 검증한다.
// 장 시간이 아니면 체결·호가 이벤트가 오지 않으므로 데이터 도착은 요구하지 않는다.
// 주문 이벤트는 실제 주문을 내야 발생하므로 여기서 검증하지 않는다.
func TestIntegration_Stream(t *testing.T) {
	c := newIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	s, err := c.Stream(ctx)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer s.Close()

	if err := s.Subscribe(ctx, stream.Trade(tosstypes.MarketCountryKR, "005930")); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	// 존재하지 않는 심볼은 rejected 로 돌아온다 — ack 경로가 살아 있다는 증거로 쓴다.
	if err := s.Subscribe(ctx, stream.Trade(tosstypes.MarketCountryKR, "999999")); err != nil {
		t.Fatalf("Subscribe(bad): %v", err)
	}

	select {
	case err := <-s.Errors():
		var re *stream.RejectedError
		if !errors.As(err, &re) {
			t.Fatalf("에러 = %v", err)
		}
		if re.Target != "trade:kr:999999" || re.Code == "" {
			t.Errorf("rejected = %+v", re)
		}
		t.Logf("구독 거부 확인: %s (%s)", re.Target, re.Code)
	case ev := <-s.Trades():
		t.Logf("체결 수신: %s %s @ %s", ev.Symbol, ev.Volume, ev.Price)
	case <-ctx.Done():
		t.Fatal("ack 도 데이터도 받지 못했다")
	}

	// 거부된 항목은 집합에서 자동으로 빠진다
	for _, sub := range s.Subscriptions() {
		for _, code := range sub.Codes {
			if code == "999999" {
				t.Error("거부된 항목이 집합에 남아 있다")
			}
		}
	}
}
EOF
```
import 에 `"errors"`, `"github.com/kenshin579/toss-go/stream"` 를 추가한다(`tosstypes` 는 이미 있다).

```bash
gofmt -w . && go vet -tags integration ./... && echo VET_OK
eval "$(grep -E '^export TOSS_CLIENT_(ID|SECRET)=' ~/.zshrc)" && go test -tags integration -run TestIntegration_Stream -v -count=1 . 2>&1 | tail -20
```
Expected: `VET_OK`. 실호출은 허용 IP 가 등록된 환경에서만 성공한다 — 결과를 그대로 보고한다.
장 시간이 아니면 `구독 거부 확인` 로그만 나오는 것이 정상이다.

- [ ] **Step 3: README**

`## 커버리지` 표 아래에 실시간 섹션을 추가한다:
```markdown
### 실시간(WebSocket)

| 채널 | 접근자 | 전달 보장 |
| --- | --- | --- |
| 체결 | `Trades()` | LOSSY — 소비가 밀리면 오래된 이벤트가 버려진다 |
| 호가 | `Orderbooks()` | LOSSY |
| 본인 주문 | `Orders()` | 연결 세션 내 LOSSLESS |
| 재연결 알림 | `Reconnects()` | — |
| 구독 거부·에러 | `Errors()` | — |
```

`## 사용` 끝에 예시와 주의사항을 넣는다:
````markdown
### 실시간 스트림

```go
s, err := c.Stream(ctx)
defer s.Close()

s.Subscribe(ctx,
    stream.Trade(tosstypes.MarketCountryKR, "005930"),
    stream.PersonalOrder(accountSeq), // codes 는 종목이 아니라 계좌 accountSeq
)

for {
    select {
    case t := <-s.Trades():
        fmt.Println(t.Symbol, t.Price)
    case ev := <-s.Orders():
        fmt.Println(ev.Event, ev.Order.OrderID)
    case r := <-s.Reconnects():
        // 끊긴 구간의 주문 이벤트는 재전송되지 않는다 — REST 로 재동기화한다
        _ = r
        page, _ := a.Order.List(ctx, order.ListParams{Status: order.StatusFilterOpen})
        _ = page
    case err := <-s.Errors():
        fmt.Println(err)
    }
}
```

**실시간 주의**

- 구독은 **선언형 full-replace** 다. SDK 가 현재 집합을 들고 있다가 `Subscribe`/`Unsubscribe` 때마다
  전체를 다시 선언하고, 재연결 시 자동으로 재선언한다. 거부된 항목은 집합에서 자동으로 빠진다.
- **재연결 구간의 주문 이벤트는 복구되지 않는다.** `Reconnects()` 신호를 받으면 REST 로 주문 상태를 맞춘다.
- `Orders()` 를 계속 소비하지 않으면 버퍼가 차고 SDK 가 연결을 끊는다(`ReconnectBackpressure`).
  시세 채널은 대신 오래된 이벤트를 버린다.
- 한도: 계정당 동시 연결 **2개**, 연결당 구독 **100건**(채널×종목 조합), 선언 **5회/초**.
- keepalive PING 은 SDK 가 보낸다(기본 60초). 별도로 보낼 필요가 없다.
````

- [ ] **Step 4: 워크스페이스 CLAUDE.md** — `/Users/user/src/workspace_moneyflow/CLAUDE.md` 의 toss-go 항목 2곳에서 "WS 예정"/"WebSocket 은 후속" 을 실제 상태로 바꾸고, 그룹 목록에 `Stream` 을 추가한다(파일만 수정, 커밋 없음 — git 저장소 아님).

- [ ] **Step 5: 전체 검증 + 커밋**

```bash
gofmt -l . ; go build ./... && go vet ./... && go vet -tags integration ./... && go test ./... -race -count=1 2>&1 | tail -18 && go mod tidy && git status --short
git add -A && git commit -m "docs: 실시간 스트림 예시·integration·README

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```
Expected: 전 패키지 `ok`, `go mod tidy` 후 변경 없음.

---

### Task 5: PR 생성

- [ ] **Step 1: 푸시 + PR (gh + HEREDOC, 리뷰어 지정 금지)**

```bash
git push -u origin feature/websocket && gh pr create --title "feat: WebSocket 실시간 스트림 (v0.3.0)" --body "$(cat <<'EOF'
## Summary
- `stream` 패키지 추가 — 실시간 체결·호가·본인 주문 이벤트를 채널로 수신
- 진입점 `c.Stream(ctx)` — 연결마다 클라이언트의 access token 을 재사용
- **선언형 full-replace 구독을 SDK 가 대신 관리** — `Subscribe`/`Unsubscribe` 는 전체 집합을 다시 선언하고, 재연결 시 자동 재선언, 거부된 항목은 집합에서 자동 제거
- 자동 재연결(지수 백오프 + jitter) + `Reconnects()` 알림 — 끊긴 구간의 주문 이벤트는 복구 불가라 REST 재동기화를 유도
- 백프레셔: 시세는 오래된 이벤트를 버리고(LOSSY 규약), 주문은 버리지 않는 대신 버퍼가 차면 연결을 끊고 재연결
- 설계 `docs/superpowers/specs/2026-09-05-websocket-design.md`, 계획 `docs/superpowers/plans/2026-09-05-websocket.md`

## 프로토콜에서 놓치기 쉬운 지점(테스트로 고정)
- PING 은 JSON 이 아니라 **순수 텍스트 `PING`**. 서버는 클라이언트 송신이 180초 없으면 끊으며 수신 데이터는 타이머를 리셋하지 않는다.
- **재연결 전에 기존 연결을 먼저 닫는다** — 계정당 연결 2개 한도라 안 닫으면 자기 자신을 밀어내 끊김이 반복된다.
- `personal:order` 의 codes 는 종목이 아니라 **accountSeq**.
- 거부된 구독 항목은 고치기 전엔 재선언해도 계속 거부되므로 SDK 가 집합에서 자동 제거한다.

## Test plan
- [x] `go build ./... && go vet ./... && go vet -tags integration ./... && go test ./... -race` 통과
- [x] 스텁 웹소켓 서버로 선언 직렬화·ack·프레임 디스패치·PING·재연결/재선언·백프레셔·상한 검증
- [x] `go test -tags integration -run TestIntegration_Stream` — 연결·구독 ack 확인(장 시간 밖에서는 데이터 미도착이 정상)
- [ ] 머지 후 `./scripts/release.sh v0.3.0`

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)" && gh pr view --json number,title,url,baseRefName,headRefName,reviewRequests
```
Expected: PR URL, base `main`, head `feature/websocket`, reviewRequests 비어 있음.

---

## 머지 후 (사용자 머지 뒤 실행)

```bash
git checkout main && git pull origin main && ./scripts/release.sh v0.3.0
```
이후 메모리(`toss_go_library.md`)를 갱신한다. toss-go 는 REST 36 ops + 실시간까지 완성되며, 남은 후속은
moneyflow 통합이다.
