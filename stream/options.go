package stream

import (
	"net/http"
	"time"
)

// 기본값.
const (
	DefaultPingInterval   = 60 * time.Second // 서버는 클라이언트 송신이 180초 없으면 끊는다
	DefaultTradeBuffer    = 1024
	DefaultOrderBuffer    = 256
	DefaultDialTimeout    = 10 * time.Second // 재연결 시 한 번의 연결 시도에 허용하는 시간
	defaultDiagBuffer     = 16
	defaultBackoffMin     = time.Second
	defaultBackoffMax     = 30 * time.Second
	defaultCoalesceDelay  = 100 * time.Millisecond // 짧은 창 안의 연속 호출을 한 선언으로 묶는다
	defaultRateLimitRetry = time.Second            // rate-limit-exceeded 수신 뒤 재선언까지 대기
	// minDeclareInterval 은 연속된(코얼레싱되지 않은) 선언 사이에 강제하는 최소 간격이다. 문서상
	// 한도는 5회/초(=200ms 간격)이므로 210ms 로 여유를 둔다. coalesceDelay 는 짧은 창 안의 호출을
	// 하나로 묶을 뿐이라, 그 창보다 넓게 떨어진(하지만 여전히 잦은) 연속 Subscribe 호출까지는
	// 막지 못한다 — 이 상수가 그 간격을 추가로 강제한다. 유휴 상태 뒤의 첫 선언은 대기 없이 바로
	// 나간다(연속 선언만 조절된다).
	minDeclareInterval = 210 * time.Millisecond
)

type config struct {
	pingInterval   time.Duration
	tradeBuffer    int
	orderBuffer    int
	backoffMin     time.Duration
	backoffMax     time.Duration
	coalesceDelay  time.Duration
	rateLimitRetry time.Duration
	dialTimeout    time.Duration
	autoReconnect  bool
	baseURL        string       // 테스트용 ws:// 오버라이드
	httpClient     *http.Client // 핸드셰이크에 쓸 클라이언트(nil 이면 라이브러리 기본값)
	// afterDeclareWrite 는 선언을 소켓에 쓴 직후 호출되는 테스트 전용 훅이다(평시 nil).
	// "대기 배치를 쓰기보다 먼저 등록한다"는 순서를 확률이 아니라 결정적으로 검증하는 데 쓴다.
	afterDeclareWrite func()
}

func defaultConfig() config {
	return config{
		pingInterval:   DefaultPingInterval,
		tradeBuffer:    DefaultTradeBuffer,
		orderBuffer:    DefaultOrderBuffer,
		backoffMin:     defaultBackoffMin,
		backoffMax:     defaultBackoffMax,
		coalesceDelay:  defaultCoalesceDelay,
		rateLimitRetry: defaultRateLimitRetry,
		dialTimeout:    DefaultDialTimeout,
		autoReconnect:  true,
	}
}

// Option 은 Stream 생성 옵션.
type Option func(*config)

// WithPingInterval 은 keepalive PING 주기를 바꾼다(기본 60초).
// 서버는 클라이언트로부터의 수신이 180초 없으면 연결을 끊으므로 그보다 짧아야 한다.
// 1 미만이면(0 이하) 기본값으로 되돌린다.
func WithPingInterval(d time.Duration) Option { return func(c *config) { c.pingInterval = d } }

// WithTradeBuffer 는 시세 채널(Trades·Orderbooks) 버퍼 크기를 바꾼다(기본 1024).
// 가득 차면 가장 오래된 이벤트를 버린다 — 시세는 LOSSY 규약이다. 1 미만이면 기본값으로 되돌린다 —
// 버퍼가 0(또는 음수)이면 읽기 루프가 멈추거나 패닉할 수 있기 때문이다.
func WithTradeBuffer(n int) Option { return func(c *config) { c.tradeBuffer = n } }

// WithOrderBuffer 는 주문 채널 버퍼 크기를 바꾼다(기본 256).
// 주문 이벤트는 버리지 않는다 — 가득 차면 연결을 끊고 재연결하며 Reconnects() 로 알린다.
// 1 미만이면 기본값으로 되돌린다 — 버퍼가 0(또는 음수)이면 모든 주문 이벤트가 즉시 백프레셔로
// 판정돼 무한 재연결에 빠지기 때문이다.
func WithOrderBuffer(n int) Option { return func(c *config) { c.orderBuffer = n } }

// WithBackoff 는 재연결 지수 백오프의 시작·상한을 바꾼다(기본 1초·30초).
// min 이 0 이하면 기본값으로, max 가 min 보다 작으면 min 으로 되돌린다.
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

// WithRateLimitRetryDelay 는 rate-limit-exceeded 를 받은 뒤 재선언까지의 대기 시간을 바꾼다(기본 1초).
// 1 미만이면(0 이하) 기본값으로 되돌린다.
func WithRateLimitRetryDelay(d time.Duration) Option {
	return func(c *config) { c.rateLimitRetry = d }
}

// WithDialTimeout 은 재연결 다이얼 타임아웃을 바꾼다(기본 10초). 최초 연결은 New 에 넘긴 ctx 를
// 따른다. 1 미만이면(0 이하) 기본값으로 되돌린다.
func WithDialTimeout(d time.Duration) Option { return func(c *config) { c.dialTimeout = d } }

// WithHTTPClient 는 핸드셰이크에 쓸 *http.Client 를 지정한다(프록시·TLS·타임아웃 제어용).
func WithHTTPClient(hc *http.Client) Option { return func(c *config) { c.httpClient = hc } }
