# toss-go

토스증권 Open API(https://developers.tossinvest.com) 의 Go 클라이언트.

- OAuth2 Client Credentials 토큰 자동 발급·캐시(만료 60초 전 갱신, 401 토큰 오류 시 1회 재발급)
- 수치는 [`shopspring/decimal`](https://github.com/shopspring/decimal), 시각은 `time.Time`(KST 오프셋), 날짜는 `tosstypes.Date`
- 토스 에러 봉투를 `*toss.APIError`(StatusCode / Code / RequestID / Data / RetryAfter)로 매핑. `toss.IsCode(err, toss.CodeStockNotFound)`
- 실시간 웹소켓 스트림(체결·호가·본인 주문) — 선언형 full-replace 구독, 자동 재연결·재선언
- 429/5xx 재시도·스로틀링 없음(401 토큰 오류만 1회 재발급) — 429 는 `APIError.RetryAfter` 로 전달되며 속도 조절은 호출자 책임

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

모듈 경로는 `toss-go`, 패키지 이름은 `toss` 라 별칭 import 를 권장합니다.

```go
import (
    toss "github.com/kenshin579/toss-go"
    "github.com/kenshin579/toss-go/marketdata"
    "github.com/kenshin579/toss-go/stockinfo"
    "github.com/kenshin579/toss-go/stream"
    "github.com/kenshin579/toss-go/tosstypes"
)
```

```go
ctx := context.Background()

// 현재가 (국내 6자리 코드, 해외 티커 혼용 가능)
prices, err := c.MarketData.Prices(ctx, "005930", "AAPL")

// 일봉 캔들 + 페이지네이션
page, err := c.MarketData.Candles(ctx, marketdata.CandlesParams{Symbol: "005930", Interval: tosstypes.Interval1d, Count: 200})
for err == nil && page.NextBefore != nil {
    page, err = c.MarketData.Candles(ctx, marketdata.CandlesParams{Symbol: "005930", Interval: tosstypes.Interval1d, Count: 200, Before: page.NextBefore})
}
if err != nil {
    log.Fatal(err)
}

// 투자자별 매매동향 (국내 종목)
trend, err := c.StockInfo.InvestorTrading(ctx, "005930", stockinfo.TrendParams{Count: 20})

// 에러 처리
if err != nil {
    var ae *toss.APIError
    if errors.As(err, &ae) && ae.StatusCode == 429 {
        time.Sleep(ae.RetryAfter)
    }
    if toss.IsCode(err, toss.CodeStockNotFound) { /* ... */ }
}
```

실행 가능한 예시: `examples/basic`, `examples/order`.

### 계좌·주문

```go
accts, _ := c.Accounts(ctx)          // 계좌 헤더가 필요 없는 유일한 계좌 API
a := c.Account(accts[0].AccountSeq)  // 이후 모든 호출에 계좌 헤더 자동 주입

h, _ := a.Asset.Holdings(ctx, asset.HoldingsParams{})
bp, _ := a.Order.BuyingPower(ctx, tosstypes.CurrencyKRW)

res, err := a.Order.Place(ctx, order.PlaceRequest{
    Symbol: "005930", Side: order.SideBuy, OrderType: order.TypeLimit,
    Quantity: decimal.NewFromInt(1), Price: decimal.NewFromInt(70000),
    ClientOrderID: toss.NewClientOrderID(), // 멱등성 키(권장)
})
```

**주문 시 주의**

- `ClientOrderID` 를 넣으면 10분간 멱등성이 적용되고(같은 값으로 재요청 시 이전 결과 반환),
  SDK 가 401 토큰 오류에 요청을 1회 재시도한다. **키가 없으면 쓰기 요청은 재시도하지 않는다** — 중복 주문을 만들지 않기 위해서다.
- 1억원 이상 주문은 `ConfirmHighValue: true` 가 없으면 `400 confirm-high-value-required`.
- 금액 주문(`PlaceAmount`)은 **미국 주식 시장가 전용**이고, 소수점 수량은 **미국 주식 시장가 매도**에만 허용된다. 둘 다 정규장 시작~종료 1시간 전에만 접수된다.
- SDK 는 요청 조립 오류(필수 누락·형식)만 검증한다. 호가단위·잔고·거래시간 같은 상태 의존 규칙은
  서버가 판단하며 `*toss.APIError` 로 돌아온다.

### 실시간 스트림

```go
s, _ := c.Stream(ctx)
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

실행 가능한 예시: `examples/stream`.

**실시간 주의**

- 구독은 **선언형 full-replace** 다. SDK 가 현재 집합을 들고 있다가 `Subscribe`/`Unsubscribe` 때마다
  전체를 다시 선언하고, 재연결 시 자동으로 재선언한다. 거부된 항목은 집합에서 자동으로 빠진다.
- **재연결 구간의 주문 이벤트는 복구되지 않는다.** `Reconnects()` 신호를 받으면 REST 로 주문 상태를 맞춘다.
- `Orders()` 를 계속 소비하지 않으면 버퍼가 차고 SDK 가 연결을 끊는다(`ReconnectBackpressure`).
  시세 채널은 대신 오래된 이벤트를 버린다.
- 한도: 계정당 동시 연결 **2개**, 연결당 구독 **100건**(채널×종목 조합), 선언 **5회/초**.
- keepalive PING 은 SDK 가 보낸다(기본 60초). 별도로 보낼 필요가 없다.

## 커버리지

| 그룹 | 필드 | 메서드 |
| --- | --- | --- |
| Market Data | `MarketData` | `Prices` `Orderbook` `Trades` `PriceLimits` `Candles` |
| Stock Info | `StockInfo` | `Stocks` `ListStocks` `Warnings` `InvestorTrading` `ProgramTrades` `ShortSelling` `CreditTrades` `SecuritiesLending` |
| Market Info | `MarketInfo` | `ExchangeRate` `KRMarketCalendar` `USMarketCalendar` |
| Ranking | `Ranking` | `Rankings` |
| Market Indicators | `MarketIndicators` | `Prices` `Candles` `InvestorTrading` |

계좌가 필요한 API 는 `c.Account(accountSeq)` 스코프 아래에 있다(`X-Tossinvest-Account` 헤더 자동 주입).

| 그룹 | 필드 | 메서드 |
| --- | --- | --- |
| Account | (루트) | `Accounts` |
| Asset | `Asset` | `Holdings` |
| Order | `Order` | `Place` `PlaceAmount` `Modify` `Cancel` `List` `Get` `BuyingPower` `SellableQuantity` `Commissions` |
| Conditional Order | `ConditionalOrder` | `Place` `Modify` `Cancel` `List` `Get` |

### 실시간(WebSocket)

| 채널 | 접근자 | 전달 보장 |
| --- | --- | --- |
| 체결 | `Trades()` | LOSSY — 소비가 밀리면 오래된 이벤트가 버려진다 |
| 호가 | `Orderbooks()` | LOSSY |
| 본인 주문 | `Orders()` | 연결 세션 내 LOSSLESS |
| 재연결 알림 | `Reconnects()` | — |
| 구독 거부·에러 | `Errors()` | — |

`c.Stream(ctx)` 로 연결하고 `stream.Trade` / `stream.Orderbook` / `stream.PersonalOrder` 로 구독한다.

조회 21 + 계좌·주문 15 = 36 ops + 실시간 스트림(체결·호가·본인 주문).

## 문서

토스 공식 문서 원본(`openapi.json`, `asyncapi.json`, `overview.md`, `api-reference.md`)을 [`docs/api/`](docs/api/) 에 보관한다. 갱신은 `./scripts/fetch-docs.sh`.

## 개발

```bash
go build ./... && go vet ./... && go test ./... -race
go test -tags integration ./...         # 실호출 (TOSS_CLIENT_ID/SECRET + 허용 IP 필요)
./scripts/release.sh vX.Y.Z              # 태그 + GitHub Release
```

## License

MIT
