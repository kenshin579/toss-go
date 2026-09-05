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
	live     int           // 현재 살아 있는 연결 수
	maxLive  int           // 관측된 최대 동시 연결 수
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
		ts.live++
		if ts.live > ts.maxLive {
			ts.maxLive = ts.live
		}
		ts.mu.Unlock()
		defer func() {
			ts.mu.Lock()
			ts.live--
			ts.mu.Unlock()
		}()

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

// maxConcurrent 는 지금까지 관측된 최대 동시 연결 수다(재연결 전에 기존 연결을 닫는지 검증용).
func (ts *testServer) maxConcurrent() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.maxLive
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
