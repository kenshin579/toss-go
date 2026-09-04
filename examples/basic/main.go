// 시세·캔들 조회 예시. 실행: TOSS_CLIENT_ID=... TOSS_CLIENT_SECRET=... go run ./examples/basic
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	toss "github.com/kenshin579/toss-go"
	"github.com/kenshin579/toss-go/marketdata"
	"github.com/kenshin579/toss-go/tosstypes"
)

func main() {
	c, err := toss.NewClientFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	prices, err := c.MarketData.Prices(ctx, "005930", "AAPL")
	if err != nil {
		log.Fatal(err)
	}
	for _, p := range prices {
		fmt.Printf("%s %s %s\n", p.Symbol, p.LastPrice, p.Currency)
	}

	page, err := c.MarketData.Candles(ctx, marketdata.CandlesParams{Symbol: "005930", Interval: tosstypes.Interval1d, Count: 5})
	if err != nil {
		log.Fatal(err)
	}
	for _, k := range page.Candles {
		fmt.Printf("%s O=%s H=%s L=%s C=%s V=%s\n", k.Timestamp.Format("2006-01-02"), k.OpenPrice, k.HighPrice, k.LowPrice, k.ClosePrice, k.Volume)
	}
	if page.NextBefore != nil {
		fmt.Println("next before:", page.NextBefore.Format(time.RFC3339))
	}

	// 에러 처리: 토스 에러 코드로 분기
	if _, err := c.StockInfo.Warnings(ctx, "000000"); err != nil {
		var ae *toss.APIError
		if errors.As(err, &ae) {
			fmt.Printf("api error: status=%d code=%s requestId=%s\n", ae.StatusCode, ae.Code, ae.RequestID)
		}
		if toss.IsCode(err, "stock-not-found") {
			fmt.Println("존재하지 않는 종목")
		}
	}
}
