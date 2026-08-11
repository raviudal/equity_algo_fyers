package fyers

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SymbolMasterRecord struct {
	FyToken     string  `json:"fy_token"`
	Symbol      string  `json:"symbol"`
	Description string  `json:"description"`
	Exchange    string  `json:"exchange"`
	LotSize     int     `json:"lot_size"`
	TickSize    float64 `json:"tick_size"`
	Segment     string  `json:"segment"`
}

type TokenMaster struct {
	mu          sync.RWMutex
	bySymbol    map[string]*SymbolMasterRecord
	byFyToken   map[string]*SymbolMasterRecord
	isLoaded    bool
	lastUpdated time.Time
}

func NewTokenMaster() *TokenMaster {
	return &TokenMaster{
		bySymbol:  make(map[string]*SymbolMasterRecord),
		byFyToken: make(map[string]*SymbolMasterRecord),
	}
}

func (tm *TokenMaster) IsLoaded() bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.isLoaded
}

func (tm *TokenMaster) LookupBySymbol(symbol string) (*SymbolMasterRecord, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	rec, ok := tm.bySymbol[symbol]
	return rec, ok
}

func (tm *TokenMaster) LookupByFyToken(token string) (*SymbolMasterRecord, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	rec, ok := tm.byFyToken[token]
	return rec, ok
}

func (tm *TokenMaster) DownloadAllMastersAsync() {
	go func() {
		log.Println("[TokenMaster] Starting download of Fyers Cash Market Symbol Master...")

		// Download Cash Market Master (NSE_CM)
		cmCount, err := tm.downloadAndParseCSV("https://public.fyers.in/sym_details/NSE_CM.csv", "NSE_CM")
		if err != nil {
			log.Printf("[TokenMaster] Warning: Failed downloading NSE_CM.csv: %v", err)
		} else {
			log.Printf("[TokenMaster] Successfully loaded %d Cash Market symbols (NSE_CM)", cmCount)
		}

		tm.mu.Lock()
		tm.isLoaded = true
		tm.lastUpdated = time.Now()
		tm.mu.Unlock()
	}()
}

func (tm *TokenMaster) downloadAndParseCSV(url string, segment string) (int, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("http error %d fetching %s", resp.StatusCode, url)
	}

	reader := csv.NewReader(resp.Body)
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1 // Allow variable number of columns

	count := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(record) < 3 {
			continue
		}

		// Fyers CSV Token Master columns: [FyToken, SymbolDetails, ExchangeToken, LotSize, TickSize, ISIN, ...]
		fyToken := strings.TrimSpace(record[0])
		symbol := strings.TrimSpace(record[1])

		if fyToken == "" || symbol == "" {
			continue
		}

		lotSize := 1
		if len(record) > 3 {
			if l, e := strconv.Atoi(strings.TrimSpace(record[3])); e == nil && l > 0 {
				lotSize = l
			}
		}

		tickSize := 0.05
		if len(record) > 4 {
			if t, e := strconv.ParseFloat(strings.TrimSpace(record[4]), 64); e == nil && t > 0 {
				tickSize = t
			}
		}

		desc := symbol
		if len(record) > 2 {
			desc = strings.TrimSpace(record[2])
		}

		rec := &SymbolMasterRecord{
			FyToken:     fyToken,
			Symbol:      symbol,
			Description: desc,
			Exchange:    "NSE",
			LotSize:     lotSize,
			TickSize:    tickSize,
			Segment:     segment,
		}

		tm.mu.Lock()
		tm.bySymbol[symbol] = rec
		tm.byFyToken[fyToken] = rec
		tm.mu.Unlock()

		count++
	}

	return count, nil
}
