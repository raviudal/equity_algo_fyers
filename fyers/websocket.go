package fyers

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type DataStream struct {
	mu          sync.Mutex
	symbols     []string
	appID       string
	accessToken string
	tickChan    chan *Tick
	conn        *tls.Conn
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	isConnected bool
}

func NewDataStream(symbols []string, appID, accessToken string) *DataStream {
	ctx, cancel := context.WithCancel(context.Background())
	return &DataStream{
		symbols:     symbols,
		appID:       appID,
		accessToken: accessToken,
		tickChan:    make(chan *Tick, 5000),
		ctx:         ctx,
		cancel:      cancel,
	}
}

func (ds *DataStream) GetTickChan() <-chan *Tick {
	return ds.tickChan
}

func (ds *DataStream) Start() {
	ds.mu.Lock()
	if ds.appID == "" || ds.accessToken == "" {
		ds.mu.Unlock()
		log.Println("[DataStream] Fyers credentials missing. Real-time data socket is dormant until login.")
		return
	}
	ds.mu.Unlock()

	ds.wg.Add(1)
	go ds.runSocketLoop()
}

func (ds *DataStream) UpdateCredentialsAndSymbols(appID, token string, symbols []string) {
	ds.mu.Lock()
	ds.appID = appID
	ds.accessToken = token
	ds.symbols = symbols
	conn := ds.conn
	ds.mu.Unlock()

	if conn != nil {
		conn.Close() // Force reconnect with new credentials/symbols
	}
}

func (ds *DataStream) Stop() {
	ds.cancel()
	ds.mu.Lock()
	if ds.conn != nil {
		ds.conn.Close()
	}
	ds.mu.Unlock()
	ds.wg.Wait()
}

func (ds *DataStream) runSocketLoop() {
	defer ds.wg.Done()

	backoff := 1 * time.Second

	for {
		select {
		case <-ds.ctx.Done():
			return
		default:
		}

		err := ds.connectAndStream()
		if err != nil {
			log.Printf("[DataStream Warning] Socket disconnected: %v. Reconnecting in %v...", err, backoff)
			select {
			case <-ds.ctx.Done():
				return
			case <-time.After(backoff):
				backoff *= 2
				if backoff > 15*time.Second {
					backoff = 15 * time.Second
				}
			}
		} else {
			backoff = 1 * time.Second
		}
	}
}

func (ds *DataStream) connectAndStream() error {
	ds.mu.Lock()
	appID := ds.appID
	token := ds.accessToken
	symbols := ds.symbols
	ds.mu.Unlock()

	if appID == "" || token == "" {
		return fmt.Errorf("missing credentials")
	}

	wsHost := "api-t1.fyers.in"
	wsPort := "443"
	authParam := fmt.Sprintf("%s:%s", appID, token)
	targetURI := fmt.Sprintf("/data-ws/v3/webSocket?access_token=%s", url.QueryEscape(authParam))

	dialer := &tls.Dialer{
		Config: &tls.Config{InsecureSkipVerify: false},
	}

	log.Printf("[DataStream] Connecting to Fyers v3 Data Socket (%s)...", wsHost)
	conn, err := dialer.DialContext(ds.ctx, "tcp", wsHost+":"+wsPort)
	if err != nil {
		return fmt.Errorf("TLS dial failed: %w", err)
	}

	ds.mu.Lock()
	ds.conn = conn
	ds.isConnected = true
	ds.mu.Unlock()

	defer func() {
		conn.Close()
		ds.mu.Lock()
		ds.conn = nil
		ds.isConnected = false
		ds.mu.Unlock()
	}()

	// RFC 6455 Handshake
	secKey := generateWSKey()
	reqBuf := fmt.Sprintf(
		"GET %s HTTP/1.1\r\n"+
			"Host: %s\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Key: %s\r\n"+
			"Sec-WebSocket-Version: 13\r\n"+
			"User-Agent: AlgoEngine-Go/1.0\r\n\r\n",
		targetURI, wsHost, secKey,
	)

	_, err = conn.Write([]byte(reqBuf))
	if err != nil {
		return fmt.Errorf("handshake write error: %w", err)
	}

	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		return fmt.Errorf("handshake response error: %w", err)
	}

	if resp.StatusCode != 101 {
		return fmt.Errorf("handshake rejected status %d: %s", resp.StatusCode, resp.Status)
	}

	log.Printf("[DataStream] Handshake successful! Subscribing to %d symbols...", len(symbols))

	// Send subscription frame
	if len(symbols) > 0 {
		subPayload := map[string]interface{}{
			"symbol":   symbols,
			"dataType": "symbolUpdate",
		}
		subBytes, _ := json.Marshal(subPayload)
		if err := writeWSFrame(conn, 0x1, subBytes); err != nil {
			return fmt.Errorf("subscription write error: %w", err)
		}
	}

	// Start ping routine (heartbeat every 10s)
	pingCtx, pingCancel := context.WithCancel(ds.ctx)
	defer pingCancel()

	go func() {
		pingTicker := time.NewTicker(10 * time.Second)
		defer pingTicker.Stop()

		for {
			select {
			case <-pingCtx.Done():
				return
			case <-pingTicker.C:
				pingMsg := []byte(`{"ping":"pong"}`)
				ds.mu.Lock()
				c := ds.conn
				ds.mu.Unlock()
				if c != nil {
					writeWSFrame(c, 0x1, pingMsg)
				}
			}
		}
	}()

	// Incoming WS frame loop
	for {
		select {
		case <-ds.ctx.Done():
			return nil
		default:
		}

		opcode, payload, err := readWSFrame(reader)
		if err != nil {
			return fmt.Errorf("read frame error: %w", err)
		}

		if opcode == 0x8 { // Connection close
			return fmt.Errorf("remote closed connection")
		}

		if opcode == 0x1 || opcode == 0x2 { // Text or Binary Frame
			ds.parseAndEmitTick(payload)
		}
	}
}

