# toss-go 계좌·주문 15 ops 설계 (v0.2.0)

> 내부 개발 문서(설계/실행 계획). 라이브러리 사용법은 [README](../../../README.md) 를 보세요.

- 작성일: 2026-09-04
- 상태: 확정 (브레인스토밍 완료)
- 레포: `github.com/kenshin579/toss-go` (워크스페이스 `toss-go/`, branch `feature/account-order`)
- 선행: v0.1.0 (조회 21 ops, main merged 493ceb0). 문서 정본은 `docs/api/openapi.json`
- 토픽: 계좌·자산·주문·주문정보·조건주문 15 ops — SDK 의 첫 쓰기(write) 경로

## 배경 / 목적

v0.1.0 은 조회 21 ops 만 담았다. 본 스펙은 **실제 돈이 오가는 주문 경로**를 포함한 15 ops 를 추가한다.
조회와 달리 (1) 계좌 헤더가 필요하고, (2) POST/DELETE 가 처음 등장하며, (3) 잘못된 호출의 비용이
에러 메시지가 아니라 **잘못된 주문**이다. 설계 판단은 전부 이 비대칭을 기준으로 한다.

## 사전 조사 결과 (확정 사실, openapi.json 1.2.14)

| 그룹(tagGroup) | ops | 태그 |
|---|---|---|
| 계좌·자산 | 2 | Account(1), Asset(1) |
| 주문 | 8 | Order(3), Order History(2), Order Info(3) |
| 조건주문 | 5 | Conditional Order(3), Conditional Order History(2) |

- **계좌 헤더**: `GET /api/v1/accounts` 를 뺀 **14개 전부** `X-Tossinvest-Account: {accountSeq}`(int64) 필요.
  `accountSeq` 는 `GET /api/v1/accounts` 응답에서 얻는다. 누락 시 `400 account-header-required`.
- **HTTP 메서드**: GET 7, POST 6, DELETE 1. 현재 `internal/httpclient` 는 `Get` 만 있다.
- **조건주문 취소는 `204 No Content`** — 본문이 없어 `fetch.One` 으로 디코딩할 수 없다.
- **주문 생성 바디는 `oneOf` 2종**(정확히 하나만):
  - `OrderCreateQuantityBased`: required `symbol, side, orderType, quantity`. 선택 `price`,
    `timeInForce`(기본 DAY), `clientOrderId`, `confirmHighValueOrder`. 소수점 수량은 **US MARKET SELL 전용**.
  - `OrderCreateAmountBased`: required `symbol, side, orderType(MARKET 고정), orderAmount`.
    **US 전용**(달러 금액). 체결 수량은 시장가에 따라 결정.
  - 금액 주문·소수점 수량 주문은 **정규장 시작 ~ 정규장 종료 1시간 전**에만 접수
    (`422 amount-order-outside-regular-hours` / `422 fractional-quantity-outside-regular-hours`).
- **`clientOrderId` = 멱등성 키**: 최대 36자, 영숫자와 `-`, `_`. **10분간 유효**하며 같은 값으로
  재요청하면 이전 주문 결과를 그대로 재반환한다. 미전달 시 멱등성 미적용(매 요청이 별개 주문).
  **서버는 자동 생성하지 않는다.**
- **`confirmHighValueOrder`**: 기본 false. 1억원 이상 주문에 true 가 아니면 `400 confirm-high-value-required`.
- **주문 상태**: `OrderStatus` 10종(PENDING, PENDING_CANCEL, PENDING_REPLACE, PARTIAL_FILLED, FILLED,
  CANCELED, REJECTED, CANCEL_REJECTED, REPLACE_REJECTED, REPLACED).
  조건주문 상태 6종(WATCHING, PAUSED, ORDERING, ORDERED, COMPLETED, EXPIRED),
  조건 상태 8종(+HOLDING, CANCELED), 조건 유형 2종(STOP, PROFIT_RATE), 조건주문 유형 3종(SINGLE, OCO, OTO).
