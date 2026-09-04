# toss-go SDK 기반 + 조회 API 설계 (v0.1.0)

- 작성일: 2026-09-04
- 상태: 확정 (브레인스토밍 완료)
- 레포: `github.com/kenshin579/toss-go` (워크스페이스 `toss-go/`, branch `feature/sdk-foundation`)
- 선행: 문서 카탈로그 PR #1 (main merged b549071) — `docs/api/openapi.json` 이 엔드포인트·스키마 정본
- 토픽: 토스증권 Open API 의 Go 클라이언트 — 확장 가능한 기반 + 조회(시세·종목·시장정보·랭킹·지표) 21 ops

## 배경 / 목적

moneyflow 는 국내 시세·종목 메타·투자자/프로그램/공매도/신용/대차 동향을 KIS(한국투자증권) API 로
가져온다. 토스증권 Open API 를 **보완/대체 데이터 소스**로 쓰기 위해, moneyflow 가 직접 호출하지
않고 fmp-go / ecos-go / korea-investment-stock 과 동급의 **독립 Go 라이브러리**로 만든다.

1차 용도는 moneyflow 데이터 소스(조회)이므로 이번 스펙은 **기반 + 조회 21 ops** 를 다룬다.
계좌·주문·조건주문(15 ops)과 WebSocket 은 별도 스펙으로 뒤따른다.

## 사전 조사 결과 (확정 사실, 2026-09-04 실호출로 검증)

- 자격 증명은 `~/.zshrc` 의 `TOSS_CLIENT_ID` / `TOSS_CLIENT_SECRET`. 허용 IP 등록 필요
  (미등록 IP 는 토큰 발급이 403 `{"error":"access_denied","error_description":"IP address not allowed"}`).
- 토큰: `POST /oauth2/token` (form: grant_type=client_credentials, client_id, client_secret) →
  `{"access_token","token_type":"Bearer","expires_in":86399}`. **토큰 엔드포인트의 에러는 공통
  봉투가 아니라 OAuth2 표준 형식**(`{"error","error_description"}`).
- API 성공 응답은 `{"result": ...}` 봉투. 에러는 `{"error":{"requestId","code","message","data"}}`.
  에러 코드는 flat string(약 40종, `overview.md` 표)이며 **unknown code 허용 필수**.
- 수치 필드는 **전부 문자열**(`"lastPrice":"248000"`, `"rate":"1359.63"`, `"changeRate":"0.2986"`).
  스키마상 `string/decimal` 84개.
- 시각은 ISO 8601 + 오프셋(`2026-09-03T19:59:59.000+09:00`), 날짜는 `2026-09-03`. null 가능 필드
  존재(`delistDate`, `leverageFactor`, `koreanMarketDetail`, 지표 `timestamp`).
- **없는 심볼은 404 가 아니라 `{"result": []}`**.
- Rate limit 은 그룹별 TPS(MARKET_DATA 15/s, MARKET_DATA_CHART 20/s, STOCK 5/s,
  STOCK_TRADING_TREND 10/s, MARKET_INFO 3/s, RANKING 5/s, MARKET_INDICATOR* 5~10/s, AUTH 5/s,
  ACCOUNT 1/s). 응답 헤더 `X-RateLimit-Limit/Remaining/Reset`, 429 에 `Retry-After`.
- 응답은 gzip 압축(Go `http.Client` 가 자동 해제).
- 페이지네이션 3종: `nextBefore`(캔들), `nextUntil`(매매동향 5종·지표 투자자), `cursor/hasNext`(주문,
  이번 범위 밖).
- 참고 코드: fmp-go(`Client{http; Company *company.Client ...}` + `internal/httpclient` + 그룹별
  패키지), korea-investment-stock Go SDK(`shopspring/decimal`, `kistypes` 공용 패키지).

## 결정 사항 (브레인스토밍)

1. **1차 용도 = moneyflow 데이터 소스.** 조회 21 ops 전부를 v0.1.0 에 포함. 주문·WS 는 후속.
2. **수치 타입 = `github.com/shopspring/decimal`.** moneyflow 백엔드·KIS SDK 와 동일 타입이라
   바로 호환되고, 토스가 문자열로 주는 의도(정밀도)를 보존. null 가능은 `*decimal.Decimal`.
