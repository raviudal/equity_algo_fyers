package strategy

import (
	"testing"
	"time"

	"github.com/algoengine/trading-system/fyers"
)

func TestEmaVwapStrategy(t *testing.T) {
	strat := NewEmaVwapStrategy(9, 21, 0.5, 1.5)
	if strat.Name() == "" {
		t.Fatalf("Expected strategy name to be non-empty")
	}

	// Create 30 synthetic bars
	now := time.Now().Unix()
	bars := make([]*fyers.Candle, 0, 30)
	price := 500.0

	for i := 0; i < 30; i++ {
		price += float64(i % 3)
		bars = append(bars, &fyers.Candle{
			Symbol:    "NSE:SBIN-EQ",
			Timestamp: now + int64(i*60),
			Open:      price,
			High:      price + 1.0,
			Low:       price - 1.0,
			Close:     price + 0.5,
			Volume:    1000,
			Period:    "1m",
			IsClosed:  true,
		})
	}

	sig := strat.Evaluate("NSE:SBIN-EQ", bars)
	if sig == nil {
		t.Fatalf("Expected non-nil signal")
	}
	if sig.Symbol != "NSE:SBIN-EQ" {
		t.Fatalf("Expected symbol NSE:SBIN-EQ, got %s", sig.Symbol)
	}
}
