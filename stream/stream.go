package stream

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// ackResult 는 하나의 선언에 대한 서버 응답(또는 그 선언을 보내거나 기다리는 도중 실패한 사유)이다.
type ackResult struct {
	rejected []rejectedItem
	err      error // 비어 있지 않으면 대기자에게 그대로 돌려준다(*DeclareError, 전송 실패, 연결 종료 등)
}

// declareBatch 는 전송된 특정 선언 id 에 매달린 대기자들이다(takeBatch 참고 — id 없는 ack 는
// 어떤 배치에도 매달지 않는다).
type declareBatch struct {
	id      string
	waiters []chan ackResult
}

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

	mu             sync.Mutex
	conn           *websocket.Conn // 현재 연결. 쓰기 전에 잠근다
	lastDeclareID  string          // 마지막으로 전송에 성공한 선언의 id. 낡은 ack 를 걸러내는 데 쓴다
	lastDeclareAt  time.Time       // 마지막으로 전송에 성공한 선언 시각. minDeclareInterval 강제에 쓴다
	pendingWaiters []chan ackResult
	waiterBatches  []declareBatch
}

// New 는 스트림을 만들고 연결한다. 보통은 toss.Client.Stream 을 쓴다.
//
// ctx 는 최초 연결에만 쓰인다. 이후 스트림은 ctx 취소와 무관하게 살아 있으며 Close 로만 끝난다.
func New(ctx context.Context, token TokenFunc, opts ...Option) (*Stream, error) {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}
	// 버퍼가 1 미만이면 pushLossy 가 무한 스핀하거나(시세) 모든 이벤트가 백프레셔로 판정된다(주문).
	// 잘못된 값은 조용히 기본값으로 되돌린다 — 스트림이 멈추는 것보다 낫다.
	if cfg.tradeBuffer < 1 {
		cfg.tradeBuffer = DefaultTradeBuffer
	}
	if cfg.orderBuffer < 1 {
		cfg.orderBuffer = DefaultOrderBuffer
	}
	if cfg.backoffMin <= 0 {
		cfg.backoffMin = defaultBackoffMin
	}
	if cfg.backoffMax < cfg.backoffMin {
		cfg.backoffMax = cfg.backoffMin
	}
	if cfg.pingInterval <= 0 {
		cfg.pingInterval = DefaultPingInterval
	}
	if cfg.rateLimitRetry <= 0 {
		cfg.rateLimitRetry = defaultRateLimitRetry
	}
	if cfg.dialTimeout <= 0 {
		cfg.dialTimeout = DefaultDialTimeout
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
	conn, err := dial(ctx, url, token, cfg.httpClient)
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
//
// 서버의 구독 ack 를 기다렸다가, 이 호출이 추가한 항목이 거부됐으면 *RejectedError 를,
// 선언 자체가 실패했으면 *DeclareError 를 돌려준다. ctx 가 먼저 끝나면 ctx.Err() 를 돌려주는데,
// 이때 선언은 이미 전송됐을 수 있다. 연속 호출은 최대 coalesceDelay(기본 100ms) 동안 하나의
// 선언으로 합쳐지므로 그만큼 지연될 수 있다.
//
// 이 호출이 넣은 항목 중 여럿이 거부돼도 반환값은 그중 하나뿐이다 — 나머지도 포함해 거부된 항목
// 전체는 Errors() 로 통지된다. *DeclareError 로 실패해도(선언 자체가 실패한 것이지 항목이 거부된
// 게 아니므로) 추가한 항목은 구독 집합에 남아 다음 선언에 다시 포함된다 — 호출자가 별도로
// 재시도할 필요가 없다.
//
// rate-limit-exceeded 는 SDK 가 자동으로 재선언하므로 에러로 반환하지 않는다(Errors() 로는
// 통지된다) — 곧 성공할 구독을 실패로 알고 호출자가 재시도하면 선언 빈도 한도를 더 악화시키기
// 때문이다.
func (s *Stream) Subscribe(ctx context.Context, subs ...Subscription) error {
	if s.closed.Load() {
		return errStreamClosed
	}
	if err := s.subs.add(subs...); err != nil {
		return err
	}
	return s.declareAndWait(ctx, subKeys(subs))
}

// Unsubscribe 는 구독을 빼고 전체 집합을 다시 선언한다.
//
// 서버의 구독 ack 를 기다린다. 이 호출은 항목을 추가하지 않으므로 거부와는 무관하며(항상 nil 이거나
// 선언 자체가 실패했을 때만 *DeclareError), ctx 가 먼저 끝나면 ctx.Err() 를 돌려주는데 이때 선언은
// 이미 전송됐을 수 있다. 단, 이 호출로 구독 집합이 완전히 비면 Declare(ctx) 의 빈 선언과 같은
// 경로다 — 프로토콜상 `[]` 로만 보내고 서버도 개별 ack 를 주지 않는 전체 해제라, 서버 확인을
// 기다리지 않고 곧바로 nil 을 돌려준다. rate-limit-exceeded 는 SDK 가 자동으로 재선언하므로 에러로
// 반환하지 않는다(Errors() 로는 통지된다).
func (s *Stream) Unsubscribe(ctx context.Context, subs ...Subscription) error {
	if s.closed.Load() {
		return errStreamClosed
	}
	if err := s.subs.remove(subs...); err != nil {
		return err
	}
	return s.declareAndWait(ctx, nil)
}

// Declare 는 구독 집합을 통째로 바꾼다. 인자가 없으면 전체 구독을 해제한다.
//
// 서버의 구독 ack 를 기다렸다가, 이 호출이 넣은 항목이 거부됐으면 *RejectedError 를, 선언 자체가
// 실패했으면 *DeclareError 를 돌려준다(단, 빈 Declare 는 프로토콜상 id 없이 `[]` 로만 보내고 서버도
// 개별 ack 를 주지 않는 전체 해제라 즉시 nil 을 돌려준다 — 거부될 항목이 있을 수 없다). ctx 가 먼저
// 끝나면 ctx.Err() 를 돌려주는데, 이때 선언은 이미 전송됐을 수 있다. rate-limit-exceeded 는 SDK 가
// 자동으로 재선언하므로 에러로 반환하지 않는다(Errors() 로는 통지된다).
func (s *Stream) Declare(ctx context.Context, subs ...Subscription) error {
	if s.closed.Load() {
		return errStreamClosed
	}
	if err := s.subs.replace(subs...); err != nil {
		return err
	}
	return s.declareAndWait(ctx, subKeys(subs))
}

// declareAndWait 는 선언 전송을 요청하고, 그 선언의 ack 를 기다려 keys 중 하나가 거부됐으면
// *RejectedError 를, 선언 자체가 실패했으면 그 에러를 돌려준다.
func (s *Stream) declareAndWait(ctx context.Context, keys []string) error {
	ch := make(chan ackResult, 1)
	s.registerWaiter(ch)

	select {
	case res := <-ch:
		if res.err != nil {
			return res.err
		}
		for _, r := range res.rejected {
			if containsKey(keys, r.Target) {
				item := r
				return &RejectedError{Target: item.Target, Code: item.Code, Message: item.Message}
			}
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		// Close() 가 s.closed 를 세우기 직전에 이 호출이 위의 검사를 통과한 경우를 위한
		// 안전망 — 이게 없으면 영원히 대기한다(그 경우 pendingWaiters 에 등록은 됐지만
		// run() 이 이미 완전히 끝나 다시는 failAllWaiters 가 불리지 않는다).
		return errStreamClosed
	}
}

// subKeys 는 Subscription 들이 나타내는 full key(type:code) 목록이다.
func subKeys(subs []Subscription) []string {
	var keys []string
	for _, sub := range subs {
		for _, c := range sub.Codes {
			keys = append(keys, sub.Type+":"+c)
		}
	}
	return keys
}

func containsKey(keys []string, target string) bool {
	for _, k := range keys {
		if k == target {
			return true
		}
	}
	return false
}

// Close 는 재연결을 멈추고 연결을 닫으며 모든 채널을 닫는다. 여러 번 호출해도 안전하다.
func (s *Stream) Close() error {
	if s.closed.Swap(true) {
		return nil
	}
	s.cancel()
	s.mu.Lock()
	c := s.conn
	s.conn = nil
	s.mu.Unlock()
	closeConn(c)
	<-s.done
	return nil
}

func (s *Stream) requestDeclare() {
	select {
	case s.declareCh <- struct{}{}:
	default: // 이미 예약돼 있으면 합친다(선언 5회/초 한도 대응)
	}
}

// registerWaiter 는 대기자를 등록하고 선언을 요청하는 것을 하나의 임계구역으로 묶는다 —
// declareLoop 의 snapshot+drain 도 같은 s.mu 로 원자적이라(아래 참고), 이 둘이 상호 배타적이 되어
// "등록은 이번 배치에 잡혔는데 신호는 다음 drain 에 먹혀 requestDeclare 없이 방치되는" 창이 없다.
func (s *Stream) registerWaiter(ch chan ackResult) {
	s.mu.Lock()
	s.pendingWaiters = append(s.pendingWaiters, ch)
	select {
	case s.declareCh <- struct{}{}:
	default:
	}
	s.mu.Unlock()
}

// run 은 연결 수명 주기를 관리한다: 읽기·PING·선언 루프를 돌리고, 끊기면 재연결한다.
func (s *Stream) run(ctx context.Context, conn *websocket.Conn) {
	defer close(s.done)
	defer s.closeChannels()

	attempt := 0
	cause := ReconnectReadError
	for {
		connStart := time.Now() // 최초 연결·재연결 모두 여기서 이 연결의 "시작"으로 친다
		connCtx, connCancel := context.WithCancel(ctx)
		causeCh := make(chan ReconnectCause, 1)
		var wg sync.WaitGroup
		wg.Add(3)
		go func() { defer wg.Done(); s.readLoop(connCtx, conn, causeCh); connCancel() }()
		go func() { defer wg.Done(); _ = pingLoop(connCtx, conn, s.cfg.pingInterval); connCancel() }()
		go func() { defer wg.Done(); s.declareLoop(connCtx); connCancel() }()
		wg.Wait()

		// 곧바로 다시 끊긴 연결은 성공으로 치지 않는다 — attempt 를 유지해 백오프 곡선을 이어간다.
		// 이게 없으면 백프레셔처럼 클라이언트가 스스로 끊는 경우 즉시 재시도가 폭주한다: 첫 시도가
		// 즉시(0 대기)인 데다 재연결 성공마다 attempt 를 0 으로 되돌리면, 짧게 살다 죽는 연결이
		// 반복될 때마다 매번 "새로 시작"으로 착각해 초당 수천 회 재다이얼하게 된다(실측: 기본
		// backoff 로 2초에 8949회).
		healthy := time.Since(connStart) >= s.cfg.backoffMin

		// 이 연결에 걸려 있던 ack 대기자는 다시는 응답을 받지 못한다 — 여기서 풀어준다
		// (영원히 막히지 않게). 재연결에 성공하면 저장된 집합이 다시 선언되지만, 그 새 선언에
		// 대한 새 대기는 재연결 이후 호출에서만 만들어진다.
		s.failAllWaiters(fmt.Errorf("toss: connection closed while waiting for declare ack"))

		select {
		case c := <-causeCh:
			cause = c
		default:
		}

		// 재연결 전에 반드시 기존 연결을 닫는다(계정당 2개 한도). 뮤텍스를 잡은 채로 네트워크
		// 종료 핸드셰이크를 하지 않는다 — CloseNow 조차 상대가 응답하지 않는 소켓에서는 지연될
		// 수 있어, 그 지연이 다른 goroutine 의 s.conn 접근(예: declareLoop 의 쓰기)을 막아서는
		// 안 된다.
		s.mu.Lock()
		c := s.conn
		s.conn = nil
		s.mu.Unlock()
		closeConn(c)
		connCancel()

		if ctx.Err() != nil || !s.cfg.autoReconnect {
			if !s.cfg.autoReconnect && !s.closed.Load() {
				s.emitErr(fmt.Errorf("toss: stream disconnected (%s); auto-reconnect is disabled", cause))
			}
			return
		}

		// dial 에 성공할 때까지 재시도한다. 실패한 dial 시도는 cause 를 덮지 않는다 —
		// 이 연결이 왜 끊겼는지(backpressure 등)를 재연결 성공 때까지 그대로 들고 있어야 한다.
		// (readLoop/pingLoop 를 죽은 conn 으로 다시 돌리면 즉시 read-error 가 나서 cause 를
		// 덮어써 버리므로, dial 재시도는 별도 루프로 두고 연결 루프를 다시 시작하지 않는다.)
		var next *websocket.Conn
		for {
			attempt++
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff(attempt, s.cfg.backoffMin, s.cfg.backoffMax)):
			}
			dialCtx, dialCancel := context.WithTimeout(ctx, s.cfg.dialTimeout)
			c, err := dial(dialCtx, s.url, s.token, s.cfg.httpClient)
			dialCancel()
			if err != nil {
				s.emitErr(err)
				continue
			}
			next = c
			break
		}
		s.mu.Lock()
		s.conn = next
		s.mu.Unlock()
		conn = next
		s.emitReconnect(Reconnect{Attempt: attempt, Cause: cause, At: time.Now()})
		s.requestDeclare() // 저장된 집합을 다시 선언한다
		if healthy {
			attempt = 0
		}
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
			// 낡은 선언의 ack 는 구독 집합에는 반영하지 않는다 — 그 사이 사용자가 다시 넣은
			// 구독을 지울 수 있다. 다만 거부 알림은 stale 여부와 무관하게 항상 보낸다.
			s.mu.Lock()
			stale := f.id != "" && s.lastDeclareID != "" && f.id != s.lastDeclareID
			s.mu.Unlock()
			for _, r := range f.rejected {
				if !stale {
					s.subs.reject(r.Target)
				}
				s.emitErr(&RejectedError{Target: r.Target, Code: r.Code, Message: r.Message})
			}
			if chs := s.takeBatch(f.id); len(chs) > 0 {
				s.resolveWaiters(chs, ackResult{rejected: f.rejected})
			}
		case frameError:
			if f.errCode == "server-shutdown" {
				shutdown = true
				continue // 곧 연결이 끊긴다 — 에러가 아니라 재연결 사유다
			}
			declErr := &DeclareError{ID: f.id, Code: f.errCode, Message: f.errMessage}
			s.emitErr(declErr)
			if f.errCode == "rate-limit-exceeded" {
				// 선언 빈도(5회/초)에 걸린 선언은 반영되지 않았다. 대기 후 한 번 다시 선언한다
				// (토스는 Retry-After 를 주지 않는다). 대기자는 여기서 풀지 않는다 — 곧 보낼
				// 재선언이 프로토콜상 full-replace 라 이 선언을 대체하므로, declareLoop 가 그
				// 재선언을 보낼 때 지금 남아 있는 배치를 새 배치에 흡수한다(아래 참고). 여기서
				// 풀어 버리면 실제로는 잠시 후 성공할 구독을 호출자가 실패로 알고 재시도해
				// 선언 빈도 한도를 더 악화시킬 수 있다.
				s.retryDeclareAfter(ctx, s.cfg.rateLimitRetry)
				continue
			}
			// id 가 없는 에러 프레임은 특정 선언과 무관한 전역 에러일 수 있어(예: 일반
			// internal-error), 임의로 가장 오래된 대기자에게 떠넘기지 않는다 — 그 대기자의
			// 실제 ack 가 뒤이어 와도 이미 소비돼 사라지는 것을 막기 위함이다. id 가 있을
			// 때만 정확히 그 배치에 실패를 전달한다.
			if f.id != "" {
				if chs := s.takeBatch(f.id); len(chs) > 0 {
					s.resolveWaiters(chs, ackResult{err: declErr})
				}
			}
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
	default:
		// 알 수 없는 채널·시장은 버리되, 조용히 버리지는 않는다 —
		// 무손실 채널의 프레임이 흔적 없이 사라지는 것을 막는다.
		s.emitErr(fmt.Errorf("toss: unhandled message topic %q", f.topic))
	}
	return true
}

