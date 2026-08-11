package datamanager

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/algoengine/trading-system/config"
	"github.com/algoengine/trading-system/fyers"
	"github.com/algoengine/trading-system/state"
	"github.com/algoengine/trading-system/ws"
)

type Collector struct {
	mu           sync.Mutex
	cfg          *config.Config
	dataCfg      *DataConfig
	storage      *StorageManager
	fyersClient  *fyers.Client
	state        *state.SystemState
	hub          *ws.Hub
	isSyncing    bool
	lastSyncTime time.Time
	ctx          context.Context
	cancel       context.CancelFunc
}

func NewCollector(cfg *config.Config, sysState *state.SystemState, hub *ws.Hub) *Collector {
	ctx, cancel := context.WithCancel(context.Background())
	dataCfg := LoadDataConfig("data_settings.json")
	storage := NewStorageManager("data")
	fClient := fyers.NewClient(cfg.FyersAppID, cfg.FyersAccessToken)

	return &Collector{
		cfg:          cfg,
		dataCfg:      dataCfg,
		storage:      storage,
		fyersClient:  fClient,
		state:        sysState,
		hub:          hub,
		isSyncing:    false,
		lastSyncTime: time.Time{},
		ctx:          ctx,
		cancel:       cancel,
	}
}

func (c *Collector) GetStorageManager() *StorageManager {
	return c.storage
}

func (c *Collector) StartBackgroundSync() {
	go func() {
		log.Println("[Collector] Background historical data collector initialized.")

		// Perform initial sync if authenticated
		if c.cfg.IsAuthenticated() {
			c.SyncLatestData()
		}

		// Periodic background ticker (every 15 minutes)
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
				if c.cfg.IsAuthenticated() {
					c.SyncLatestData()
				}
			}
		}
	}()
}

func (c *Collector) Stop() {
	c.cancel()
}

func (c *Collector) GetDataConfig() *DataConfig {
	return c.dataCfg
}

func (c *DataConfig) GetConfigSummary() map[string]interface{} {
	return map[string]interface{}{
		"stock_list_csv": c.StockListCSV,
		"interval":       c.Interval,
		"backfill_days":  c.BackfillDays,
		"symbols":        c.GetNormalizedSymbols(),
	}
}

func (c *Collector) SyncLatestData() (int, error) {
	c.mu.Lock()
	if c.isSyncing {
		c.mu.Unlock()
		return 0, fmt.Errorf("sync is already in progress")
	}
	c.isSyncing = true
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.isSyncing = false
		c.lastSyncTime = time.Now()
		c.mu.Unlock()
	}()

	if !c.cfg.IsAuthenticated() {
		return 0, fmt.Errorf("Fyers API authentication required for data sync")
	}

	symbols := c.dataCfg.GetNormalizedSymbols()
	interval := c.dataCfg.Interval
	resolution := FyersResolutionFromInterval(interval)
	now := time.Now().Unix()

	logMsg := fmt.Sprintf("Starting data sync for %d stocks (Interval: %s)...", len(symbols), interval)
	c.state.AddLog(logMsg)
	c.hub.BroadcastEvent("system_log", logMsg)

	// Rate-limited parallel execution pool: 5 workers with 100ms request throttle
	workerCount := 5
	jobs := make(chan string, len(symbols))
	for _, sym := range symbols {
		jobs <- sym
	}
	close(jobs)

	var wg sync.WaitGroup
	var totalFetched int
	var muTotal sync.Mutex
	rateLimiter := time.NewTicker(100 * time.Millisecond) // Throttle to max 10 req/sec
	defer rateLimiter.Stop()

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for sym := range jobs {
				<-rateLimiter.C // Rate-limiting check

				latestTS := c.storage.GetLatestTimestamp(sym, interval)

				if latestTS == 0 {
					// Perform full 90-day backfill for new symbol
					fetched := c.downloadFull90Days(sym, interval, resolution, now)
					muTotal.Lock()
					totalFetched += fetched
					muTotal.Unlock()
				} else {
					// Incremental sync from latestTS to now
					if now-latestTS > 60 { // Only if more than 1 minute behind
						candles, err := c.fyersClient.GetHistoricalData(sym, resolution, latestTS, now)
						if err == nil && len(candles) > 0 {
							added, _ := c.storage.AppendCandles(sym, interval, candles)
							muTotal.Lock()
							totalFetched += added
							muTotal.Unlock()
						}
					}
				}
			}
		}()
	}

	wg.Wait()

	doneMsg := fmt.Sprintf("Data sync complete! Fetched %d new candles for %d stocks.", totalFetched, len(symbols))
	c.state.AddLog(doneMsg)
	c.hub.BroadcastEvent("system_log", doneMsg)

	return totalFetched, nil
}

