# toss-go WebSocket 실시간 스트림 설계 (v0.3.0)

> 내부 개발 문서(설계/실행 계획). 라이브러리 사용법은 [README](../../../README.md) 를 보세요.

- 작성일: 2026-09-05
- 상태: 확정 (브레인스토밍 완료)
- 레포: `github.com/kenshin579/toss-go` (워크스페이스 `toss-go/`, branch `feature/websocket`)
- 선행: v0.2.0 (조회 21 + 계좌·주문 15 = 36 ops, main merged 6b2e91c)
- 문서 정본: `docs/api/asyncapi.json` (AsyncAPI 3.0, v1.2.2) + `docs/api/overview.md` 의 「웹소켓 연동」
- 토픽: 실시간 체결·호가·본인 주문 이벤트 — SDK 의 마지막 미구현 영역

## 배경 / 목적

REST 36 ops 는 끝났고 남은 것은 실시간 스트림뿐이다. REST 와 성격이 근본적으로 다르다:
요청-응답이 아니라 **연결을 유지하며 밀어주는** 통로이고, 구독은 **선언형 full-replace** 이며,
채널마다 **전달 보장이 다르다**(시세 LOSSY / 주문 LOSSLESS). 설계 판단은 이 세 가지에서 갈린다.

## 사전 조사 결과 (확정 사실, asyncapi.json 1.2.2 + overview.md)

- **엔드포인트 1개**: `wss://openapi-ws.tossinvest.com/ws/v1`. 핸드셰이크에 `Authorization: Bearer {access_token}`
  (REST 와 같은 토큰). 성공 `101`, 토큰 문제 `401`, 허용 IP 미등록 `403`, 서버 오류 `503`.
  **인증은 핸드셰이크 1회뿐** — 연결 유지 중 토큰이 만료돼도 연결은 끊기지 않는다.
- **구독은 선언형 full-replace**: JSON **배열 1개**를 텍스트 프레임으로 보내면 그 배열이 곧 현재 구독 전체다.
  `subscribe`/`unsubscribe` 액션이 없다. 빠진 항목은 자동 해제, 빈 배열 `[]` 은 전체 해제.
- **구독 원소 4종**(`oneOf`):
  | 원소 | 필드 | codes 내용 |
  |---|---|---|
  | 요청 id(선택) | `{"id":"req-1"}` | — 배열에 1개 넣으면 ack·error 에 echo |
  | 체결 | `{"type":"trade:us"\|"trade:kr","codes":[...]}` | 종목 symbol |
  | 호가 | `{"type":"orderbook:us"\|"orderbook:kr","codes":[...]}` | 종목 symbol |
  | 주문 | `{"type":"personal:order","codes":[...]}` | **계좌 accountSeq(문자열)** — 종목 아님 |
  국내는 통합 시세(KRX+NXT)만 제공한다. 미국 티커는 대문자 등 종목 마스터 표기 그대로 써야 한다.
- **수신 프레임 4종**(top-level `type` 으로 구분):
  | type | 형태 | 시점 |
  |---|---|---|
  | `subscriptions` | `{type, id?, subscribed[], rejected[{target,code,message}]}` | 선언 직후, 데이터보다 먼저 |
  | `message` | `{type, topic, data}` | 구독 대상 갱신 시 |
  | `error` | `{type, id?, error{code,message}}` | 선언 전체 실패 또는 `server-shutdown` |
  | `pong` | `{type:"pong"}` | 텍스트 `PING` 송신 직후 |
- **topic 형식** = 구독 `type` + `:` + code. `trade:us:AAPL`, `orderbook:kr:005930`, `personal:order:3`.
- **PING 은 JSON 이 아니라 순수 텍스트** `PING`(대문자 4글자). 서버는 `{"type":"pong"}` 으로 답한다.
  **클라이언트로부터의 수신이 180초 없으면 서버가 끊는다.** 서버가 보내는 데이터는 이 타이머를 리셋하지
  않으므로 데이터를 받는 중에도 주기적으로 보내야 한다(60초 권장). 표준 ping/pong 프레임도 지원된다.
