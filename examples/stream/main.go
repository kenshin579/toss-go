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
