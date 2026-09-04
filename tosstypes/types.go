// Package tosstypes 는 toss-go 전역에서 쓰는 공용 타입(날짜, 열거값)이다.
// 열거값은 문자열 타입 + 상수로 두며, 토스가 새 값을 추가해도 거부하지 않고 그대로 보존한다.
package tosstypes

import (
	"fmt"
	"time"
)

// KST 는 토스증권 API 의 기준 타임존(UTC+9).
var KST = time.FixedZone("KST", 9*3600)

// Date 는 `YYYY-MM-DD` 형식의 날짜 문자열이다. JSON 에서 그대로 문자열로 오간다.
type Date string

// NewDate 는 t 의 날짜 부분(t 의 타임존 기준)으로 Date 를 만든다.
func NewDate(t time.Time) Date { return Date(t.Format("2006-01-02")) }

// String 은 원문 문자열을 돌려준다.
func (d Date) String() string { return string(d) }

// IsZero 는 빈 값 여부.
func (d Date) IsZero() bool { return d == "" }

// Time 은 KST 자정 시각으로 변환한다. 형식이 맞지 않으면 에러.
func (d Date) Time() (time.Time, error) {
	t, err := time.ParseInLocation("2006-01-02", string(d), KST)
	if err != nil {
		return time.Time{}, fmt.Errorf("tosstypes: invalid date %q: %w", string(d), err)
	}
	return t, nil
}

// Currency 는 통화 코드.
type Currency string

const (
	CurrencyKRW Currency = "KRW"
	CurrencyUSD Currency = "USD"
)

// MarketCountry 는 시장 국가.
type MarketCountry string

const (
	MarketCountryKR MarketCountry = "KR"
	MarketCountryUS MarketCountry = "US"
)

// Market 은 거래소/시장 구분.
type Market string

const (
	MarketKOSPI  Market = "KOSPI"
	MarketKOSDAQ Market = "KOSDAQ"
	MarketNYSE   Market = "NYSE"
	MarketNASDAQ Market = "NASDAQ"
	MarketAMEX   Market = "AMEX"
	MarketKRETC  Market = "KR_ETC"
	MarketUSETC  Market = "US_ETC"
)

// SecurityType 은 증권 종류.
type SecurityType string

const (
	SecurityTypeStock              SecurityType = "STOCK"
	SecurityTypeForeignStock       SecurityType = "FOREIGN_STOCK"
	SecurityTypeDepositaryReceipt  SecurityType = "DEPOSITARY_RECEIPT"
	SecurityTypeInfrastructureFund SecurityType = "INFRASTRUCTURE_FUND"
	SecurityTypeREIT               SecurityType = "REIT"
	SecurityTypeETF                SecurityType = "ETF"
	SecurityTypeForeignETF         SecurityType = "FOREIGN_ETF"
	SecurityTypeETN                SecurityType = "ETN"
	SecurityTypeStockWarrants      SecurityType = "STOCK_WARRANTS"
)

// StockStatus 는 상장 상태.
type StockStatus string

const (
	StockStatusScheduled StockStatus = "SCHEDULED"
	StockStatusActive    StockStatus = "ACTIVE"
	StockStatusDelisted  StockStatus = "DELISTED"
)

// Interval 은 캔들 봉 단위.
type Interval string

const (
	Interval1m Interval = "1m"
	Interval1d Interval = "1d"
)

// IndicatorInterval 은 시장 지표 투자자별 매매대금의 집계 단위.
type IndicatorInterval string

const (
	IndicatorInterval1d  IndicatorInterval = "1d"
	IndicatorInterval1w  IndicatorInterval = "1w"
	IndicatorInterval1mo IndicatorInterval = "1mo"
	IndicatorInterval1y  IndicatorInterval = "1y"
)

// RankingType 은 랭킹 종류.
type RankingType string

const (
	RankingMarketTradingAmount         RankingType = "MARKET_TRADING_AMOUNT"
	RankingMarketTradingVolume         RankingType = "MARKET_TRADING_VOLUME"
	RankingTopGainers                  RankingType = "TOP_GAINERS"
	RankingTopLosers                   RankingType = "TOP_LOSERS"
	RankingTossSecuritiesTradingAmount RankingType = "TOSS_SECURITIES_TRADING_AMOUNT"
	RankingTossSecuritiesTradingVolume RankingType = "TOSS_SECURITIES_TRADING_VOLUME"
)

// RankingDuration 은 랭킹 집계 기간.
type RankingDuration string

const (
	RankingDurationRealtime RankingDuration = "realtime"
	RankingDuration1d       RankingDuration = "1d"
	RankingDuration1w       RankingDuration = "1w"
	RankingDuration1mo      RankingDuration = "1mo"
	RankingDuration3mo      RankingDuration = "3mo"
	RankingDuration6mo      RankingDuration = "6mo"
	RankingDuration1y       RankingDuration = "1y"
)

// RateChangeType 은 환율 변동 방향.
type RateChangeType string

const (
	RateChangeUp    RateChangeType = "UP"
	RateChangeEqual RateChangeType = "EQUAL"
	RateChangeDown  RateChangeType = "DOWN"
)

// WarningType 은 매수 유의사항 종류.
type WarningType string

const (
	WarningLiquidationTrading WarningType = "LIQUIDATION_TRADING"
	WarningOverheated         WarningType = "OVERHEATED"
	WarningInvestmentWarning  WarningType = "INVESTMENT_WARNING"
	WarningInvestmentRisk     WarningType = "INVESTMENT_RISK"
	WarningVIStaticAndDynamic WarningType = "VI_STATIC_AND_DYNAMIC"
	WarningVIStatic           WarningType = "VI_STATIC"
	WarningVIDynamic          WarningType = "VI_DYNAMIC"
	WarningStockWarrants      WarningType = "STOCK_WARRANTS"
)
