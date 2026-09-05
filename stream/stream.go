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

	mu            sync.Mutex
	conn          *websocket.Conn // 현재 연결. 쓰기 전에 잠근다
	lastDeclareID string          // 마지막으로 보낸 선언의 id. 낡은 ack 를 걸러내는 데 쓴다
}

// New 는 스트림을 만들고 연결한다. 보통은 toss.Client.Stream 을 쓴다.
//
// ctx 는 최초 연결에만 쓰인다. 이후 스트림은 ctx 취소와 무관하게 살아 있으며 Close 로만 끝난다.
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
		go func() { defer wg.Done(); s.declareLoop(connCtx) }()
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
			c, err := dial(ctx, s.url, s.token)
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
			// 낡은 선언의 ack 는 무시한다 — 그 사이 사용자가 다시 넣은 구독을 지울 수 있다.
			// id 가 없는 ack(서버가 echo 하지 않은 경우)는 그대로 적용한다.
			s.mu.Lock()
			stale := f.id != "" && s.lastDeclareID != "" && f.id != s.lastDeclareID
			s.mu.Unlock()
			if stale {
				continue
			}
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
			if f.errCode == "rate-limit-exceeded" {
				// 선언 빈도(5회/초)에 걸린 선언은 반영되지 않았다. 대기 후 한 번 다시 선언한다
				// (토스는 Retry-After 를 주지 않는다). 재시도 자체가 또 걸리면 다음 Subscribe 나
				// 재연결 때 어차피 전체 집합이 다시 선언된다.
				s.retryDeclareAfter(ctx, s.cfg.rateLimitRetry)
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
			// 실제로 보낸 선언만 기록한다 — 보내지 못한 id 를 기록하면 그 다음에 도착하는
			// 정상 ack 가 stale 로 오인돼 거부 항목이 집합에 남는다.
			s.mu.Lock()
			s.lastDeclareID = id
			s.mu.Unlock()
		}
	}
}

// retryDeclareAfter 는 d 후에 선언을 한 번 더 요청한다. 연결이 끝나면 취소된다.
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
