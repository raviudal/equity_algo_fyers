package strategy

import "github.com/algoengine/trading-system/fyers"

type SignalType string

const (
	SignalBuy  SignalType = "BUY"
	SignalSell SignalType = "SELL"
	SignalHold SignalType = "HOLD"
)

type Signal struct {
	Symbol     string     `json:"symbol"`
	Type       SignalType `json:"type"`
	Price      float64    `json:"price"`
	Reason     string     `json:"reason"`
	Timestamp  int64      `json:"timestamp"`
	StopLoss   float64    `json:"stop_loss"`
	TakeProfit float64    `json:"take_profit"`
}

type Strategy interface {
	Name() string
	Evaluate(symbol string, bars []*fyers.Candle) *Signal
}