- **페이지네이션**: 주문/조건주문 목록은 `cursor` + `hasNext` + `nextCursor`(조회 21 ops 의
  `nextBefore`/`nextUntil` 과 다른 세 번째 방식).
- **에러 코드**: 주문 생성만 28종(`insufficient-buying-power`, `order-hours-closed`,
  `invalid-tick-size`, `price-out-of-range`, `stock-restricted`, `max-order-amount-exceeded`,
  `opposite-pending-order-exists`, `idempotency-key-conflict`, `request-in-progress` 등),
  정정 21종, 취소 7종. 대부분 상태 의존이라 사전 검증이 불가능하고 **문서화가 유일한 방어**다.
- **Rate limit 그룹**: ORDER 10/s(피크 09:00~09:10 도 10/s), ORDER_HISTORY 5/s,
  ORDER_INFO 6/s(피크 3/s), CONDITIONAL_ORDER 5/s, CONDITIONAL_ORDER_HISTORY 10/s,
  ACCOUNT **1/s**, ASSET 5/s.
- **응답 예시(fixture 원천)**: 조회 계열은 openapi `examples` 보유 —
  accounts(brokerageAccount/emptyAccounts), holdings(5종), orders(3종), order 상세(3종),
  buying-power(krw/usd), sellable-quantity(kr/us), commissions(standard/unlimited).
  **쓰기 계열(POST/DELETE)은 2xx 예시가 없다** — 4xx 예시만 있다.

## 결정 사항 (브레인스토밍)

1. **실주문 금지.** integration 테스트는 **조회 9개만** 실호출한다(accounts, holdings, orders 목록,
   order 상세, buying-power, sellable-quantity, commissions, conditional-orders 목록·상세).
   생성·정정·취소·조건주문 생성/수정/취소는 **단위 테스트(fixture)로만** 검증한다. 플래그로도 실주문을
   내지 않는다 — 실수 비용이 금전이다.
2. **계좌 스코프 클라이언트.** `a := c.Account(accountSeq)` 가 헤더가 고정된 핸들을 돌려준다.
   메서드마다 `accountSeq` 를 받는 방식(반복·오발주 위험)과 클라이언트 생성 시 고정(다계좌 불가)을 기각.
3. **주문 생성은 메서드 2개.** `Place`(수량 기준)와 `PlaceAmount`(금액 기준, US 시장가 전용)로 나눠
   `oneOf` 를 타입으로 강제한다. 단일 메서드 + 런타임 검증은 잘못된 조합이 컴파일에서 안 걸려 기각.
4. **쓰기 재시도는 멱등성 키가 있을 때만.** 401 토큰 오류 시, `clientOrderId` 가 설정된 요청만 1회
   재시도한다. 없으면 재시도하지 않고 에러를 반환한다(서버가 접수한 뒤 401 이 난 극단적 경우의 중복 주문 방지).
5. **`clientOrderId` 자동 생성 안 함.** `toss.NewClientOrderID()` 헬퍼만 제공한다. 자동 생성은 호출마다
   새 키가 되어 앱 레벨 재시도에 도움이 안 되면서 "멱등성이 켜졌다"는 착각만 준다.
6. **패키지는 tagGroup 3개.** 태그 7개를 그대로 쪼개면 과분할이라 `asset`, `order`, `conditionalorder`
   로 묶는다. 헤더가 필요 없는 `Accounts` 만 루트에 둔다.

## 아키텍처

```
toss-go/
├── client.go               # (수정) Accounts(ctx), Account(seq) 추가
├── clientorderid.go        # (신규) NewClientOrderID()
├── account.go              # (신규) AccountScope — Asset/Order/ConditionalOrder 보유
├── internal/httpclient/    # (수정) Post/Delete, 헤더 주입, 204 처리, 쓰기 재시도 정책
├── internal/fetch/         # (수정) PostOne/PostNoContent 추가
├── asset/                  # (신규) Holdings
├── order/                  # (신규) Place/PlaceAmount/Modify/Cancel/List/Get/
│                           #        BuyingPower/SellableQuantity/Commissions
└── conditionalorder/       # (신규) Place/Modify/Cancel/List/Get
```