// pushLossy 는 가득 차면 가장 오래된 것을 버리고 새 이벤트를 넣는다(시세 LOSSY 규약, 재연결 알림도
// 같은 규약을 쓴다). 반복하지 않는다 — 버퍼가 0 이면 무한 스핀이 되고(cap 0 채널은 동시 수신자가
// 없는 한 항상 default 로 빠진다), 경쟁하는 소비자가 있으면 필요 이상으로 버릴 수 있기 때문이다.
func pushLossy[T any](ch chan T, ev T) {
	select {
	case ch <- ev:
	default:
		select {
		case <-ch: // 가장 오래된 것을 버린다
		default:
		}
		select {
		case ch <- ev:
		default: // 그 사이 다시 찼다면(경쟁 소비자) 이번 이벤트를 버린다 — LOSSY 규약이니 허용한다
		}
	}
}

// declareLoop 는 코얼레싱된 선언 요청을 처리하고, 그 선언을 기다리던 호출자(Subscribe 등)를
// 결과가 나오는 대로 풀어준다.
func (s *Stream) declareLoop(ctx context.Context) {
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

			// snapshot 과 drain 을 하나의 임계구역으로 묶는다 — registerWaiter 의 append+signal 도
			// 같은 s.mu 로 원자적이므로, 이 둘은 서로 완전히 앞서거나 뒤서게 된다: 요청이 이 스냅샷에
			// 잡혔다면 그 요청이 남긴 신호도 반드시 이 안에서 함께 지워지고(잔여 토큰에 의한 대기자
			// 없는 중복 선언 없음), 스냅샷 뒤에 등록됐다면 신호도 그대로 살아남아 다음 루프가 정확히
			// 그 요청을 처리한다(신호가 먼저 사라져 대기자가 다음 requestDeclare 까지 방치되는 일 없음).
			s.mu.Lock()
			waiting := s.pendingWaiters
			s.pendingWaiters = nil
			select {
			case <-s.declareCh:
			default:
			}
			s.mu.Unlock()

			body, err := s.subs.declaration(id)
			if err != nil {
				s.emitErr(err)
				s.resolveWaiters(waiting, ackResult{err: err})
				continue
			}
			s.mu.Lock()
			c := s.conn
			s.mu.Unlock()
			if c == nil {
				// 방어적 경로: declareLoop 는 항상 유효한 연결과 함께 시작되므로 실전에서는
				// 거의 발생하지 않는다. 다음 연결에서 저장된 집합이 다시 선언된다.
				s.resolveWaiters(waiting, ackResult{err: errNotConnected})
				continue
			}
			// 최소 선언 간격을 강제한다 — coalesceDelay 는 짧은 창 안의 호출만 하나로 묶어, 그보다
			// 넓게(하지만 여전히 잦게) 떨어진 연속 Subscribe 는 그대로 다 나가 문서상 5회/초 한도를
			// 넘길 수 있었다(실측: 순차 Subscribe 10회가 1.01초에 전부 나감). 이 대기는 반드시
			// 배치 등록보다 앞에 둔다 — 등록 뒤에 대기하면 그 사이 도착하는 ack 와의 순서 관계가
			// 바뀌어 "쓰기 전에 등록한다" 는 불변식이 깨진다. 유휴 상태 뒤의 첫 선언은 곧바로 나간다.
			s.mu.Lock()
			wait := minDeclareInterval - time.Since(s.lastDeclareAt)
			s.mu.Unlock()
			if wait > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(wait):
				}
			}
			if isEmptyDeclaration(body) {
				// 빈 배열은 프로토콜상 id 없이 `[]` 로만 전송되는 전체 해제 전용 형태라 서버도
				// 개별 subscriptions ack 를 보내지 않는다 — 그래서 아래(비어 있지 않은 경우)의
				// "쓰기 전에 등록" 규칙과 무관하다: 애초에 매칭할 id 가 없으니 ack 가 등록을
				// 앞지를 일도 없다. 그래도 소켓에는 반드시 써야 한다(`[]` 를 실제로 보내지 않으면
				// 서버 구독이 그대로 남는다) — 쓰기 성공을 확인한 뒤에만 성공으로 풀어준다.
				// full-replace 라 이 선언이 이전 선언들을 전부 대체한다 — 앞서 쌓인 채 아직 ack
				// 를 못 받은 배치가 있다면(예: rate-limit 로 대기 중이던 것) 그 대기자도 함께
				// 풀어준다(id 를 기록하지 않는다 — 실제로 echo 될 id 가 없다).
				s.mu.Lock()
				carried := waiting
				for _, b := range s.waiterBatches {
					carried = append(carried, b.waiters...)
				}
				s.waiterBatches = nil
				s.mu.Unlock()
				if err := c.Write(ctx, websocket.MessageText, body); err != nil {
					werr := fmt.Errorf("toss: declare: %w", err)
					s.emitErr(werr)
					s.resolveWaiters(carried, ackResult{err: werr})
					return
				}
				s.mu.Lock()
				s.lastDeclareAt = time.Now()
				s.mu.Unlock()
				s.resolveWaiters(carried, ackResult{})
				continue
			}
			// 등록을 쓰기보다 먼저 한다 — 반대 순서면 readLoop 이 동시에 도는 goroutine이라, 서버
			// ack 가 이 등록보다 먼저 도착해 처리될 수 있다. 그러면 takeBatch(id) 가 아직 없는
			// 배치를 찾지 못해 ack 는 소비된 채 버려지고, 뒤이어 등록되는 배치는 그 id 로 다시는
			// ack 를 받을 수 없어 영구 고아가 된다(호출자가 무기한 대기). full-replace 라 이 선언이
			// 이전 선언들을 전부 대체한다 — 서버가 error 프레임에는 id 를 echo 하지 않는 경우가
			// 있어(예: rate-limit-exceeded), 그런 선언에 매달린 배치는 결코 자기 id 로 된 ack 를
			// 받지 못한다 — 방치하면 그 배치의 호출자가 영원히 막히고 waiterBatches 도 연결 수명
			// 내내 계속 자란다. 이번에 새로 보낼 선언에 흡수해 이번 선언의 ack 가 대신 풀어주게 한다.
			s.mu.Lock()
			prevID := s.lastDeclareID
			s.lastDeclareID = id
			carried := waiting
			for _, b := range s.waiterBatches {
				carried = append(carried, b.waiters...)
			}
			s.waiterBatches = nil
			if len(carried) > 0 {
				s.waiterBatches = append(s.waiterBatches, declareBatch{id: id, waiters: carried})
			}
			s.mu.Unlock()

			if err := c.Write(ctx, websocket.MessageText, body); err != nil {
				// 못 보낸 선언이니 되돌린다 — id 를 남기면 다음 ack 가 stale 로 오인되고, 방금
				// 등록한 배치는 애초에 ack 를 받을 길이 없으니 여기서 직접 실패로 풀어준다.
				s.mu.Lock()
				s.lastDeclareID = prevID
				chs := s.waiterBatches
				s.waiterBatches = nil
				s.mu.Unlock()
				werr := fmt.Errorf("toss: declare: %w", err)
				s.emitErr(werr)
				for _, b := range chs {
					s.resolveWaiters(b.waiters, ackResult{err: werr})
				}
				return
			}
			s.mu.Lock()
			s.lastDeclareAt = time.Now()
			s.mu.Unlock()
			if s.cfg.afterDeclareWrite != nil {
				s.cfg.afterDeclareWrite()
			}
		}
	}
}

