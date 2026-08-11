package state

import (
	"sync"
	"time"

	"github.com/algoengine/trading-system/fyers"
)

type SystemState struct {
	mu              sync.RWMutex
	StartTime       time.Time
	IsAlgoRunning   bool
	Candles         map[string]map[string][]*fyers.Candle // symbol -> period -> []Candle
	Positions       map[string]*fyers.Position            // positionID -> Position
	Orders          []*fyers.Order
	Metrics         fyers.AccountMetrics
	MaxCandleBuffer int
	Logs            []string
}

func NewSystemState() *SystemState {
	return &SystemState{
		StartTime:       time.Now(),
		IsAlgoRunning:   true,
		Candles:         make(map[string]map[string][]*fyers.Candle),
		Positions:       make(map[string]*fyers.Position),
		Orders:          make([]*fyers.Order, 0, 50),
		MaxCandleBuffer: 1000,
		Metrics: fyers.AccountMetrics{
			TotalPnL:        0.0,
			RealizedPnL:     0.0,
			UnrealizedPnL:   0.0,
			TotalTrades:     0,
			WinningTrades:   0,
			LosingTrades:    0,
			WinRate:         0.0,
			MaxDrawdown:     0.0,
			AvailableMargin: 0.0,
			UsedMargin:      0.0,
		},
		Logs: make([]string, 0, 100),
	}
}

func (s *SystemState) SetAlgoRunning(running bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.IsAlgoRunning = running
}

func (s *SystemState) GetAlgoRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.IsAlgoRunning
}

// AddOrUpdateCandle inserts or updates an OHLC bar while ensuring memory stays capped at 1000 bars
func (s *SystemState) AddOrUpdateCandle(candle *fyers.Candle) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.Candles[candle.Symbol]; !exists {
		s.Candles[candle.Symbol] = make(map[string][]*fyers.Candle)
	}

	periodBars := s.Candles[candle.Symbol][candle.Period]
	n := len(periodBars)

	if n > 0 && periodBars[n-1].Timestamp == candle.Timestamp {
		// Update existing candle bar in-place
		periodBars[n-1] = candle
	} else {
		// Append new candle bar
		periodBars = append(periodBars, candle)
		// Prune buffer if it exceeds max limit (memory protection)
		if len(periodBars) > s.MaxCandleBuffer {
			periodBars = periodBars[len(periodBars)-s.MaxCandleBuffer:]
		}
		s.Candles[candle.Symbol][candle.Period] = periodBars
	}
}

func (s *SystemState) GetCandles(symbol, period string) []*fyers.Candle {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if symMap, ok := s.Candles[symbol]; ok {
		if bars, ok := symMap[period]; ok {
			result := make([]*fyers.Candle, len(bars))
			copy(result, bars)
			return result
		}
	}
	return []*fyers.Candle{}
}

func (s *SystemState) AddPosition(pos *fyers.Position) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Positions[pos.ID] = pos
	s.recalculateMetricsLocked()
}

func (s *SystemState) UpdatePositionPrice(symbol string, currentPrice float64) []*fyers.Position {
	s.mu.Lock()
	defer s.mu.Unlock()

	updatedPositions := make([]*fyers.Position, 0)
	for _, pos := range s.Positions {
		if pos.Symbol == symbol && pos.Status == "OPEN" {
			pos.CurrentPrice = currentPrice
			if pos.Side == "BUY" {
				pos.UnrealizedPnL = float64(pos.Qty) * (currentPrice - pos.EntryPrice)
			} else {
				pos.UnrealizedPnL = float64(pos.Qty) * (pos.EntryPrice - currentPrice)
			}
			updatedPositions = append(updatedPositions, pos)
		}
	}
	s.recalculateMetricsLocked()
	return updatedPositions
}

func (s *SystemState) ClosePosition(posID string, exitPrice float64) *fyers.Position {
	s.mu.Lock()
	defer s.mu.Unlock()

	pos, exists := s.Positions[posID]
	if !exists || pos.Status != "OPEN" {
		return nil
	}

	pos.Status = "CLOSED"
	pos.CurrentPrice = exitPrice
	if pos.Side == "BUY" {
		pos.RealizedPnL = float64(pos.Qty) * (exitPrice - pos.EntryPrice)
	} else {
		pos.RealizedPnL = float64(pos.Qty) * (pos.EntryPrice - exitPrice)
	}
	pos.UnrealizedPnL = 0

	s.Metrics.TotalTrades++
	if pos.RealizedPnL > 0 {
		s.Metrics.WinningTrades++
	} else {
		s.Metrics.LosingTrades++
	}
	s.Metrics.RealizedPnL += pos.RealizedPnL

	s.recalculateMetricsLocked()
	return pos
}

func (s *SystemState) AddOrder(order *fyers.Order) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Orders = append(s.Orders, order)
	if len(s.Orders) > 200 {
		s.Orders = s.Orders[len(s.Orders)-200:]
	}
}

func (s *SystemState) GetPositions() []*fyers.Position {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*fyers.Position, 0, len(s.Positions))
	for _, p := range s.Positions {
		result = append(result, p)
	}
	return result
}

func (s *SystemState) GetMetrics() fyers.AccountMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Metrics
}

func (s *SystemState) recalculateMetricsLocked() {
	unrealized := 0.0
	usedMargin := 0.0

	for _, pos := range s.Positions {
		if pos.Status == "OPEN" {
			unrealized += pos.UnrealizedPnL
			usedMargin += pos.EntryPrice * float64(pos.Qty) * 0.20 // 20% margin requirement
		}
	}

	s.Metrics.UnrealizedPnL = unrealized
	s.Metrics.TotalPnL = s.Metrics.RealizedPnL + s.Metrics.UnrealizedPnL
	s.Metrics.UsedMargin = usedMargin

	if s.Metrics.TotalTrades > 0 {
		s.Metrics.WinRate = (float64(s.Metrics.WinningTrades) / float64(s.Metrics.TotalTrades)) * 100.0
	} else {
		s.Metrics.WinRate = 0.0
	}
}

func (s *SystemState) AddLog(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	timestamp := time.Now().Format("15:04:05")
	entry := "[" + timestamp + "] " + msg
	s.Logs = append(s.Logs, entry)
	if len(s.Logs) > 100 {
		s.Logs = s.Logs[len(s.Logs)-100:]
	}
}

func (s *SystemState) GetLogs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]string, len(s.Logs))
	copy(res, s.Logs)
	return res
}

func (s *SystemState) ClearSessionData() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Candles = make(map[string]map[string][]*fyers.Candle)
	s.Positions = make(map[string]*fyers.Position)
	s.Orders = make([]*fyers.Order, 0)
	s.Metrics = fyers.AccountMetrics{}
}