- **한도**: 계정당 동시 연결 **2개**(초과 시 새 연결 수락 + 가장 오래된 연결 종료), 연결당 구독 **100건**
  (`codes` 합산, 채널×종목 조합 기준 — `trade:us:AAPL` + `orderbook:us:AAPL` = 2건), 선언 **5회/초**.
  초과 시 각각 `too-many-topics`, `rate-limit-exceeded` 에러 프레임. `Retry-After` 는 없고 약 1초 후 재선언.
- **에러 코드**: `error.code` 는 `wrong-format`·`no-type`·`invalid-type`·`no-codes`·`too-many-topics`·
  `too-many`·`rate-limit-exceeded`·`internal-error`·`server-shutdown`.
  `rejected[].code` 는 `stock-not-found`·`symbol-market-mismatch`·`account-not-found`.
  **거부된 항목은 원인을 고치기 전에는 재선언해도 같은 이유로 다시 거부된다.**
- **전달 보장**: 시세(`trade`·`orderbook`)는 **LOSSY**(밀리면 중간 프레임 유실, 유실 감지용 sequence 없음),
  주문(`personal:order`)은 **LOSSLESS**(미소비분을 건너뛰지 않음, 다만 **수신이 2초 이상 막히면 연결 종료**).
  LOSSLESS 는 **연결 세션 내에 한정**되며 끊긴 구간의 이벤트는 재전송되지 않는다 —
  **재연결 후 `GET /api/v1/orders` 로 재동기화해야 한다.**
- **종료·재연결**: 서버 배포 시에는 `server-shutdown` 프레임이 먼저 오고 곧 끊긴다. 그 외(idle 초과,
  주문 backpressure, 연결 한도 초과로 밀려남)는 close code 없이 비정상 종료로 보일 수 있다.
  지수 백오프(1s→2s→4s, jitter)로 재시도하고 구독을 다시 선언한다.
  **재연결 전에 쓰던 연결을 먼저 닫아야 한다** — 남아 있으면 새 연결이 앞의 연결을 밀어내 끊김이 반복된다.
- **주문 이벤트 payload**: `{event, accountSeq, order}`. `event` 10종(PENDING, PARTIAL_FILL, FILL,
  CANCELING, CANCELED, REPLACING, REPLACED, REJECTED, CANCEL_REJECTED, REPLACE_REJECTED).
  `order` 는 REST `GET /api/v1/orders/{orderId}` 와 동일 구조이나 **`execution.filledAt` 이 없다.**
- 참고: korea-investment-stock 의 `websocket` 패키지는 엔드포인트마다 `SubscribeXxx`/`OnXxx` 를 두는
  형태(30+ 메서드)인데, 토스는 구독 모델이 달라 그 방식을 그대로 쓸 수 없다.

## 결정 사항 (브레인스토밍)

1. **채널 수신 API.** 콜백이 아니라 Go 채널로 이벤트를 넘긴다. 토스가 채널별로 전달 보장을 다르게
   정의했으므로 시세용(가득 차면 버림)과 주문용(버리지 않음) 버퍼를 분리해 **LOSSY/LOSSLESS 차이를
   타입과 동작으로 드러낸다**. 단일 이벤트 채널 + 타입 스위치는 이 구분을 흐려 기각.
2. **구독 집합은 SDK 가 소유하고, `Declare` 도 함께 제공한다.** `Subscribe`/`Unsubscribe` 로 집합을
   갱신하면 SDK 가 전체 배열을 다시 선언한다. 재연결 시 저장된 집합으로 자동 재선언하고, ack 의
   `rejected` 항목은 집합에서 자동 제거한다(고치기 전엔 계속 거부되는 함정 회피). 원본 모델을 원하는
   사용자를 위해 `Declare`(집합 통째 교체)도 남긴다.
3. **자동 재연결 + 자동 재선언.** 지수 백오프(1s→2s→4s, 상한 30s, jitter). 재연결 사실은 **옵션이 아니라
   필수 경로**로 알린다 — `Reconnects()` 채널에 이벤트를 보내고, 받으면 `GET /orders` 로 주문 상태를
   재동기화하라고 godoc·README 에 명시한다. SDK 가 대신 재동기화하지는 않는다(무엇을 어떻게 쓸지 모름).
4. **진입점은 `Client.Stream(ctx)`.** 재연결마다 새 토큰이 필요하고 클라이언트가 이미 토큰을 캐시하므로,
   사용자가 토큰 문자열을 넘기는 형태보다 낫다.
