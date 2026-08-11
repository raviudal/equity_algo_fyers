package fyers

import (
	"sync"
	"time"
)

type CandleCallback func(candle *Candle)

type TickAggregator struct {
	mu           sync.Mutex
	symbols      []string
	current1m    map[string]*Candle
	current5m    map[string]*Candle
	onCandleUpd  CandleCallback
	onCandleDone CandleCallback
}

func NewTickAggregator(symbols []string, onUpdate, onComplete CandleCallback) *TickAggregator {
	return &TickAggregator{
		symbols:      symbols,
		current1m:    make(map[string]*Candle),
		current5m:    make(map[string]*Candle),
		onCandleUpd:  onUpdate,
		onCandleDone: onComplete,
	}
}

func (a *TickAggregator) ProcessTick(tick *Tick) {
	a.mu.Lock()
	defer a.mu.Unlock()

	tickTime := time.Unix(tick.Timestamp, 0)
	price := tick.LastPrice

	// Process 1-minute bar
	bar1mTime := tickTime.Truncate(time.Minute).Unix()
	c1m, exists1m := a.current1m[tick.Symbol]

	if !exists1m || c1m.Timestamp != bar1mTime {
		if exists1m {
			c1m.IsClosed = true
			if a.onCandleDone != nil {
				a.onCandleDone(c1m)
			}
		}
		c1m = &Candle{
			Symbol:    tick.Symbol,
			Timestamp: bar1mTime,
			Open:      price,
			High:      price,
			Low:       price,
			Close:     price,
			Volume:    tick.Volume,
			Period:    "1m",
			IsClosed:  false,
			TimeStr:   time.Unix(bar1mTime, 0).Format("15:04"),
		}
		a.current1m[tick.Symbol] = c1m
	} else {
		if price > c1m.High {
			c1m.High = price
		}
		if price < c1m.Low {
			c1m.Low = price
		}
		c1m.Close = price
		c1m.Volume += tick.Volume
	}

	if a.onCandleUpd != nil {
		a.onCandleUpd(c1m)
	}

	// Process 5-minute bar
	bar5mTime := tickTime.Truncate(5 * time.Minute).Unix()
	c5m, exists5m := a.current5m[tick.Symbol]

	if !exists5m || c5m.Timestamp != bar5mTime {
		if exists5m {
			c5m.IsClosed = true
			if a.onCandleDone != nil {
				a.onCandleDone(c5m)
			}
		}
		c5m = &Candle{
			Symbol:    tick.Symbol,
			Timestamp: bar5mTime,
			Open:      price,
			High:      price,
			Low:       price,
			Close:     price,
			Volume:    tick.Volume,
			Period:    "5m",
			IsClosed:  false,
			TimeStr:   time.Unix(bar5mTime, 0).Format("15:04"),
		}
		a.current5m[tick.Symbol] = c5m
	} else {
		if price > c5m.High {
			c5m.High = price
		}
		if price < c5m.Low {
			c5m.Low = price
		}
		c5m.Close = price
		c5m.Volume += tick.Volume
	}
}
