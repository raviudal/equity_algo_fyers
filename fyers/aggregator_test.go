package fyers

import (
	"testing"
	"time"
)

func TestTickAggregator1mAnd5m(t *testing.T) {
	updatedCandles := make([]*Candle, 0)
	doneCandles := make([]*Candle, 0)

	agg := NewTickAggregator(
		[]string{"NSE:SBIN-EQ"},
		func(c *Candle) {
			updatedCandles = append(updatedCandles, c)
		},
		func(c *Candle) {
			doneCandles = append(doneCandles, c)
		},
	)

	baseTime := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC).Unix()

	// Tick 1 at 10:00:05
	agg.ProcessTick(&Tick{
		Symbol:    "NSE:SBIN-EQ",
		LastPrice: 500.0,
		Volume:    10,
		Timestamp: baseTime + 5,
	})

	// Tick 2 at 10:00:30 (High 505.0)
	agg.ProcessTick(&Tick{
		Symbol:    "NSE:SBIN-EQ",
		LastPrice: 505.0,
		Volume:    20,
		Timestamp: baseTime + 30,
	})

	// Tick 3 at 10:01:05 (New 1-min bar starts, closes previous bar)
	agg.ProcessTick(&Tick{
		Symbol:    "NSE:SBIN-EQ",
		LastPrice: 502.0,
		Volume:    15,
		Timestamp: baseTime + 65,
	})

	if len(doneCandles) == 0 {
		t.Fatalf("Expected completed 1-minute candle when new minute started")
	}

	closedBar := doneCandles[0]
	if closedBar.Open != 500.0 || closedBar.High != 505.0 || closedBar.Close != 505.0 {
		t.Fatalf("Unexpected closed bar OHLC values: O=%.2f H=%.2f C=%.2f", closedBar.Open, closedBar.High, closedBar.Close)
	}

	if closedBar.Volume != 30 {
		t.Fatalf("Expected volume 30, got %d", closedBar.Volume)
	}
}
