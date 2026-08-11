package main

import (
	"embed"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/algoengine/trading-system/config"
	"github.com/algoengine/trading-system/datamanager"
	"github.com/algoengine/trading-system/fyers"
	"github.com/algoengine/trading-system/server"
	"github.com/algoengine/trading-system/state"
	"github.com/algoengine/trading-system/strategy"
	"github.com/algoengine/trading-system/ws"
)

//go:embed web/*
var embedWebFiles embed.FS

func main() {
	log.Println("==========================================================")
	log.Println(" Starting AlgoEngine Trading Core (Go + Fyers API v3)")
	log.Println("==========================================================")

	// 1. Load Configuration
	cfg := config.LoadConfig()
	if !cfg.IsAuthenticated() {
		log.Println("[Config] FYERS live credentials missing. Awaiting user login.")
	} else {
		log.Printf("[Config] FYERS App ID: %s authenticated.", cfg.FyersAppID)
	}

	// 2. Initialize Thread-safe State Manager
	sysState := state.NewSystemState()
	sysState.AddLog(fmt.Sprintf("Engine started in %s mode", cfg.Env))

	// 3. Initialize Token Master & Download Cash Market Symbols (NSE_CM)
	tokenMaster := fyers.NewTokenMaster()
	tokenMaster.DownloadAllMastersAsync()

	// 4. Initialize Fyers REST Client
	fyersClient := fyers.NewClient(cfg.FyersAppID, cfg.FyersAccessToken)
	if cfg.IsAuthenticated() {
		seedHistoricalCandles(sysState, fyersClient, cfg.Symbols)
	}

	// 5. Initialize WebSocket Server Hub
	wsHub := ws.NewHub()
	go wsHub.Run()

	// 6. Initialize Data Manager & Background Collector Routine
	collector := datamanager.NewCollector(cfg, sysState, wsHub)
	collector.StartBackgroundSync()

	// 7. Initialize Strategy Engine (EMA 9/21 Crossover + VWAP Breakout)
	strat := strategy.NewEmaVwapStrategy(9, 21, 0.5, 1.5)
	log.Printf("[Strategy] Loaded strategy: %s", strat.Name())

	// 8. Initialize Tick Aggregator
	aggregator := fyers.NewTickAggregator(
		cfg.Symbols,
		func(c *fyers.Candle) {
			// On candle update
			sysState.AddOrUpdateCandle(c)
			wsHub.BroadcastEvent("candle_update", c)
		},
		func(c *fyers.Candle) {
			// On candle complete/closed
			sysState.AddOrUpdateCandle(c)
			wsHub.BroadcastEvent("candle_update", c)
		},
	)

	// 9. Initialize Data Stream (Fyers WebSocket Socket)
	dataStream := fyers.NewDataStream(cfg.Symbols, cfg.FyersAppID, cfg.FyersAccessToken)
	dataStream.Start()
	defer dataStream.Stop()

	// 10. Main Engine Processing Loop (Ticks & Strategy Evaluation)
	go func() {
		tickChan := dataStream.GetTickChan()

		for tick := range tickChan {
			// Aggregate tick into OHLC bars
			aggregator.ProcessTick(tick)

			// Update active position PnLs with LTP
			sysState.UpdatePositionPrice(tick.Symbol, tick.LastPrice)

			// Broadcast metrics to web clients
			wsHub.BroadcastEvent("metrics_update", sysState.GetMetrics())

			// Skip strategy signals if Algo is paused by user
			if !sysState.GetAlgoRunning() {
				continue
			}

			// Evaluate Strategy on 1m candles
			bars1m := sysState.GetCandles(tick.Symbol, "1m")
			if len(bars1m) < 21 {
				continue
			}

			sig := strat.Evaluate(tick.Symbol, bars1m)
			if sig != nil && (sig.Type == strategy.SignalBuy || sig.Type == strategy.SignalSell) {
				handleTradeSignal(sysState, wsHub, fyersClient, sig, tick.LastPrice)
			}
		}
	}()

	// 11. Start Embedded Static Web Server & REST API
	srv := server.NewServer(cfg, sysState, wsHub, collector, embedWebFiles)
	handler := srv.SetupRoutes()

	serverAddr := ":" + cfg.Port
	log.Printf("[HTTP Server] Listening on http://localhost%s (Default Chart: NSE:ITC-EQ)", serverAddr)
	sysState.AddLog(fmt.Sprintf("Dashboard active on port %s (Default: NSE:ITC-EQ)", cfg.Port))

	if err := http.ListenAndServe(serverAddr, handler); err != nil {
		log.Fatalf("[HTTP Server] Fatal error: %v", err)
	}
}

func seedHistoricalCandles(sysState *state.SystemState, client *fyers.Client, symbols []string) {
	now := time.Now().Unix()
	from1m := now - (200 * 60) // Last 200 minutes

	for _, sym := range symbols {
		bars1m, err := client.GetHistoricalData(sym, "1", from1m, now)
		if err == nil {
			for _, b := range bars1m {
				sysState.AddOrUpdateCandle(b)
			}
			log.Printf("[Seeder] Loaded %d 1m candles for %s", len(bars1m), sym)
		} else {
			log.Printf("[Seeder] Notice: Historical candles fetch for %s: %v", sym, err)
		}
	}
}

func handleTradeSignal(sysState *state.SystemState, wsHub *ws.Hub, client *fyers.Client, sig *strategy.Signal, price float64) {
	positions := sysState.GetPositions()
	for _, p := range positions {
		if p.Symbol == sig.Symbol && p.Status == "OPEN" {
			if p.Side == string(sig.Type) {
				return // Already in position
			} else {
				// Reverse position: Close existing
				closedPos := sysState.ClosePosition(p.ID, price)
				if closedPos != nil {
					logMsg := fmt.Sprintf("Position Closed: %s %s @ ₹%.2f | PnL: ₹%.2f", closedPos.Side, closedPos.Symbol, price, closedPos.RealizedPnL)
					sysState.AddLog(logMsg)
					wsHub.BroadcastEvent("system_log", logMsg)
				}
			}
		}
	}

	// Create new order & position
	order := &fyers.Order{
		Symbol:     sig.Symbol,
		Side:       string(sig.Type),
		Qty:        10,
		Type:       "MARKET",
		LimitPrice: price,
	}

	if err := client.PlaceOrder(order); err != nil {
		sysState.AddLog(fmt.Sprintf("Order Failed: %v", err))
		return
	}

	posID := fmt.Sprintf("POS-%d", time.Now().UnixNano())
	newPos := &fyers.Position{
		ID:           posID,
		Symbol:       sig.Symbol,
		Side:         string(sig.Type),
		Qty:          10,
		EntryPrice:   price,
		CurrentPrice: price,
		StopLoss:     sig.StopLoss,
		TakeProfit:   sig.TakeProfit,
		EntryTime:    time.Now(),
		Status:       "OPEN",
	}

	sysState.AddPosition(newPos)
	sysState.AddOrder(order)

	// Broadcast trade execution event to chart (places BUY/SELL markers)
	tradeExecData := map[string]interface{}{
		"symbol":    sig.Symbol,
		"side":      sig.Type,
		"price":     price,
		"qty":       10,
		"reason":    sig.Reason,
		"timestamp": sig.Timestamp,
	}
	wsHub.BroadcastEvent("trade_execution", tradeExecData)

	logMsg := fmt.Sprintf("Trade Executed: %s 10 qty of %s @ ₹%.2f", sig.Type, sig.Symbol, price)
	sysState.AddLog(logMsg)
	wsHub.BroadcastEvent("system_log", logMsg)
}
