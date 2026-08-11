package fyers

import (
	"context"
	"log"
	"sync"
)

type DataStream struct {
	symbols     []string
	appID       string
	accessToken string
	tickChan    chan *Tick
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

func NewDataStream(symbols []string, appID, accessToken string) *DataStream {
	ctx, cancel := context.WithCancel(context.Background())
	return &DataStream{
		symbols:     symbols,
		appID:       appID,
		accessToken: accessToken,
		tickChan:    make(chan *Tick, 1000),
		ctx:         ctx,
		cancel:      cancel,
	}
}

func (ds *DataStream) GetTickChan() <-chan *Tick {
	return ds.tickChan
}

func (ds *DataStream) Start() {
	if ds.appID == "" || ds.accessToken == "" {
		log.Println("[DataStream] Fyers credentials missing. Real-time data stream is dormant until login.")
		return
	}
	ds.startRealWebSocket()
}

func (ds *DataStream) Stop() {
	ds.cancel()
	ds.wg.Wait()
	close(ds.tickChan)
}

func (ds *DataStream) startRealWebSocket() {
	log.Printf("[DataStream] Connecting to Fyers v3 Live Data Socket for symbols: %v...", ds.symbols)
	// Production Fyers v3 Data Socket connection:
	// Endpoint: wss://api-t1.fyers.in/data-ws/v3/webSocket?access_token=APP_ID:ACCESS_TOKEN
}