### 사용 흐름

```go
c, _ := toss.NewClientFromEnv()

accts, _ := c.Accounts(ctx)                 // 헤더 불필요
a := c.Account(accts[0].AccountSeq)         // 네트워크 호출 없음, 헤더 고정

h, _ := a.Asset.Holdings(ctx, asset.HoldingsParams{Symbol: "005930"})
bp, _ := a.Order.BuyingPower(ctx, tosstypes.CurrencyKRW)

res, err := a.Order.Place(ctx, order.Request{
    Symbol: "005930", Side: order.SideBuy, OrderType: order.TypeLimit,
    Quantity: decimal.NewFromInt(1), Price: decimal.NewFromInt(70000),
    ClientOrderID: toss.NewClientOrderID(), // 멱등성 + 401 재시도 허용
})
```

### `AccountScope`

```go
// AccountScope 는 특정 계좌(accountSeq)에 고정된 sub-client 묶음이다.
// 모든 요청에 X-Tossinvest-Account 헤더가 자동으로 실린다.
type AccountScope struct {
    Asset            *asset.Client
    Order            *order.Client
    ConditionalOrder *conditionalorder.Client
    // accountSeq 는 불변이며 AccountSeq() 로만 읽는다.
}

func (c *Client) Account(accountSeq int64) *AccountScope // 네트워크 호출 없음
func (c *Client) Accounts(ctx context.Context) ([]Account, error) // 헤더 불필요
```

- `Account` 타입(`AccountNo`, `AccountSeq int64`, `AccountType`)은 루트에 둔다(`accounts.go`).
  `AccountType` 4종(BROKERAGE, OVERSEAS_DERIVATIVES, PENSION_SAVINGS, RESHORING_INVESTMENT)은
  `tosstypes` 에 상수로 추가.
- 그룹 생성자는 기존 관례를 따르되 헤더를 받는다: `order.New(hc, accountSeq)`.

### `internal/httpclient` 확장

```go
// Request 는 한 번의 HTTP 호출을 기술한다.
type Request struct {
    Method     string      // GET/POST/DELETE
    Path       string
    Query      url.Values
    Body       any         // nil 이 아니면 JSON 직렬화
    AccountSeq int64       // 0 이 아니면 X-Tossinvest-Account 헤더
    IdempotencyKey string  // 바디에 실린 멱등성 키(clientOrderId). 비어 있지 않은 쓰기만 401 재시도
    Out        any         // nil 이면 응답 본문 무시(204 포함)
}

func (c *Client) Do(ctx context.Context, r Request) error
func (c *Client) Get(ctx, path string, q url.Values, out any) error // 기존 시그니처 유지 = Do(GET) — GET 은 IdempotencyKey 없이도 항상 재시도
```

- **GET 은 항상 재시도**(HTTP 정의상 멱등). 쓰기는 `IdempotencyKey` 가 있을 때만 재시도.
- **204 / 빈 본문**: `Out == nil` 이면 본문을 읽지 않고 성공 처리. `Out != nil` 인데 204 면 에러.
- 헤더: `Authorization: Bearer`, `Accept: application/json`, 바디가 있으면
  `Content-Type: application/json`, `AccountSeq != 0` 이면 `X-Tossinvest-Account`.
- 에러 매핑·429 `RetryAfter`·`{result}` 봉투 해제는 기존 로직 재사용.

### `internal/fetch` 확장

```go
func PostOne[T any](ctx, hc, path string, body any, accountSeq int64, clientOrderID string) (*T, error)
func Send(ctx, hc, method, path string, q url.Values, body any, accountSeq int64) error // 본문 없는 응답(204)
```

기존 `One`/`List` 는 계좌 헤더를 받도록 시그니처를 확장한다(내부 패키지라 호환성 부담 없음).

