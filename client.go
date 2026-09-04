// Package toss 는 토스증권 Open API(https://developers.tossinvest.com)의 Go 클라이언트다.
//
// 인증은 OAuth2 Client Credentials 로, 첫 호출 때 토큰을 발급해 만료 전까지 재사용한다.
// 수치는 shopspring/decimal, 시각은 time.Time(KST 오프셋), 날짜는 tosstypes.Date 로 표현한다.
//
//	import toss "github.com/kenshin579/toss-go"
//
//	c, _ := toss.NewClientFromEnv() // TOSS_CLIENT_ID / TOSS_CLIENT_SECRET
//	ps, err := c.MarketData.Prices(ctx, "005930", "AAPL")
package toss

import (
	"context"
	"errors"
	"net/http"

	"github.com/kenshin579/toss-go/indicators"
	"github.com/kenshin579/toss-go/internal/auth"
	"github.com/kenshin579/toss-go/internal/httpclient"
	"github.com/kenshin579/toss-go/marketdata"
	"github.com/kenshin579/toss-go/marketinfo"
	"github.com/kenshin579/toss-go/ranking"
	"github.com/kenshin579/toss-go/stockinfo"
)

// Client 는 toss-go 의 단일 진입점. 그룹별 sub-client 를 필드로 노출한다.
// Client 와 sub-client 는 여러 goroutine 에서 동시에 사용해도 안전하다.
type Client struct {
	hc     *http.Client
	tokens *auth.TokenSource

	MarketData       *marketdata.Client // 시세: 현재가·호가·체결·상하한가·캔들
	StockInfo        *stockinfo.Client  // 종목: 메타·전체 목록·유의사항·매매동향 5종
	MarketInfo       *marketinfo.Client // 시장 정보: 환율·장 운영 정보
	Ranking          *ranking.Client    // 주식 랭킹
	MarketIndicators *indicators.Client // 시장 지표: 지수 현재가·캔들·투자자별 매매대금
}

// NewClient 는 client credentials 로 Client 를 만든다. 생성 시 네트워크 호출은 없다(토큰은 lazy).
func NewClient(clientID, clientSecret string, opts ...Option) (*Client, error) {
	if clientID == "" || clientSecret == "" {
		return nil, errors.New("toss: clientID and clientSecret are required")
	}
	cfg := clientOptions{baseURL: httpclient.DefaultBaseURL, timeout: httpclient.DefaultTimeout}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.baseURL == "" { // WithBaseURL("") 이 토큰 URL 만 상대경로로 만드는 것을 막는다
		cfg.baseURL = httpclient.DefaultBaseURL
	}
	hc := cfg.httpClient
	if hc == nil {
		hc = &http.Client{Timeout: cfg.timeout}
	}
	tokens := auth.New(clientID, clientSecret, cfg.baseURL, hc)
	h := httpclient.New(httpclient.Config{BaseURL: cfg.baseURL, HTTPClient: hc, Tokens: tokens})

	return &Client{
		hc:               hc,
		tokens:           tokens,
		MarketData:       marketdata.New(h),
		StockInfo:        stockinfo.New(h),
		MarketInfo:       marketinfo.New(h),
		Ranking:          ranking.New(h),
		MarketIndicators: indicators.New(h),
	}, nil
}

// AccessToken 은 유효한 access token 을 돌려준다(필요 시 발급/갱신). 웹소켓 연결 등 외부 용도.
func (c *Client) AccessToken(ctx context.Context) (string, error) {
	return c.tokens.Token(ctx)
}
