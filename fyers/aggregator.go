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
	current15m   map[string]*Candle
	current1h    map[string]*Candle
	onCandleUpd  CandleCallback
	onCandleDone CandleCallback
}

func NewTickAggregator(symbols []string, onUpdate, onComplete CandleCallback) *TickAggregator {
	return &TickAggregator{
		symbols:      symbols,
		current1m:    make(map[string]*Candle),
		current5m:    make(map[string]*Candle),
		current15m:   make(map[string]*Candle),
		current1h:    make(map[string]*Candle),
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
	a.processBar(tick.Symbol, tickTime.Truncate(time.Minute).Unix(), "1m", price, tick.Volume, a.current1m)

	// Process 5-minute bar
	a.processBar(tick.Symbol, tickTime.Truncate(5*time.Minute).Unix(), "5m", price, tick.Volume, a.current5m)

	// Process 15-minute bar
	a.processBar(tick.Symbol, tickTime.Truncate(15*time.Minute).Unix(), "15m", price, tick.Volume, a.current15m)

	// Process 1-hour bar
	a.processBar(tick.Symbol, tickTime.Truncate(time.Hour).Unix(), "1h", price, tick.Volume, a.current1h)
}

func (a *TickAggregator) processBar(symbol string, barTime int64, period string, price float64, volume int64, candleMap map[string]*Candle) {
	c, exists := candleMap[symbol]

	if !exists || c.Timestamp != barTime {
		if exists {
			c.IsClosed = true
			if a.onCandleDone != nil {
				a.onCandleDone(c)
			}
		}
		c = &Candle{
			Symbol:    symbol,
			Timestamp: barTime,
			Open:      price,
			High:      price,
			Low:       price,
			Close:     price,
			Volume:    volume,
			Period:    period,
			IsClosed:  false,
			TimeStr:   time.Unix(barTime, 0).Format("15:04"),
		}
		candleMap[symbol] = c
	} else {
		if price > c.High {
			c.High = price
		}
		if price < c.Low {
			c.Low = price
		}
		c.Close = price
		c.Volume += volume
	}

	if a.onCandleUpd != nil {
		a.onCandleUpd(c)
	}
}