## 엔드포인트 매핑 (15 ops)

| 패키지 | 메서드 | HTTP |
|---|---|---|
| 루트 | `Accounts(ctx) ([]Account, error)` | GET /accounts |
| asset | `Holdings(ctx, HoldingsParams) (*Holdings, error)` — zero value 면 전체 | GET /holdings |
| order | `Place(ctx, Request) (*PlaceResult, error)` | POST /orders |
| order | `PlaceAmount(ctx, AmountRequest) (*PlaceResult, error)` | POST /orders |
| order | `Modify(ctx, orderID string, ModifyRequest) (*OperationResult, error)` | POST /orders/{id}/modify |
| order | `Cancel(ctx, orderID string) (*OperationResult, error)` | POST /orders/{id}/cancel |
| order | `List(ctx, ListParams{Status, Symbol, From, To, Cursor, Limit}) (*Page, error)` | GET /orders |
| order | `Get(ctx, orderID string) (*Order, error)` | GET /orders/{id} |
| order | `BuyingPower(ctx, currency) (*BuyingPower, error)` | GET /buying-power |
| order | `SellableQuantity(ctx, symbol string) (*SellableQuantity, error)` | GET /sellable-quantity |
| order | `Commissions(ctx) ([]Commission, error)` | GET /commissions |
| conditionalorder | `Place(ctx, Request) (*PlaceResult, error)` | POST /conditional-orders |
| conditionalorder | `Modify(ctx, id string, ModifyRequest) (*Result, error)` | POST /conditional-orders/{id}/modify |
| conditionalorder | `Cancel(ctx, id string) error` | DELETE /conditional-orders/{id} (204) |
| conditionalorder | `List(ctx, ListParams{Status, Symbol, Cursor, Limit}) (*Page, error)` | GET /conditional-orders |
| conditionalorder | `Get(ctx, id string) (*Detail, error)` | GET /conditional-orders/{id} |

- 응답 타입은 openapi 스키마를 그대로 옮긴다. 수치 `decimal.Decimal`, nullable 은 포인터,
  date-time `time.Time`, date `tosstypes.Date` — v0.1.0 규약 유지.
- 커서 페이지는 `Page{Orders []Order; NextCursor *string; HasNext bool}` 로 토스 필드를 그대로 노출한다.
  반복은 호출자 책임(이터레이터 없음).
- 열거값은 `tosstypes` 가 아니라 **각 패키지**에 둔다(`order.SideBuy`, `order.StatusFilled`,
  `conditionalorder.TypeOCO`). 주문 도메인 전용이라 공용 패키지를 오염시키지 않는다.

## 주문 요청 타입

```go
// Request 는 수량 기준 주문(POST /api/v1/orders).
type Request struct {
    Symbol           string          // 필수
    Side             Side            // 필수 BUY/SELL
    OrderType        Type            // 필수 LIMIT/MARKET
    Quantity         decimal.Decimal // 필수. 소수점은 US MARKET SELL 전용
    Price            decimal.Decimal // LIMIT 필수, MARKET 은 생략
    TimeInForce      TimeInForce     // 비우면 서버 기본 DAY
    ClientOrderID    string          // 멱등성 키(10분). 설정 시에만 401 재시도
    ConfirmHighValue bool            // 1억원 이상 주문에 필수
}

// AmountRequest 는 금액 기준 주문 — US 시장가 전용(POST /api/v1/orders).
type AmountRequest struct {
    Symbol           string
    Side             Side
    OrderAmount      decimal.Decimal // 필수(USD)
    ClientOrderID    string
    ConfirmHighValue bool
}
```

- 클라이언트 사전 검증은 **요청 조립 오류만** 한다: 필수 필드 누락, `LIMIT` 인데 `Price` 0,
  `Quantity`/`OrderAmount` 0 이하, `ClientOrderID` 형식(36자·`^[A-Za-z0-9_-]+$`).
  호가단위·잔고·거래시간 등 **상태 의존 규칙은 검증하지 않는다**(서버 권위).