// isEmptyDeclaration 은 declaration() 이 만든 body 가 전체 해제(`[]`)인지 본다.
func isEmptyDeclaration(body []byte) bool {
	return string(bytes.TrimSpace(body)) == "[]"
}

// takeBatch 는 id 에 매달린 대기자를 찾아 제거하고 돌려준다. id 가 비어 있으면 아무 배치에도
// 매달지 않는다 — SDK 는 비어 있지 않은 선언에는 항상 id 를 붙이므로, id 없는 ack 가 올 수 있는
// 유일한 경우는 `[]`(전체 해제) 뿐이고 그 경로는 자기 대기자를 직접 해소한다(declareLoop 참고).
// 예전에는 "가장 오래 기다린 배치" 로 폴백했는데, id 없는 ack 의 유일한 출처가 `[]` 인 이상 그
// 폴백은 항상 틀린 배치를 집었다 — 뒤이어 보낸 선언(예: Subscribe)의 배치를 가로채 그 결과를
// "성공, 거부 없음" 으로 잘못 확정시키고, 진짜 ack 는 주인을 잃었다.
func (s *Stream) takeBatch(id string) []chan ackResult {
	if id == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := -1
	for i, b := range s.waiterBatches {
		if b.id == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}
	w := s.waiterBatches[idx].waiters
	s.waiterBatches = append(s.waiterBatches[:idx], s.waiterBatches[idx+1:]...)
	return w
}