3. **토큰 = 자동 발급·메모리 캐시.** `NewClient(clientID, clientSecret)`. 파일 캐시 없음(1일
   만료·AUTH 5/s 라 불필요). 후속 WS 가 쓸 수 있게 `Client.AccessToken(ctx)` 노출.
4. **Rate limit = SDK 가 조절하지 않는다.** 429 는 `APIError`(RetryAfter 포함)로 반환, 재시도 없음.
   속도 조절은 호출자(moneyflow 배치) 책임.
5. **패키지 = OpenAPI tag 별.** `marketdata`, `stockinfo`, `marketinfo`, `ranking`, `indicators`.
   공용 타입은 `tosstypes` 소형 패키지(순환 import 회피, KIS 의 `kistypes` 방식).
6. 수작업 구현(codegen 안 함) — 21 ops 규모이고 fmp-go/ecos-go 와 관례 통일. 응답 타입은
   2026-09-04 캡처한 실응답 fixture 로 확정.

## 아키텍처

```
toss-go/
├── go.mod                  # module github.com/kenshin579/toss-go, go 1.25, dep: shopspring/decimal
├── client.go               # toss.Client — 단일 진입점, 그룹 서브클라이언트 필드
├── config.go               # Option: WithBaseURL / WithTimeout(기본 30s) / WithHTTPClient
├── from_env.go             # NewClientFromEnv() — TOSS_CLIENT_ID, TOSS_CLIENT_SECRET
├── errors.go               # APIError, AuthError 재수출 + IsCode(err, code) 헬퍼
├── internal/
│   ├── auth/               # 토큰 발급 + 메모리 캐시 (mutex, 만료 60초 전 갱신)
│   ├── httpclient/         # Bearer 주입, {result} 봉투 해제, {error}→APIError, 401 토큰 오류 1회 재발급·재시도
│   ├── strutil/            # 에러 메시지 절단 헬퍼(auth·httpclient 공용)
│   ├── fetch/              # One/List 제네릭 조회 헬퍼
│   └── params/             # 쿼리 조립·심볼 검증(Symbol/Symbols, 최대 200)
├── tosstypes/              # Date, Currency, Market, Interval, SecurityType, ... (문자열 enum + 상수)
├── marketdata/             # Prices / Orderbook / Trades / PriceLimits / Candles
├── stockinfo/              # Stocks / ListStocks / Warnings / 매매동향 5종
├── marketinfo/             # ExchangeRate / KRMarketCalendar / USMarketCalendar
├── ranking/                # Rankings
├── indicators/             # Prices / Candles / InvestorTrading
├── examples/basic/         # 시세·캔들 조회 예시
├── integration_test.go     # build tag integration — 자격 증명 있을 때만, 읽기 전용 ops
├── scripts/release.sh      # fmp-go 와 동일 절차
└── README.md               # 설치·사용·커버리지 표
```

### `toss.Client`

```go
type Client struct {
    http *httpclient.Client

    MarketData       *marketdata.Client
    StockInfo        *stockinfo.Client
    MarketInfo       *marketinfo.Client
    Ranking          *ranking.Client
    MarketIndicators *indicators.Client
}

func NewClient(clientID, clientSecret string, opts ...Option) (*Client, error)
func NewClientFromEnv(opts ...Option) (*Client, error)          // TOSS_CLIENT_ID / TOSS_CLIENT_SECRET
func (c *Client) AccessToken(ctx context.Context) (string, error) // 유효 토큰(필요 시 발급/갱신)
```

- `NewClient` 는 clientID/clientSecret 빈 값이면 에러. 생성 시 네트워크 호출 없음(토큰은 lazy).
- 각 그룹 패키지는 `New(hc *httpclient.Client) *Client` 로 만들고 `toss.Client` 가 필드로 보유.

### `internal/auth`

- `TokenSource` — `Token(ctx) (string, error)`: 캐시된 토큰이 있고 `expiresAt - 60s` 이전이면
  그대로 반환, 아니면 발급. 발급은 `sync.Mutex` 로 단일화(동시 호출이 몰려도 1회만 발급).