5. **주문 채널 백프레셔는 "유한 버퍼 + 가득 차면 연결 종료 후 재연결".** 무제한 버퍼는 장애를 메모리
   증가로 미룰 뿐이고 어차피 재연결 구간에서 이벤트가 유실된다. 유실이 불가피하다면 사용자가 즉시 알고
   REST 로 맞추는 편이 안전하다. 서버의 2초 룰이 발동하게 두는 방식(블로킹)은 원인이 불투명해 기각.
6. **의존성 `github.com/coder/websocket` v1.8.14 추가.** 표준 라이브러리만으로는 WebSocket 을 구현할 수
   없다. KIS SDK 가 같은 라이브러리를 쓰고 있고 context 친화적이다.

## 아키텍처

```
toss-go/
├── client.go               # (수정) Client.Stream(ctx) 추가
├── stream/
│   ├── stream.go           # 공개 API: Stream, Subscribe/Unsubscribe/Declare, 채널 접근자, Close
│   ├── subscription.go     # 구독 원소 생성자(Trade/Orderbook/Order) + 집합 관리·직렬화
│   ├── conn.go             # 핸드셰이크·PING·재연결 루프·백오프
│   ├── frames.go           # 수신 프레임 디코딩·디스패치(subscriptions/message/error/pong)
│   ├── events.go           # 공개 이벤트 타입(TradeEvent/OrderbookEvent/OrderEvent/Reconnect)
│   ├── options.go          # 버퍼 크기·PING 주기·백오프·자동 재연결 on/off
│   └── *_test.go           # 실제 WebSocket 업그레이드 스텁 서버 기반
└── examples/stream/main.go # 시세 구독 예시(주문 구독은 주석)
```

### 공개 API

```go
// 루트
func (c *Client) Stream(ctx context.Context, opts ...stream.Option) (*stream.Stream, error)

// stream 패키지
type Stream struct{ /* unexported */ }

func (s *Stream) Subscribe(ctx context.Context, subs ...Subscription) error
func (s *Stream) Unsubscribe(ctx context.Context, subs ...Subscription) error
func (s *Stream) Declare(ctx context.Context, subs ...Subscription) error // 집합 통째 교체
func (s *Stream) Subscriptions() []Subscription                          // 현재 집합 스냅샷
func (s *Stream) Close() error

func (s *Stream) Trades() <-chan TradeEvent
func (s *Stream) Orderbooks() <-chan OrderbookEvent
func (s *Stream) Orders() <-chan OrderEvent
func (s *Stream) Reconnects() <-chan Reconnect
func (s *Stream) Errors() <-chan error

// 구독 생성자
func Trade(market tosstypes.MarketCountry, symbols ...string) Subscription
func Orderbook(market tosstypes.MarketCountry, symbols ...string) Subscription
func PersonalOrder(accountSeqs ...int64) Subscription
```

- 모든 채널은 `Close()` 또는 자동 재연결을 포기했을 때 닫힌다. 소비자는 `for ... range` 또는 `select` 로 읽는다.
- `Subscription` 은 `{Type string; Codes []string}` 를 감싼 불투명 값 타입. 같은 `Type` 끼리는 집합 갱신 시
  합쳐서 하나의 배열 원소로 직렬화한다.

### 구독 집합과 선언

- SDK 는 `map[type]set[code]` 로 현재 집합을 들고 있다. `Subscribe`/`Unsubscribe`/`Declare` 는 집합을 갱신한
  뒤 **전체를 다시 선언**한다(프로토콜이 full-replace 이므로 부분 전송이 없다).
- 선언마다 `id` 를 채워 보내고(예: `d-7`), 같은 `id` 의 `subscriptions` ack 를 기다려 결과를 확정한다.
  ack 의 `rejected[]` 는 집합에서 제거하고 `Errors()` 로 사유를 전달한다.
- **사전 검사**: 총 codes 합이 100 초과면 요청 전에 에러. 심볼 형식은 REST 와 같은 `params.Symbol` 규칙을
  쓰되, `personal:order` 의 codes 는 accountSeq(양수)로 검증한다.