// resolveWaiters 는 res 를 각 채널로 보낸다. 채널은 버퍼 1개짜리이고 정확히 한 번만 보내지도록
// 호출부가 보장한다(대기자 슬라이스는 등록·전달마다 통째로 옮겨지고 다시 쓰이지 않는다).
func (s *Stream) resolveWaiters(chs []chan ackResult, res ackResult) {
	for _, ch := range chs {
		ch <- res
	}
}

// failAllWaiters 는 아직 남아 있는 모든 대기자(전송 전 pendingWaiters + 전송 후 waiterBatches)를
// 주어진 에러로 풀어준다. 연결 하나의 수명이 끝날 때(재연결이든 최종 종료든) run 이 호출해,
// 다시는 오지 않을 ack 를 기다리며 영원히 막히는 일이 없게 한다.
func (s *Stream) failAllWaiters(err error) {
	s.mu.Lock()
	pending := s.pendingWaiters
	s.pendingWaiters = nil
	batches := s.waiterBatches
	s.waiterBatches = nil
	s.mu.Unlock()
	res := ackResult{err: err}
	for _, ch := range pending {
		ch <- res
	}
	for _, b := range batches {
		for _, ch := range b.waiters {
			ch <- res
		}
	}
}

// pendingBatchCountForTest 는 아직 해소되지 않은 대기 배치 수다(테스트 전용).
func (s *Stream) pendingBatchCountForTest() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.waiterBatches)
}

