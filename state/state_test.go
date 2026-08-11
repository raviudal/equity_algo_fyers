package state

import (
	"testing"
	"time"

	"github.com/algoengine/trading-system/fyers"
)

func TestSystemStateBufferLimit(t *testing.T) {
	sysState := NewSystemState()
	sysState.MaxCandleBuffer = 10 // Set small limit for testing

	now := time.Now().Unix()
	for i := 0; i < 25; i++ {
		c := &fyers.Candle{
			Symbol:    "NSE:ITC-EQ",
			Timestamp: now + int64(i*60),
			Open:      480.0,
			High:      485.0,
			Low:       475.0,
			Close:     482.0,
			Volume:    100,
			Period:    "1m",
		}
		sysState.AddOrUpdateCandle(c)
	}

	candles := sysState.GetCandles("NSE:ITC-EQ", "1m")
	if len(candles) != 10 {
		t.Fatalf("Expected buffer pruned to 10 candles, got %d", len(candles))
	}
}

func TestPositionPnLCalculation(t *testing.T) {
	sysState := NewSystemState()

	pos := &fyers.Position{
		ID:           "POS-1",
		Symbol:       "NSE:ITC-EQ",
		Side:         "BUY",
		Qty:          100,
		EntryPrice:   480.0,
		CurrentPrice: 480.0,
		Status:       "OPEN",
	}
	sysState.AddPosition(pos)

	// Price increases to 490 (+10)
	updated := sysState.UpdatePositionPrice("NSE:ITC-EQ", 490.0)
	if len(updated) == 0 {
		t.Fatalf("Expected 1 updated position")
	}

	metrics := sysState.GetMetrics()
	if metrics.UnrealizedPnL != 1000.0 {
		t.Fatalf("Expected unrealized PnL 1000.0, got %f", metrics.UnrealizedPnL)
	}

	// Close position at 490
	closed := sysState.ClosePosition("POS-1", 490.0)
	if closed.RealizedPnL != 1000.0 {
		t.Fatalf("Expected realized PnL 1000.0, got %f", closed.RealizedPnL)
	}

	metrics2 := sysState.GetMetrics()
	if metrics2.RealizedPnL != 1000.0 {
		t.Fatalf("Expected total realized PnL 1000.0, got %f", metrics2.RealizedPnL)
	}
	if metrics2.WinRate != 100.0 {
		t.Fatalf("Expected win rate 100%%, got %f", metrics2.WinRate)
	}
}
