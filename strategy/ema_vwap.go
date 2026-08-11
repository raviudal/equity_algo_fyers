package strategy

import (
	"fmt"
	"math"

	"github.com/algoengine/trading-system/fyers"
)

type EmaVwapStrategy struct {
	FastPeriod int
	SlowPeriod int
	StopLossPct float64
	TakeProfitPct float64
}

func NewEmaVwapStrategy(fastPeriod, slowPeriod int, slPct, tpPct float64) *EmaVwapStrategy {
	return &EmaVwapStrategy{
		FastPeriod:   fastPeriod,
		SlowPeriod:   slowPeriod,
		StopLossPct:  slPct,
		TakeProfitPct: tpPct,
	}
}

func (s *EmaVwapStrategy) Name() string {
	return fmt.Sprintf("EMA(%d/%d) + VWAP Breakout", s.FastPeriod, s.SlowPeriod)
}

func (s *EmaVwapStrategy) Evaluate(symbol string, bars []*fyers.Candle) *Signal {
	n := len(bars)
	if n < s.SlowPeriod {
		return &Signal{Symbol: symbol, Type: SignalHold, Reason: "Insufficient bars"}
	}

	closes := make([]float64, n)
	for i, b := range bars {
		closes[i] = b.Close
	}

	fastEMA := calculateEMA(closes, s.FastPeriod)
	slowEMA := calculateEMA(closes, s.SlowPeriod)
	vwap := calculateVWAP(bars)

	lastIdx := n - 1
	currClose := closes[lastIdx]
	currFast := fastEMA[lastIdx]
	currSlow := slowEMA[lastIdx]
	currVWAP := vwap[lastIdx]

	prevFast := fastEMA[lastIdx-1]
	prevSlow := slowEMA[lastIdx-1]

	// Bullish Crossover: Fast EMA crosses above Slow EMA AND Price is above VWAP
	isBullishCross := (prevFast <= prevSlow) && (currFast > currSlow)
	isAboveVWAP := currClose > currVWAP

	if isBullishCross && isAboveVWAP {
		sl := round2(currClose * (1.0 - s.StopLossPct/100.0))
		tp := round2(currClose * (1.0 + s.TakeProfitPct/100.0))

		return &Signal{
			Symbol:     symbol,
			Type:       SignalBuy,
			Price:      currClose,
			Reason:     fmt.Sprintf("EMA(%d) %.2f crossed above EMA(%d) %.2f & Price > VWAP %.2f", s.FastPeriod, currFast, s.SlowPeriod, currSlow, currVWAP),
			Timestamp:  bars[lastIdx].Timestamp,
			StopLoss:   sl,
			TakeProfit: tp,
		}
	}

	// Bearish Condition: Fast EMA crosses below Slow EMA OR Price drops below VWAP
	isBearishCross := (prevFast >= prevSlow) && (currFast < currSlow)
	isBelowVWAP := currClose < (currVWAP * 0.998)

	if isBearishCross || isBelowVWAP {
		sl := round2(currClose * (1.0 + s.StopLossPct/100.0))
		tp := round2(currClose * (1.0 - s.TakeProfitPct/100.0))

		return &Signal{
			Symbol:     symbol,
			Type:       SignalSell,
			Price:      currClose,
			Reason:     fmt.Sprintf("EMA(%d) %.2f below EMA(%d) %.2f or Price < VWAP %.2f", s.FastPeriod, currFast, s.SlowPeriod, currSlow, currVWAP),
			Timestamp:  bars[lastIdx].Timestamp,
			StopLoss:   sl,
			TakeProfit: tp,
		}
	}

	return &Signal{
		Symbol:    symbol,
		Type:      SignalHold,
		Reason:    "No crossover threshold met",
		Timestamp: bars[lastIdx].Timestamp,
	}
}

func calculateEMA(prices []float64, period int) []float64 {
	n := len(prices)
	ema := make([]float64, n)
	if n < period {
		return ema
	}

	// Calculate initial SMA for the first 'period' elements
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += prices[i]
	}
	ema[period-1] = sum / float64(period)

	multiplier := 2.0 / float64(period+1)
	for i := period; i < n; i++ {
		ema[i] = (prices[i]-ema[i-1])*multiplier + ema[i-1]
	}
	return ema
}

func calculateVWAP(bars []*fyers.Candle) []float64 {
	n := len(bars)
	vwap := make([]float64, n)

	cumVolume := 0.0
	cumTPV := 0.0 // Typical Price * Volume

	for i, b := range bars {
		typicalPrice := (b.High + b.Low + b.Close) / 3.0
		vol := float64(b.Volume)
		if vol == 0 {
			vol = 1
		}
		cumTPV += typicalPrice * vol
		cumVolume += vol
		vwap[i] = cumTPV / cumVolume
	}

	return vwap
}

func round2(val float64) float64 {
	return math.Round(val*100) / 100
}
