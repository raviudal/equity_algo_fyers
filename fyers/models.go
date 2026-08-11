package fyers

import "time"

// Tick represents a single price update tick from Fyers API
type Tick struct {
	Symbol    string  `json:"symbol"`
	LastPrice float64 `json:"last_price"`
	Volume    int64   `json:"volume"`
	BidPrice  float64 `json:"bid_price,omitempty"`
	AskPrice  float64 `json:"ask_price,omitempty"`
	Open      float64 `json:"open,omitempty"`
	High      float64 `json:"high,omitempty"`
	Low       float64 `json:"low,omitempty"`
	Close     float64 `json:"close,omitempty"`
	PrevClose float64 `json:"prev_close,omitempty"`
	Timestamp int64   `json:"timestamp"`
}

// Candle represents an OHLC bar (1m, 5m, etc.)
type Candle struct {
	Symbol    string    `json:"symbol"`
	Timestamp int64     `json:"timestamp"` // Unix timestamp in seconds
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    int64     `json:"volume"`
	TimeStr   string    `json:"time_str,omitempty"`
	IsClosed  bool      `json:"is_closed"`
	Period    string    `json:"period"` // e.g. "1m", "5m"
}

// Position represents an active or closed trading position
type Position struct {
	ID           string    `json:"id"`
	Symbol       string    `json:"symbol"`
	Side         string    `json:"side"` // "BUY" or "SELL"
	Qty          int       `json:"qty"`
	EntryPrice   float64   `json:"entry_price"`
	CurrentPrice float64   `json:"current_price"`
	StopLoss     float64   `json:"stop_loss"`
	TakeProfit   float64   `json:"take_profit"`
	UnrealizedPnL float64  `json:"unrealized_pnl"`
	RealizedPnL   float64  `json:"realized_pnl"`
	EntryTime    time.Time `json:"entry_time"`
	Status       string    `json:"status"` // "OPEN", "CLOSED"
}

// Order represents an execution order
type Order struct {
	OrderID    string    `json:"order_id"`
	Symbol     string    `json:"symbol"`
	Side       string    `json:"side"` // "BUY" or "SELL"
	Qty        int       `json:"qty"`
	Type       string    `json:"type"` // "MARKET", "LIMIT"
	LimitPrice float64   `json:"limit_price,omitempty"`
	ExecPrice  float64   `json:"exec_price"`
	Status     string    `json:"status"` // "FILLED", "REJECTED", "CANCELLED"
	Message    string    `json:"message"`
	Timestamp  time.Time `json:"timestamp"`
}

// AccountMetrics holds performance and margin statistics
type AccountMetrics struct {
	TotalPnL       float64 `json:"total_pnl"`
	RealizedPnL    float64 `json:"realized_pnl"`
	UnrealizedPnL  float64 `json:"unrealized_pnl"`
	TotalTrades    int     `json:"total_trades"`
	WinningTrades  int     `json:"winning_trades"`
	LosingTrades   int     `json:"losing_trades"`
	WinRate        float64 `json:"win_rate"`
	MaxDrawdown    float64 `json:"max_drawdown"`
	AvailableMargin float64 `json:"available_margin"`
	UsedMargin     float64 `json:"used_margin"`
}

// Fyers Historical Response structure
type FyersHistoricalResponse struct {
	S       string        `json:"s"`
	Candles [][]interface{} `json:"candles"`
}