func (c *Collector) RedownloadFullData() (int, error) {
	c.mu.Lock()
	if c.isSyncing {
		c.mu.Unlock()
		return 0, fmt.Errorf("sync is already in progress")
	}
	c.isSyncing = true
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.isSyncing = false
		c.lastSyncTime = time.Now()
		c.mu.Unlock()
	}()

	if !c.cfg.IsAuthenticated() {
		return 0, fmt.Errorf("Fyers API authentication required for data sync")
	}

	symbols := c.dataCfg.GetNormalizedSymbols()
	interval := c.dataCfg.Interval
	resolution := FyersResolutionFromInterval(interval)
	now := time.Now().Unix()

	logMsg := fmt.Sprintf("Starting FULL 90-day re-download for %d stocks (Interval: %s)...", len(symbols), interval)
	c.state.AddLog(logMsg)
	c.hub.BroadcastEvent("system_log", logMsg)

	workerCount := 5
	jobs := make(chan string, len(symbols))
	for _, sym := range symbols {
		jobs <- sym
	}
	close(jobs)

	var wg sync.WaitGroup
	var totalFetched int
	var muTotal sync.Mutex
	rateLimiter := time.NewTicker(100 * time.Millisecond)
	defer rateLimiter.Stop()

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for sym := range jobs {
				<-rateLimiter.C
				fetched := c.downloadFull90Days(sym, interval, resolution, now)
				muTotal.Lock()
				totalFetched += fetched
				muTotal.Unlock()
			}
		}()
	}

	wg.Wait()

	doneMsg := fmt.Sprintf("Full 90-day re-download complete! Loaded %d total candles.", totalFetched)
	c.state.AddLog(doneMsg)
	c.hub.BroadcastEvent("system_log", doneMsg)

	return totalFetched, nil
}

func (c *Collector) downloadFull90Days(sym, interval, resolution string, now int64) int {
	days := c.dataCfg.BackfillDays
	if days <= 0 {
		days = 90
	}

	// Split 90 days into 3x 30-day date windows to avoid single API chunk payload limits
	windowSecs := int64(30 * 86400)
	startTime := now - int64(days*86400)

	allCandles := make([]*fyers.Candle, 0, 3000)

	for from := startTime; from < now; from += windowSecs {
		to := from + windowSecs
		if to > now {
			to = now
		}

		time.Sleep(100 * time.Millisecond) // Throttle request
		candles, err := c.fyersClient.GetHistoricalData(sym, resolution, from, to)
		if err == nil {
			allCandles = append(allCandles, candles...)
		}
	}

	if len(allCandles) > 0 {
		c.storage.SaveCandles(sym, interval, allCandles)
		return len(allCandles)
	}
	return 0
}

func (c *Collector) ClearAllData() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	err := c.storage.DeleteAllData()
	if err == nil {
		logMsg := "Cleared all stored historical candle data files."
		c.state.AddLog(logMsg)
		c.hub.BroadcastEvent("system_log", logMsg)
	}
	return err
}

func (c *Collector) GetSummary() map[string]interface{} {
	c.mu.Lock()
	isSyncing := c.isSyncing
	lastSync := c.lastSyncTime
	c.mu.Unlock()

	symbols := c.dataCfg.GetNormalizedSymbols()
	interval := c.dataCfg.Interval
	summaries := c.storage.GetDataSummary(symbols, interval)

	totalBars := 0
	for _, s := range summaries {
		totalBars += s.TotalBars
	}

	lastSyncStr := "Never"
	if !lastSync.IsZero() {
		lastSyncStr = lastSync.Format("2006-01-02 15:04:05")
	}

	return map[string]interface{}{
		"is_syncing":      isSyncing,
		"last_sync_time":  lastSyncStr,
		"stock_list_csv":  c.dataCfg.StockListCSV,
		"interval":        c.dataCfg.Interval,
		"backfill_days":   c.dataCfg.BackfillDays,
		"total_symbols":   len(symbols),
		"total_bars":      totalBars,
		"symbol_details":  summaries,
	}
}