// retryDeclareAfter 는 d 후에 선언을 한 번 더 요청한다. 연결이 끝나면 취소된다.
//
// 이 goroutine 은 WaitGroup 에 넣지 않는다 — declareCh 는 절대 닫히지 않으므로 채널 종료 후에도
// 안전하다. 여기에 emitErr 등 닫히는 채널로의 전송을 추가하면 panic 이 난다.
func (s *Stream) retryDeclareAfter(ctx context.Context, d time.Duration) {
	go func() {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
		case <-t.C:
			s.requestDeclare()
		}
	}()
}

// emitErr 는 Errors() 채널이 가득 찼을 때 가장 오래된 에러를 버린다 — 다른 LOSSY 채널(시세,
// 재연결 알림)과 같은 규약이다. 새 에러를 버리면 Errors() 를 소비하지 않는 호출자가 최초 16건에서
// 멈춘 채 그 뒤의 모든 진단을 영영 놓친다 — 최신 상태가 오래된 상태보다 알 가치가 크다.
func (s *Stream) emitErr(err error) {
	pushLossy(s.errs, err)
}

// emitReconnect 는 재연결 채널이 가득 찼을 때 가장 오래된 신호를 버린다 — 최신 재연결 신호가
// 살아남아야 한다(오래된 Cause 보다 지금 상태가 더 중요하다).
func (s *Stream) emitReconnect(r Reconnect) {
	pushLossy(s.reconnects, r)
}

func (s *Stream) closeChannels() {
	close(s.trades)
	close(s.orderbooks)
	close(s.orders)
	close(s.reconnects)
	close(s.errs)
}
