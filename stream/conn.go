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