- `Invalidate(stale)` — httpclient 가 401 토큰 오류를 받았을 때 그 요청에 쓴 토큰을 넘겨 호출한다. 캐시된 토큰이 stale 과 같을 때만 비운다(토스는 client 당 유효 토큰 1개 — 재발급이 이전 토큰을 무효화하므로, 다른 goroutine 이 막 받은 새 토큰을 지우지 않기 위함).
- 발급 요청: `POST {baseURL}/oauth2/token`, `application/x-www-form-urlencoded`.
  응답 `expires_in` 으로 `expiresAt` 계산.
- 발급 실패(비-200)는 `AuthError{StatusCode int, Code string, Description string}` (OAuth2
  형식 `error`/`error_description` 매핑; 바디가 그 형식이 아니면 Code 빈 값 + 바디 앞부분을
  Description 에).
- 발급 응답의 `expires_in` 이 0 이하면 에러(재발급 루프 방지).

### `internal/httpclient`

```go
type Client struct { baseURL string; http *http.Client; tokens *auth.TokenSource }
func (c *Client) Get(ctx context.Context, path string, query url.Values, out any) error
```

- 요청마다 `Authorization: Bearer {token}` 주입(`tokens.Token(ctx)`), `Accept: application/json`.
- 2xx: 바디 `{"result": <T>}` 에서 `result` 만 `out` 으로 디코딩(`json.RawMessage` 경유).
- 4xx/5xx: 바디 `{"error":{...}}` → `APIError`. 바디가 봉투 형식이 아니면(엣지 차단 등)
  `APIError{StatusCode, Code:"", Message: 바디 앞 200자}`.
- 401 이고 `Code ∈ {expired-token, invalid-token}` 이면 사용한 토큰을 `tokens.Invalidate` 로 제거하고, 첫 번째 실패인 경우에만 1회 재발급·재시도한다. 재시도도 실패하면 그 에러 반환.
- 2xx 인데 result 가 없거나 null 이면 에러(토스는 2xx 에 항상 result 를 채움).
- 429 는 `Retry-After` 헤더(초)를 `APIError.RetryAfter time.Duration` 에 담는다.
- 재시도(429/5xx)·스로틀링·캐싱 없음. 네트워크/디코딩 오류는 `%w` 래핑.
- 기본 baseURL `https://openapi.tossinvest.com`, 타임아웃 30s.

### `errors.go`

```go
type APIError struct {
    StatusCode int
    RequestID  string
    Code       string          // unknown 허용
    Message    string
    Data       map[string]any  // 해결 힌트(에러 코드별 서브셋), 없으면 nil
    RetryAfter time.Duration   // 429 일 때만
}
func (e *APIError) Error() string   // "toss: 404 stock-not-found: 요청한 종목을 ... (requestId=...)"

type AuthError struct { StatusCode int; Code, Description string }

func IsCode(err error, code string) bool   // errors.As(APIError) && Code == code
```

- `ErrNotFound` sentinel 은 두지 않는다. 토스는 없는 심볼에 `[]` 를 주므로 목록 메서드는 빈
  슬라이스를 반환하고, 404 `stock-not-found` 등은 `APIError` 로 그대로 전달한다.

### `tosstypes`

- `Date` — `YYYY-MM-DD` 문자열 래퍼(`type Date string`), `Time() (time.Time, error)`,
  `UnmarshalJSON` 은 형식 검증 없이 문자열 보존(null → 빈 문자열, 포인터 필드는 nil).
- 문자열 enum: `Currency`(KRW/USD), `Market`(KOSPI/KOSDAQ/NASDAQ/NYSE/AMEX …),
  `MarketCountry`(KR/US), `Interval`(`1m`, `1d`), `SecurityType`, `StockStatus`,
  `RankingType`, `RankingDuration`, `RateChangeType` 등 — `type X string` + 상수. **unknown 값도
  그대로 보존**(검증으로 거부하지 않음). 정확한 상수 목록은 `openapi.json` enum 에서 구현 시 확정.
- date-time 은 표준 `time.Time`(RFC3339 `.000+09:00` 파싱 가능), null 가능은 `*time.Time`.
- 수치는 `decimal.Decimal`, null 가능은 `*decimal.Decimal`.
- 심볼은 요청 전에 `^[A-Za-z0-9.\-]+$` 로 검증하고 `symbols=` 는 최대 200개(openapi 규칙) — 서버 400 으로 rate limit 을 소모하지 않기 위함.

## 엔드포인트 매핑 (21 ops)