- **선언 빈도 제어**: 5회/초 한도가 있으므로 짧은 코얼레싱 창(기본 100ms)을 두고 연속 호출을 한 번의 선언으로
  묶는다. `rate-limit-exceeded` 를 받으면 1초 후 1회 재선언한다.
- 집합이 비면 id 와 무관하게 `[]` 를 보낸다(토스는 빈 배열만 전체 해제로 해석한다).

### 연결·재연결

- 연결마다 `Client.AccessToken(ctx)` 로 토큰을 얻어 핸드셰이크 헤더에 싣는다(만료 토큰으로 재연결하지 않도록).
- 60초(옵션)마다 텍스트 프레임 `PING` 송신. 응답 `pong` 은 소비하고 사용자에게 노출하지 않는다.
- 읽기 루프가 종료되면(에러·EOF·`server-shutdown`) **기존 연결을 먼저 닫고** 백오프 후 재연결한다.
  성공 시 저장된 집합을 재선언하고 `Reconnects()` 에 `Reconnect{Attempt, Cause, At}` 를 보낸다.
- 백오프 기본값: 1s 시작, 2배씩, 상한 30s, ±20% jitter. `WithoutAutoReconnect()` 면 채널을 닫고 종료한다.
- `Close()` 는 재연결을 멈추고 연결을 닫으며 모든 채널을 닫는다. 두 번 호출해도 안전하다.

### 백프레셔

| 채널 | 기본 버퍼 | 가득 찼을 때 |
|---|---|---|
| `Trades()` / `Orderbooks()` | 1024 | **가장 오래된 것을 버리고** 새 이벤트를 넣는다(LOSSY 규약과 일치) |
| `Orders()` | 256 | **버리지 않는다.** 연결을 끊고 재연결하며 `Reconnect{Cause: ReconnectBackpressure}` 를 보낸다 |
| `Errors()` / `Reconnects()` | 16 | 가득 차면 버린다(진단용) |

버퍼 크기는 `WithTradeBuffer`/`WithOrderBuffer` 로 조절한다. 주문 채널을 막는 소비자는 결국 서버의 2초 룰로
끊기므로, SDK 가 먼저 명시적으로 끊고 재연결 사유를 알리는 편이 진단 가능하다.

## 이벤트 타입

```go
type TradeEvent struct {
    Market    tosstypes.MarketCountry // topic 에서 파싱
    Symbol    string                  // topic 에서 파싱
    Price     decimal.Decimal
    Volume    decimal.Decimal
    Timestamp time.Time
    Currency  tosstypes.Currency
}

type Level struct{ Price, Volume decimal.Decimal }

type OrderbookEvent struct {
    Market    tosstypes.MarketCountry
    Symbol    string
    Timestamp *time.Time // 데이터 미제공 시 nil
    Currency  tosstypes.Currency
    Asks      []Level // 낮은 가격순
    Bids      []Level // 높은 가격순
}

type OrderEventType string // PENDING, PARTIAL_FILL, FILL, CANCELING, CANCELED, REPLACING, REPLACED, REJECTED, CANCEL_REJECTED, REPLACE_REJECTED

type OrderEvent struct {
    Event      OrderEventType
    AccountSeq int64       // codes 는 문자열이지만 사용 편의를 위해 int64 로 변환
    Order      order.Order // REST 와 동일 구조. 단 Execution.FilledAt 은 항상 nil
}

type ReconnectCause string // "server-shutdown", "backpressure", "read-error", "idle"

type Reconnect struct {
    Attempt int
    Cause   ReconnectCause
    At      time.Time
}
```

- `order.Order` 를 재사용한다(`stream` 이 `order` 를 import). 필드가 같고 중복 정의가 드리프트를 만든다.
  **`Execution.FilledAt` 이 스트림에는 없다**는 사실은 `OrderEvent.Order` 주석에 명시한다.
- unknown enum 값은 그대로 보존한다(문자열 타입) — asyncapi 가 명시적으로 요구.

## 에러 처리

- 연결 실패는 `Stream()` 이 에러로 반환한다. 핸드셰이크 401/403 은 `*toss.AuthError` 성격이지만 WebSocket
  업그레이드 실패이므로 `*stream.ConnectError{StatusCode, Code}` 로 감싸 상태코드를 노출한다.
