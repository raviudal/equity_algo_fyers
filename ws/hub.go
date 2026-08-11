package ws

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const magicGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

type Event struct {
	Type      string      `json:"type"` // candle_update, trade_execution, metrics_update, system_log
	Data      interface{} `json:"data"`
	Timestamp int64       `json:"timestamp"`
}

type Client struct {
	hub  *Hub
	conn net.Conn
	send chan []byte
}

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 500),
		register:   make(chan *Client, 50),
		unregister: make(chan *Client, 50),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				client.conn.Close()
			}
			h.mu.Unlock()
		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// Non-blocking drop if client buffer is full to prevent memory leak
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *Hub) BroadcastEvent(eventType string, data interface{}) {
	event := Event{
		Type:      eventType,
		Data:      data,
		Timestamp: time.Now().Unix(),
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return
	}

	select {
	case h.broadcast <- payload:
	default:
		// Non-blocking drop if hub broadcast queue is full
	}
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "Not a websocket handshake", http.StatusBadRequest)
		return
	}

	secKey := r.Header.Get("Sec-WebSocket-Key")
	if secKey == "" {
		http.Error(w, "Missing Sec-WebSocket-Key", http.StatusBadRequest)
		return
	}

	// Compute Sec-WebSocket-Accept key according to RFC 6455
	hSha := sha1.New()
	hSha.Write([]byte(secKey + magicGUID))
	acceptKey := base64.StdEncoding.EncodeToString(hSha.Sum(nil))

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Webserver doesn't support hijacking", http.StatusInternalServerError)
		return
	}

	conn, bufrw, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Send HTTP 101 Switching Protocols response
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + acceptKey + "\r\n\r\n"

	bufrw.WriteString(resp)
	bufrw.Flush()

	client := &Client{
		hub:  h,
		conn: conn,
		send: make(chan []byte, 256),
	}

	h.register <- client

	go client.writePump()
	go client.readPump(bufrw.Reader)
}

func (c *Client) readPump(r *bufio.Reader) {
	defer func() {
		c.hub.unregister <- c
	}()

	for {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		header := make([]byte, 2)
		_, err := io.ReadFull(r, header)
		if err != nil {
			break
		}

		opcode := header[0] & 0x0F
		if opcode == 8 { // Close frame
			break
		}

		payloadLen := int(header[1] & 0x7F)
		masked := (header[1] & 0x80) != 0

		if payloadLen == 126 {
			extended := make([]byte, 2)
			io.ReadFull(r, extended)
			payloadLen = int(extended[0])<<8 | int(extended[1])
		} else if payloadLen == 127 {
			extended := make([]byte, 8)
			io.ReadFull(r, extended)
			payloadLen = int(extended[7])
		}

		maskKey := make([]byte, 4)
		if masked {
			io.ReadFull(r, maskKey)
		}

		payload := make([]byte, payloadLen)
		if payloadLen > 0 {
			io.ReadFull(r, payload)
			if masked {
				for i := 0; i < payloadLen; i++ {
					payload[i] ^= maskKey[i%4]
				}
			}
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(25 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.writeFrame(0x08, []byte{}) // Close frame
				return
			}
			if err := c.writeFrame(0x01, message); err != nil { // Text frame (0x01)
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.writeFrame(0x09, []byte{}); err != nil { // Ping frame (0x09)
				return
			}
		}
	}
}

func (c *Client) writeFrame(opcode byte, data []byte) error {
	length := len(data)
	var header []byte

	if length <= 125 {
		header = []byte{0x80 | opcode, byte(length)}
	} else if length <= 65535 {
		header = []byte{0x80 | opcode, 126, byte(length >> 8), byte(length & 0xFF)}
	} else {
		header = []byte{0x80 | opcode, 127, 0, 0, 0, 0, byte(length >> 24), byte(length >> 16), byte(length >> 8), byte(length & 0xFF)}
	}

	if _, err := c.conn.Write(append(header, data...)); err != nil {
		return err
	}
	return nil
}
