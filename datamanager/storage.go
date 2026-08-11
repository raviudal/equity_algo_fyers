package datamanager

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/algoengine/trading-system/fyers"
)

type SymbolDataSummary struct {
	Symbol          string `json:"symbol"`
	Interval        string `json:"interval"`
	TotalBars       int    `json:"total_bars"`
	OldestTime      string `json:"oldest_time"`
	NewestTime      string `json:"newest_time"`
	NewestTimestamp int64  `json:"newest_timestamp"`
	IsFresh         bool   `json:"is_fresh"`
}

type StorageManager struct {
	mu      sync.RWMutex
	DataDir string
}

func NewStorageManager(dataDir string) *StorageManager {
	if dataDir == "" {
		dataDir = "data"
	}
	os.MkdirAll(dataDir, 0755)
	return &StorageManager{
		DataDir: dataDir,
	}
}

func (sm *StorageManager) getFilePath(symbol, interval string) string {
	safeSym := strings.ReplaceAll(symbol, ":", "_")
	safeSym = strings.ReplaceAll(safeSym, "-", "_")
	filename := fmt.Sprintf("candles_%s_%s.json", safeSym, interval)
	return filepath.Join(sm.DataDir, filename)
}

func (sm *StorageManager) SaveCandles(symbol, interval string, candles []*fyers.Candle) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Sort candles by timestamp ascending
	sort.Slice(candles, func(i, j int) bool {
		return candles[i].Timestamp < candles[j].Timestamp
	})

	// Remove duplicate timestamps
	unique := make([]*fyers.Candle, 0, len(candles))
	seen := make(map[int64]bool)
	for _, c := range candles {
		if !seen[c.Timestamp] {
			seen[c.Timestamp] = true
			unique = append(unique, c)
		}
	}

	data, err := json.MarshalIndent(unique, "", "  ")
	if err != nil {
		return err
	}

	filePath := sm.getFilePath(symbol, interval)
	return os.WriteFile(filePath, data, 0644)
}

func (sm *StorageManager) AppendCandles(symbol, interval string, newCandles []*fyers.Candle) (int, error) {
	existing := sm.LoadCandles(symbol, interval)
	combined := append(existing, newCandles...)
	err := sm.SaveCandles(symbol, interval, combined)
	if err != nil {
		return 0, err
	}
	return len(newCandles), nil
}

func (sm *StorageManager) LoadCandles(symbol, interval string) []*fyers.Candle {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	filePath := sm.getFilePath(symbol, interval)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return []*fyers.Candle{}
	}

	var candles []*fyers.Candle
	if err := json.Unmarshal(data, &candles); err != nil {
		return []*fyers.Candle{}
	}
	return candles
}

func (sm *StorageManager) GetLatestTimestamp(symbol, interval string) int64 {
	candles := sm.LoadCandles(symbol, interval)
	if len(candles) == 0 {
		return 0
	}
	return candles[len(candles)-1].Timestamp
}

func (sm *StorageManager) DeleteAllData() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	entries, err := os.ReadDir(sm.DataDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "candles_") {
			os.Remove(filepath.Join(sm.DataDir, entry.Name()))
		}
	}
	return nil
}

func (sm *StorageManager) GetDataSummary(symbols []string, interval string) []SymbolDataSummary {
	summaries := make([]SymbolDataSummary, 0, len(symbols))
	now := time.Now().Unix()

	for _, sym := range symbols {
		candles := sm.LoadCandles(sym, interval)
		n := len(candles)

		if n == 0 {
			summaries = append(summaries, SymbolDataSummary{
				Symbol:          sym,
				Interval:        interval,
				TotalBars:       0,
				OldestTime:      "N/A",
				NewestTime:      "N/A",
				NewestTimestamp: 0,
				IsFresh:         false,
			})
			continue
		}

		oldestStr := time.Unix(candles[0].Timestamp, 0).Format("2006-01-02 15:04")
		newestStr := time.Unix(candles[n-1].Timestamp, 0).Format("2006-01-02 15:04")
		diffSeconds := now - candles[n-1].Timestamp
		
		// 15m bar is fresh if updated within last 2 hours (taking market hours into account)
		isFresh := diffSeconds < (2 * 3600)

		summaries = append(summaries, SymbolDataSummary{
			Symbol:          sym,
			Interval:        interval,
			TotalBars:       n,
			OldestTime:      oldestStr,
			NewestTime:      newestStr,
			NewestTimestamp: candles[n-1].Timestamp,
			IsFresh:         isFresh,
		})
	}

	return summaries
}
