package datamanager

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
)

type DataConfig struct {
	mu            sync.RWMutex
	StockListCSV  string `json:"stock_list_csv"`
	Interval      string `json:"interval"` // "15m", "1h"
	BackfillDays  int    `json:"backfill_days"`
	FilePath      string `json:"-"`
}

func DefaultDataConfig() *DataConfig {
	return &DataConfig{
		StockListCSV: "ITC, RELIANCE, SBIN, TCS, INFY",
		Interval:     "15m",
		BackfillDays: 90,
		FilePath:     "data_settings.json",
	}
}

func LoadDataConfig(filepath string) *DataConfig {
	cfg := DefaultDataConfig()
	if filepath != "" {
		cfg.FilePath = filepath
	}

	data, err := os.ReadFile(cfg.FilePath)
	if err != nil {
		cfg.Save()
		return cfg
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return cfg
	}

	if cfg.Interval != "15m" && cfg.Interval != "1h" {
		cfg.Interval = "15m"
	}
	if cfg.BackfillDays <= 0 {
		cfg.BackfillDays = 90
	}

	return cfg
}

func (c *DataConfig) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.FilePath, data, 0644)
}

func (c *DataConfig) Update(stockCSV, interval string) {
	c.mu.Lock()
	if strings.TrimSpace(stockCSV) != "" {
		c.StockListCSV = stockCSV
	}
	if interval == "15m" || interval == "1h" {
		c.Interval = interval
	}
	c.mu.Unlock()

	c.Save()
}

func (c *DataConfig) GetNormalizedSymbols() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	rawList := strings.Split(c.StockListCSV, ",")
	symbols := make([]string, 0, len(rawList))

	for _, item := range rawList {
		cleaned := strings.TrimSpace(item)
		if cleaned == "" {
			continue
		}
		// Convert e.g. "itc", "ITC-EQ", "NSE:ITC-EQ" to standard "NSE:ITC-EQ"
		norm := NormalizeSymbol(cleaned)
		symbols = append(symbols, norm)
	}
	return symbols
}

func NormalizeSymbol(raw string) string {
	s := strings.ToUpper(strings.TrimSpace(raw))
	if strings.HasPrefix(s, "NSE:") {
		s = strings.TrimPrefix(s, "NSE:")
	}
	if strings.HasSuffix(s, "-EQ") {
		s = strings.TrimSuffix(s, "-EQ")
	}
	return "NSE:" + s + "-EQ"
}

func FyersResolutionFromInterval(interval string) string {
	if interval == "1h" || interval == "60m" {
		return "60"
	}
	return "15" // Default 15m
}