- `PlaceAmount` 는 `orderType=MARKET` 을 SDK 가 채운다(스키마상 유일값).

## 에러 처리

- 기존 `*toss.APIError` 와 `toss.IsCode` 를 그대로 쓴다. 새 타입 없음.
- 각 메서드 godoc 에 **대표 에러 코드**를 적는다(전수 나열 금지, 상위 5~7개):
  - `Place`: insufficient-buying-power, order-hours-closed, invalid-tick-size, price-out-of-range,
    stock-restricted, confirm-high-value-required, request-in-progress
  - `Modify`/`Cancel`: already-filled, already-canceled, already-modified, already-processing,
    order-not-found, modify-restricted/cancel-restricted, order-hours-closed
  - 공통: account-header-required, account-not-found
- 자주 쓰는 코드는 `toss` 루트에 상수로 노출한다(`toss.CodeInsufficientBuyingPower` 등 8개 내외).
  unknown 코드 허용 원칙은 유지 — 상수는 편의일 뿐 검증 수단이 아니다.

## 테스트

- **`internal/httpclient`**: POST 바디 직렬화·Content-Type, `X-Tossinvest-Account` 주입(0 이면 미주입),
  204 처리(`Out == nil` 성공 / `Out != nil` 에러), **쓰기 재시도 정책**(IdempotencyKey 없으면 401 에
  재시도 없이 에러, 있으면 1회 재시도; GET 은 항상 재시도), DELETE.
- **`asset`/`order`/`conditionalorder`**: openapi `examples` 를 fixture 로 쓰는 조회 테스트 +
  쓰기 요청 조립 테스트(바디 JSON 을 서버 스텁에서 확인). `oneOf` 는 메서드 분리로 강제되므로
  "둘 다 지정" 케이스 자체가 존재하지 않는다.
- **검증 테스트**: 필수 누락, LIMIT+Price 0, 잘못된 `ClientOrderID` 형식 — 모두 `New(nil, 1)` 로
  요청 전 실패 확인.
- **루트**: `Account(seq)` 가 3개 sub-client 를 연결하고 헤더가 실제로 실리는지 end-to-end 스텁 검증.
  `NewClientOrderID()` 는 형식·유일성.
- **integration**(`-tags integration`): 조회 9개만. 쓰기 메서드는 **호출하지 않는다**.
  ACCOUNT 1/s 이므로 호출 간 간격을 둔다.

## 릴리스 & 문서

- README 커버리지 표에 3그룹 15 ops 추가, 계좌 스코프 사용 예시, **주문 주의사항 섹션**
  (멱등성 키 권장, confirmHighValue, 소수점/금액 주문 시간 제약, SDK 는 상태 검증을 하지 않음).
- `examples/order/main.go` — 조회만 하는 안전한 예시(계좌 목록 → 보유 → 매수가능금액 → 주문목록).
  실주문 예시는 주석으로만 제시한다.
- 완료 후 main 머지 → `v0.2.0` 태그.

## 범위 밖 / 후속

- WebSocket(실시간 체결·호가·본인 주문 이벤트) — 다음 스펙. `AccessToken(ctx)` 소비.
- 주문 상태 폴링/대기 헬퍼, 429 자동 재시도, 커서 이터레이터.
- moneyflow 통합.

## 위험 / 주의

- **실주문 사고**: integration·예시·문서 어디에도 무조건 실행되는 주문 코드를 두지 않는다.
- **중복 주문**: 멱등성 키 없는 쓰기는 재시도하지 않는다는 규칙을 httpclient 테스트로 못 박는다.
- 허용 IP 미등록 시 조회 integration 도 403 — v0.1.0 과 동일한 환경 제약.
- 쓰기 2xx 예시가 없어 응답 타입은 스키마 기반이다. 실주문 검증 전까지 필드 누락 가능성이 남는다
  (사용자가 실계좌로 1회 확인하면 fixture 로 고정한다).