파라미터 3개 이상은 `XxxParams` 구조체, 그 외는 위치 인자. 선택 파라미터는 zero-value 면 생략.

| 패키지 | 메서드 | HTTP |
|---|---|---|
| marketdata | `Prices(ctx, symbols ...string) ([]Price, error)` | GET /api/v1/prices?symbols= |
| marketdata | `Orderbook(ctx, symbol) (*Orderbook, error)` | GET /api/v1/orderbook |
| marketdata | `Trades(ctx, symbol, count int) ([]Trade, error)` | GET /api/v1/trades |
| marketdata | `PriceLimits(ctx, symbol) (*PriceLimits, error)` | GET /api/v1/price-limits |
| marketdata | `Candles(ctx, CandlesParams{Symbol, Interval, Count, Before *time.Time, Adjusted *bool}) (*CandlePage, error)` | GET /api/v1/candles |
| stockinfo | `Stocks(ctx, symbols ...string) ([]Stock, error)` | GET /api/v1/stocks?symbols= |
| stockinfo | `ListStocks(ctx, ListStocksParams{Market, Status, SecurityType, CommonShare *bool}) ([]ListedStock, error)` | GET /api/v1/stocks/all |
| stockinfo | `Warnings(ctx, symbol) ([]Warning, error)` | GET /api/v1/stocks/{symbol}/warnings |
| stockinfo | `InvestorTrading(ctx, symbol, TrendParams{Count, Until Date}) (*InvestorTradingPage, error)` | GET /api/v1/stocks/{symbol}/investor-trading |
| stockinfo | `ProgramTrades(ctx, symbol, TrendParams) (*ProgramTradesPage, error)` | GET /api/v1/stocks/{symbol}/program-trades |
| stockinfo | `ShortSelling(ctx, symbol, TrendParams) (*ShortSellingPage, error)` | GET /api/v1/stocks/{symbol}/short-selling |
| stockinfo | `CreditTrades(ctx, symbol, TrendParams) (*CreditTradesPage, error)` | GET /api/v1/stocks/{symbol}/credit-trades |
| stockinfo | `SecuritiesLending(ctx, symbol, TrendParams) (*SecuritiesLendingPage, error)` | GET /api/v1/stocks/{symbol}/securities-lending |
| marketinfo | `ExchangeRate(ctx, base, quote Currency, at *time.Time) (*ExchangeRate, error)` | GET /api/v1/exchange-rate |
| marketinfo | `KRMarketCalendar(ctx, date Date) (*KRMarketCalendar, error)` | GET /api/v1/market-calendar/KR |
| marketinfo | `USMarketCalendar(ctx, date Date) (*USMarketCalendar, error)` | GET /api/v1/market-calendar/US |
| ranking | `Rankings(ctx, RankingsParams{Type, MarketCountry, Duration, ExcludeInvestmentCaution *bool, Count}) (*Rankings, error)` | GET /api/v1/rankings |
| indicators | `Prices(ctx, symbols ...string) ([]IndicatorPrice, error)` | GET /api/v1/market-indicators/prices |
| indicators | `Candles(ctx, symbol, IndicatorCandlesParams{Interval, Count, Before *time.Time}) (*IndicatorCandlePage, error)` | GET /api/v1/market-indicators/{symbol}/candles |
| indicators | `InvestorTrading(ctx, symbol, IndicatorInvestorTradingParams{Interval, Count, Until Date}) (*IndicatorInvestorTradingPage, error)` | GET /api/v1/market-indicators/{symbol}/investor-trading |

- `*Page` 응답은 토스 필드를 그대로 노출: `Candles []Candle; NextBefore *time.Time`,
  `Records []T; NextUntil *Date`. 반복 조회는 호출자가 `NextBefore/NextUntil` 을 다음 요청에 넣는다
  (v1 에 이터레이터 없음). `before` 쿼리의 `+` 는 `url.Values` 인코딩이 `%2B` 로 처리.
- 응답 구조체 필드·타입은 `openapi.json` 스키마 + 캡처 fixture 로 구현 시 확정. 필드명은 토스
  camelCase 를 Go 관례로 변환(`lastPrice` → `LastPrice`), json 태그는 원본 유지.
- `Stock.KoreanMarketDetail` 등 중첩 객체는 null 가능 → 포인터.

## 데이터 흐름 (예: Candles)