- 연결 후의 실패는 전부 `Errors()` 채널로 나간다: 선언 실패(`error` 프레임), 항목 거부(`rejected[]`),
  디코딩 실패. 각각 `*stream.DeclareError{Code, Message, ID}` / `*stream.RejectedError{Target, Code, Message}`.
- `server-shutdown` 은 에러가 아니라 재연결 사유로 처리한다(`Reconnects()` 로 나간다).

## 테스트

- **스텁 서버**: `httptest.NewServer` + `coder/websocket` 업그레이드로 실제 WebSocket 핸드셰이크를 태운다.
  헬퍼는 `internal/testutil` 이 아니라 `stream/testserver_test.go` 에 둔다(외부 의존성이 테스트에만 필요).
- 선언 직렬화: full-replace 배열 형태, 같은 type 병합, `id` 포함, 빈 집합이면 `[]`.
- ack 처리: `subscribed` 반영, `rejected` 항목이 집합에서 빠지고 `Errors()` 로 나가는지.
- 프레임 디스패치 4종 + 알 수 없는 `type` 무시. topic 파싱(`trade:us:AAPL` → Market/Symbol).
- PING: 주기적으로 텍스트 `PING` 이 나가는지(짧은 주기 옵션으로 가속), `pong` 은 사용자에게 노출되지 않는지.
- 재연결: 서버가 강제 종료 → 백오프 후 재연결 → **저장된 집합이 재선언되는지** → `Reconnects()` 수신.
  `server-shutdown` 프레임 → 재연결. `WithoutAutoReconnect()` → 채널이 닫히는지.
- 백프레셔: 시세 채널을 안 읽으면 오래된 것이 버려지고 최신이 들어오는지, 주문 채널을 안 읽으면 연결이
  끊기고 `ReconnectBackpressure` 가 나가는지.
- 상한: codes 합 101건이면 선언 전에 에러, 연속 호출이 코얼레싱되는지.
- **integration**(`-tags integration`): 시세 구독만 실호출한다(`trade:kr` 삼성전자 등). 장 시간이 아니면
  이벤트가 없을 수 있으므로 **ack 수신까지만 검증**하고 데이터 도착은 요구하지 않는다.
  **주문 이벤트는 주문을 내야 발생하므로 integration 에서 검증하지 않는다.**

## 릴리스 & 문서

- README 에 실시간 섹션 추가: 사용 예시, LOSSY/LOSSLESS 차이, **재연결 후 REST 재동기화 필요**,
  연결 2개·구독 100건·선언 5회/초 한도, PING 은 SDK 가 알아서 보낸다는 점.
- `examples/stream/main.go` — 시세 구독 예시. 주문 구독은 주석으로 두되 코드는 남긴다(실주문과 달리
  주문 **구독**은 부작용이 없으므로 주석 해제만으로 동작하도록).
- 완료 후 main 머지 → `v0.3.0` 태그.

## 범위 밖 / 후속

- 시세 스냅샷 복원(재연결 시 REST 로 현재가·호가 채우기), 자동 REST 주문 재동기화.
- 다중 연결 풀링(계정당 2개 한도를 활용한 분산), 구독 우선순위.
- moneyflow 통합.

## 위험 / 주의

- **재연결 구간의 주문 이벤트는 복구 불가**. SDK 가 할 수 있는 최선은 재연결 사실을 놓칠 수 없게 알리는 것.
  README·godoc·예시 세 곳에 재동기화 안내를 넣는다.
- **연결 2개 한도**: 재연결 시 기존 연결을 먼저 닫지 않으면 자기 자신을 밀어내 끊김이 반복된다. 구현에서
  가장 흔한 실수 지점이므로 테스트로 고정한다.
- **PING 은 JSON 이 아니다**. `{"type":"PING"}` 로 보내면 동작하지 않는다.
- `personal:order` 의 codes 는 종목이 아니라 accountSeq 다. 형식 검증을 종목 규칙으로 하면 안 된다.
- 허용 IP 는 REST 와 동일 목록. 미등록 IP 는 핸드셰이크 403.
- 장 시간 밖에는 시세 이벤트가 오지 않는다 — integration 테스트가 데이터 도착을 요구하면 안 된다.
