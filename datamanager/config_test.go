package datamanager

import (
	"testing"
)

func TestSymbolNormalization(t *testing.T) {
	tests := []struct {
		raw      string
		expected string
	}{
		{"itc", "NSE:ITC-EQ"},
		{"ITC", "NSE:ITC-EQ"},
		{"ITC-EQ", "NSE:ITC-EQ"},
		{"NSE:ITC-EQ", "NSE:ITC-EQ"},
		{"reliance", "NSE:RELIANCE-EQ"},
		{" sbin ", "NSE:SBIN-EQ"},
	}

	for _, tt := range tests {
		actual := NormalizeSymbol(tt.raw)
		if actual != tt.expected {
			t.Errorf("NormalizeSymbol(%q) = %q, expected %q", tt.raw, actual, tt.expected)
		}
	}
}

func TestDataConfigNormalizedSymbols(t *testing.T) {
	cfg := DefaultDataConfig()
	cfg.StockListCSV = "itc, RELIANCE-EQ, NSE:SBIN-EQ, tcs"

	symbols := cfg.GetNormalizedSymbols()
	if len(symbols) != 4 {
		t.Fatalf("Expected 4 symbols, got %d", len(symbols))
	}

	expected := []string{"NSE:ITC-EQ", "NSE:RELIANCE-EQ", "NSE:SBIN-EQ", "NSE:TCS-EQ"}
	for i, s := range symbols {
		if s != expected[i] {
			t.Errorf("Symbol[%d] = %q, expected %q", i, s, expected[i])
		}
	}
}