1. `client.MarketData.Candles(ctx, CandlesParams{Symbol:"005930", Interval:"1d", Count:2})`
2. `httpclient.Get(ctx, "/api/v1/candles", {symbol,interval,count}, &page)`
3. `tokens.Token(ctx)` → 캐시 없으면 `POST /oauth2/token` 발급 → Bearer 헤더
4. 200 → `{"result":{"candles":[...],"nextBefore":"..."}}` 의 `result` 를 `CandlePage` 로 디코딩
   (가격·거래량 `decimal.Decimal`, timestamp `time.Time`, nextBefore `*time.Time`)
5. 401 `expired-token` → Invalidate → 재발급 → 1회 재시도. 429 → `APIError{RetryAfter}` 반환.

## 테스트

- **`internal/auth`**: `httptest.Server` 로 발급 성공/실패(OAuth2 에러 형식 → `AuthError`),
  만료 전 캐시 재사용, 만료 임박 갱신, 동시 100 goroutine 호출 시 발급 1회, `Invalidate` 후 재발급.
- **`internal/httpclient`**: Bearer 헤더·Accept 검증, 봉투 해제, `{error}` → `APIError`(코드·requestId·
  data), 비봉투 바디, 429 `Retry-After`, 401 `expired-token` 1회 재시도(2회째 401 이면 에러),
  타임아웃/컨텍스트 취소.
- **그룹 패키지**: 2026-09-04 캡처한 실응답 fixture(`<pkg>/testdata/*.json`)로 경로·쿼리 검증 +
  디코딩 정확성(decimal 값, null → nil 포인터, Date, 중첩 breakdown). `Prices` 의 `[]` → 빈 슬라이스.
- **`integration_test.go`** (`-tags integration`): `TOSS_CLIENT_ID/SECRET` 있을 때만. 읽기 전용 ops
  (prices/candles/stocks/exchange-rate/market-calendar) 실호출로 계약 검증. 기본 `go test ./...` 제외.
- 캡처 fixture 는 계좌 응답을 제외한 시장 데이터만 커밋한다(계좌번호 등 개인정보 제외).

## 릴리스 & 문서

- `scripts/release.sh` — fmp-go 것을 복사(main/clean 검증 → build/vet/test → 모듈 zip 검증 →
  태그 → `gh release create --generate-notes`).
- `README.md` — 설치(`go get github.com/kenshin579/toss-go@v0.1.0`), 인증(env 2개 + 허용 IP 등록
  안내), 사용 예시(`NewClientFromEnv` → `MarketData.Prices`), 커버리지 표(5 그룹 21 ops, 주문·WS
  예정), 에러 처리 예시(`toss.IsCode`).
- `examples/basic/main.go` — 시세·캔들 조회.
- 워크스페이스 `CLAUDE.md` 의 toss-go 항목을 "라이브러리 v0.1.0" 단계로 갱신(별도 커밋, 워크스페이스
  루트는 git 저장소가 아니므로 파일만 수정).
- 완료 후 main 머지 → `v0.1.0` 태그.

## 범위 밖 / 후속

- 계좌·자산·주문·주문정보·조건주문 15 ops (`X-Tossinvest-Account` 헤더 주입 포함) — 다음 스펙.
- WebSocket(선언형 구독, PING, 재연결·재선언) — 그 다음 스펙. `AccessToken(ctx)` 를 소비.
- 429 자동 재시도, 그룹별 스로틀링, 파일 토큰 캐시, 페이지 이터레이터, 응답 헤더(`X-RateLimit-*`) 노출.
- moneyflow 통합(소스 추상화·배치 워밍 연결) — 별도 스펙.

## 위험 / 주의

- **허용 IP**: 실호출·integration 테스트는 등록된 IP 에서만 가능. 맥미니 등 다른 환경은 IP 추가 필요.
- 토스 스키마 버전(1.2.14)이 오르면 `scripts/fetch-docs.sh` 로 갱신 후 diff 로 영향 확인.
- 에러 코드·enum 은 예고 없이 늘 수 있으므로 unknown 값을 거부하지 않는다.
- `X-RateLimit-Remaining` 이 0 인 그룹(ACCOUNT 1/s)을 연속 호출하면 429 — 이번 범위엔 없지만
  integration 테스트에서 호출 간 간격을 둔다.
