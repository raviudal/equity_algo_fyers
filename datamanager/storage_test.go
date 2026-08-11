package datamanager

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/algoengine/trading-system/fyers"
)

func TestStorageSaveLoadAndSummary(t *testing.T) {
	tempDir := filepath.Join(os.TempDir(), "test_data_storage")
	defer os.RemoveAll(tempDir)

	sm := NewStorageManager(tempDir)

	now := time.Now().Unix()
	candles := []*fyers.Candle{
		{Symbol: "NSE:ITC-EQ", Timestamp: now - 300, Open: 480, High: 482, Low: 479, Close: 481, Volume: 1000},
		{Symbol: "NSE:ITC-EQ", Timestamp: now - 150, Open: 481, High: 483, Low: 480, Close: 482, Volume: 1200},
	}

	err := sm.SaveCandles("NSE:ITC-EQ", "15m", candles)
	if err != nil {
		t.Fatalf("Failed saving candles: %v", err)
	}

	loaded := sm.LoadCandles("NSE:ITC-EQ", "15m")
	if len(loaded) != 2 {
		t.Fatalf("Expected 2 candles loaded, got %d", len(loaded))
	}

	latestTS := sm.GetLatestTimestamp("NSE:ITC-EQ", "15m")
	if latestTS != now-150 {
		t.Fatalf("Expected latest timestamp %d, got %d", now-150, latestTS)
	}

	summaries := sm.GetDataSummary([]string{"NSE:ITC-EQ"}, "15m")
	if len(summaries) != 1 || summaries[0].TotalBars != 2 {
		t.Fatalf("Unexpected data summary: %+v", summaries)
	}
}
