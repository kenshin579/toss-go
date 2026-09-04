// 계좌·주문 조회 예시. 실행: TOSS_CLIENT_ID=... TOSS_CLIENT_SECRET=... go run ./examples/order
//
// 이 예시는 조회만 한다. 실제 주문을 내는 코드는 주석으로만 두었다 — 실행하면 실제 체결로 이어진다.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	toss "github.com/kenshin579/toss-go"
	"github.com/kenshin579/toss-go/asset"
	"github.com/kenshin579/toss-go/order"
	"github.com/kenshin579/toss-go/tosstypes"
)

func main() {
	c, err := toss.NewClientFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	accts, err := c.Accounts(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if len(accts) == 0 {
		log.Fatal("no accounts")
	}
	a := c.Account(accts[0].AccountSeq)
	fmt.Printf("account %s (%s)\n", accts[0].AccountNo, accts[0].AccountType)

	h, err := a.Asset.Holdings(ctx, asset.HoldingsParams{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("평가금액 %s KRW, 손익률 %s\n", h.MarketValue.Amount.KRW, h.ProfitLoss.Rate)
	for _, it := range h.Items {
		fmt.Printf("  %s %s: %s주 @ %s\n", it.Symbol, it.Name, it.Quantity, it.LastPrice)
	}

	bp, err := a.Order.BuyingPower(ctx, tosstypes.CurrencyKRW)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("매수 가능 금액:", bp.CashBuyingPower)

	page, err := a.Order.List(ctx, order.ListParams{Status: order.StatusFilterOpen, Limit: 10})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("진행 중 주문 %d건\n", len(page.Orders))
	for _, o := range page.Orders {
		fmt.Printf("  %s %s %s %s주 (%s)\n", o.OrderID[:8], o.Symbol, o.Side, o.Quantity, o.Status)
	}

	// 실제 주문 예시 — 실행하면 진짜 주문이 나간다. 필요할 때만 주석을 풀 것.
	//
	//	res, err := a.Order.Place(ctx, order.PlaceRequest{
	//	    Symbol: "005930", Side: order.SideBuy, OrderType: order.TypeLimit,
	//	    Quantity: decimal.NewFromInt(1), Price: decimal.NewFromInt(50000),
	//	    ClientOrderID: toss.NewClientOrderID(), // 멱등성 키 권장
	//	})
	//	if err != nil { log.Fatal(err) }
	//	fmt.Println("주문 접수:", res.OrderID)
	//	if _, err := a.Order.Cancel(ctx, res.OrderID); err != nil { log.Fatal(err) }
}