func (ds *DataStream) parseAndEmitTick(payload []byte) {
	if len(payload) == 0 {
		return
	}

	// 1. Try parsing JSON format
	if payload[0] == '{' {
		var raw map[string]interface{}
		if err := json.Unmarshal(payload, &raw); err == nil {
			sym, okSym := raw["symbol"].(string)
			if !okSym {
				sym, okSym = raw["n"].(string) // name / symbol alternative
			}

			if okSym && sym != "" {
				ltp := extractFloat(raw, "ltp", "lp", "price", "v.lp")
				vol := int64(extractFloat(raw, "vol_traded_today", "v", "volume"))
				ts := int64(extractFloat(raw, "last_traded_time", "t", "time", "tt"))

				if ts == 0 {
					ts = time.Now().Unix()
				}

				if ltp > 0 {
					tick := &Tick{
						Symbol:    sym,
						LastPrice: ltp,
						Volume:    vol,
						Timestamp: ts,
					}
					select {
					case ds.tickChan <- tick:
					default:
					}
				}
			}
		}
		return
	}

	// 2. Try parsing Binary tick packet layout
	if len(payload) >= 32 {
		// Fyers v3 binary tick packet
		topicID := binary.BigEndian.Uint32(payload[0:4])
		if topicID > 0 || len(payload) >= 48 {
			ts := int64(binary.BigEndian.Uint64(payload[4:12]))
			ltpBits := binary.BigEndian.Uint64(payload[12:20])
			ltp := float64(ltpBits)

			if ltp > 0 && ltp < 1000000 { // Basic sanity check
				if ts == 0 {
					ts = time.Now().Unix()
				}
				// Use active current symbol fallback if symbol is in topic header
				ds.mu.Lock()
				syms := ds.symbols
				ds.mu.Unlock()

				sym := "NSE:ITC-EQ"
				if len(syms) > 0 {
					sym = syms[0]
				}

				tick := &Tick{
					Symbol:    sym,
					LastPrice: ltp,
					Volume:    100,
					Timestamp: ts,
				}
				select {
				case ds.tickChan <- tick:
				default:
				}
			}
		}
	}
}

func extractFloat(m map[string]interface{}, keys ...string) float64 {
	for _, k := range keys {
		if val, exists := m[k]; exists && val != nil {
			switch v := val.(type) {
			case float64:
				return v
			case int64:
				return float64(v)
			case int:
				return float64(v)
			case string:
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					return f
				}
			}
		}
	}
	return 0
}

func generateWSKey() string {
	p := make([]byte, 16)
	rand.Read(p)
	return base64.StdEncoding.EncodeToString(p)
}

func writeWSFrame(w io.Writer, opcode byte, payload []byte) error {
	var header bytes.Buffer
	header.WriteByte(0x80 | (opcode & 0x0F)) // FIN = 1

	length := len(payload)
	if length <= 125 {
		header.WriteByte(0x80 | byte(length)) // Mask = 1
	} else if length <= 65535 {
		header.WriteByte(0x80 | 126)
		binary.Write(&header, binary.BigEndian, uint16(length))
	} else {
		header.WriteByte(0x80 | 127)
		binary.Write(&header, binary.BigEndian, uint64(length))
	}

	// Client frames MUST be masked according to RFC 6455
	maskKey := make([]byte, 4)
	rand.Read(maskKey)
	header.Write(maskKey)

	maskedPayload := make([]byte, length)
	for i := 0; i < length; i++ {
		maskedPayload[i] = payload[i] ^ maskKey[i%4]
	}

	if _, err := w.Write(header.Bytes()); err != nil {
		return err
	}
	_, err := w.Write(maskedPayload)
	return err
}

func readWSFrame(r *bufio.Reader) (byte, []byte, error) {
	b1, err := r.ReadByte()
	if err != nil {
		return 0, nil, err
	}

	opcode := b1 & 0x0F

	b2, err := r.ReadByte()
	if err != nil {
		return 0, nil, err
	}

	isMasked := (b2 & 0x80) != 0
	length := int64(b2 & 0x7F)

	if length == 126 {
		var l uint16
		if err := binary.Read(r, binary.BigEndian, &l); err != nil {
			return 0, nil, err
		}
		length = int64(l)
	} else if length == 127 {
		var l uint64
		if err := binary.Read(r, binary.BigEndian, &l); err != nil {
			return 0, nil, err
		}
		length = int64(l)
	}

	var maskKey [4]byte
	if isMasked {
		if _, err := io.ReadFull(r, maskKey[:]); err != nil {
			return 0, nil, err
		}
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}

	if isMasked {
		for i := 0; i < len(payload); i++ {
			payload[i] ^= maskKey[i%4]
		}
	}

	return opcode, payload, nil
}
