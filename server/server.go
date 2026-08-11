package server

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"time"

	"github.com/algoengine/trading-system/config"
	"github.com/algoengine/trading-system/datamanager"
	"github.com/algoengine/trading-system/fyers"
	"github.com/algoengine/trading-system/state"
	"github.com/algoengine/trading-system/ws"
)

type Server struct {
	cfg       *config.Config
	state     *state.SystemState
	hub       *ws.Hub
	collector *datamanager.Collector
	embedFS   embed.FS
}

func NewServer(cfg *config.Config, sysState *state.SystemState, hub *ws.Hub, collector *datamanager.Collector, embedFS embed.FS) *Server {
	return &Server{
		cfg:       cfg,
		state:     sysState,
		hub:       hub,
		collector: collector,
		embedFS:   embedFS,
	}
}

func (s *Server) SetupRoutes() http.Handler {
	mux := http.NewServeMux()

	// WebSocket endpoint
	mux.HandleFunc("/ws", s.hub.ServeHTTP)

	// REST API endpoints
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/positions", s.handlePositions)
	mux.HandleFunc("/api/metrics", s.handleMetrics)
	mux.HandleFunc("/api/candles", s.handleCandles)
	mux.HandleFunc("/api/strategy/toggle", s.handleToggleStrategy)

	// Fyers Credentials & Settings endpoints
	mux.HandleFunc("/api/settings", s.handleSettings)
	mux.HandleFunc("/api/fyers/auth-url", s.handleAuthURL)
	mux.HandleFunc("/api/fyers/validate-code", s.handleValidateCode)
	mux.HandleFunc("/api/fyers/callback", s.handleCallback)
	mux.HandleFunc("/api/fyers/logout", s.handleLogout)

	// Data Manager endpoints
	mux.HandleFunc("/api/datamanager/settings", s.handleDataSettings)
	mux.HandleFunc("/api/datamanager/summary", s.handleDataSummary)
	mux.HandleFunc("/api/datamanager/sync", s.handleDataSync)
	mux.HandleFunc("/api/datamanager/redownload", s.handleDataRedownload)
	mux.HandleFunc("/api/datamanager/clear", s.handleDataClear)

	// Serve embedded web directory for root and static assets
	webSubFS, err := fs.Sub(s.embedFS, "web")
	if err != nil {
		mux.Handle("/", http.NotFoundHandler())
	} else {
		fileServer := http.FileServer(http.FS(webSubFS))
		mux.Handle("/", fileServer)
	}

	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	appID, _, _, _, authCode, accessToken, isAuthenticated := s.cfg.GetCredentials()
	uptime := time.Since(s.state.StartTime).Round(time.Second).String()

	res := map[string]interface{}{
		"status":          "ok",
		"uptime":          uptime,
		"algo_running":    s.state.GetAlgoRunning(),
		"authenticated":   isAuthenticated,
		"fyers_app_id":    appID,
		"has_access_token": accessToken != "",
		"has_auth_code":   authCode != "",
		"timestamp":       time.Now().Unix(),
	}
	sendJSON(w, http.StatusOK, res)
}

func (s *Server) handlePositions(w http.ResponseWriter, r *http.Request) {
	positions := s.state.GetPositions()
	sendJSON(w, http.StatusOK, positions)
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := s.state.GetMetrics()
	sendJSON(w, http.StatusOK, metrics)
}

func (s *Server) handleCandles(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.IsAuthenticated() {
		sendJSON(w, http.StatusUnauthorized, []interface{}{})
		return
	}

	symbol := r.URL.Query().Get("symbol")
	period := r.URL.Query().Get("period")
	if symbol == "" {
		symbol = "NSE:ITC-EQ"
	}
	if period == "" {
		period = "15m"
	}

	// 1. Try loading candles directly from local disk storage first (for 15m/1h historical data)
	diskCandles := s.collector.GetStorageManager().LoadCandles(symbol, period)
	if len(diskCandles) > 0 {
		sendJSON(w, http.StatusOK, diskCandles)
		return
	}

	// 2. Fallback to state memory candles (for 1m/5m streaming candles)
	candles := s.state.GetCandles(symbol, period)
	sendJSON(w, http.StatusOK, candles)
}

func (s *Server) handleToggleStrategy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	current := s.state.GetAlgoRunning()
	newState := !current
	s.state.SetAlgoRunning(newState)

	statusStr := "STOPPED"
	if newState {
		statusStr = "RUNNING"
	}

	logMsg := "Strategy engine status changed to: " + statusStr
	s.state.AddLog(logMsg)
	s.hub.BroadcastEvent("system_log", logMsg)

	res := map[string]interface{}{
		"algo_running": newState,
		"message":      logMsg,
	}
	sendJSON(w, http.StatusOK, res)
}

type SettingsPayload struct {
	FyersAppID       string `json:"fyers_app_id"`
	FyersSecretKey   string `json:"fyers_secret_key"`
	FyersRedirectURI string `json:"fyers_redirect_uri"`
	FyersPin         string `json:"fyers_pin"`
	FyersAccessToken string `json:"fyers_access_token"`
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		appID, secretKey, redirectURI, pin, authCode, accessToken, isAuthenticated := s.cfg.GetCredentials()
		res := map[string]interface{}{
			"fyers_app_id":       appID,
			"fyers_secret_key":   secretKey,
			"fyers_redirect_uri": redirectURI,
			"fyers_pin":          pin,
			"fyers_auth_code":    authCode,
			"fyers_access_token": accessToken,
			"authenticated":      isAuthenticated,
		}
		sendJSON(w, http.StatusOK, res)
		return
	}

	if r.Method == http.MethodPost {
		var req SettingsPayload
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		s.cfg.UpdateCredentials(req.FyersAppID, req.FyersSecretKey, req.FyersRedirectURI, req.FyersPin, req.FyersAccessToken)

		s.state.AddLog("Updated Fyers API Credentials")
		s.hub.BroadcastEvent("system_log", "Updated Fyers API credentials")

		res := map[string]interface{}{
			"status":        "success",
			"message":       "Fyers API credentials saved successfully",
			"authenticated": s.cfg.IsAuthenticated(),
		}
		sendJSON(w, http.StatusOK, res)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) handleAuthURL(w http.ResponseWriter, r *http.Request) {
	appID, _, redirectURI, _, _, _, _ := s.cfg.GetCredentials()
	if appID == "" {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": "Please set Fyers App ID first"})
		return
	}

	authURL := fyers.GenerateAuthURL(appID, redirectURI)
	sendJSON(w, http.StatusOK, map[string]string{"auth_url": authURL})
}

type ValidateCodePayload struct {
	AuthCode string `json:"auth_code"`
}

func (s *Server) handleValidateCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ValidateCodePayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AuthCode == "" {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": "Auth code is required"})
		return
	}

	appID, secretKey, redirectURI, pin, _, _, _ := s.cfg.GetCredentials()
	if appID == "" || secretKey == "" {
		sendJSON(w, http.StatusBadRequest, map[string]string{"error": "Fyers App ID and Secret Key must be set in settings"})
		return
	}

	token, err := fyers.ValidateAuthCode(appID, secretKey, req.AuthCode)
	if err != nil {
		sendJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.cfg.UpdateCredentials(appID, secretKey, redirectURI, pin, token)
	s.cfg.UpdateAuthToken(req.AuthCode, token)

	logMsg := "Fyers API v3 OAuth Login Verified! Live Access Token set."
	s.state.AddLog(logMsg)
	s.hub.BroadcastEvent("system_log", logMsg)

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"status":       "success",
		"access_token": token,
		"message":      logMsg,
	})
}

func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("auth_code")
	if code == "" {
		code = r.URL.Query().Get("code")
	}

	if code == "" {
		http.Redirect(w, r, "/?login=error&message=No+auth+code+in+callback", http.StatusFound)
		return
	}

	appID, secretKey, redirectURI, pin, _, _, _ := s.cfg.GetCredentials()
	if appID == "" || secretKey == "" {
		http.Redirect(w, r, "/?login=error&message=Missing+App+ID+or+Secret+Key", http.StatusFound)
		return
	}

	token, err := fyers.ValidateAuthCode(appID, secretKey, code)
	if err != nil {
		http.Redirect(w, r, "/?login=error&message="+err.Error(), http.StatusFound)
		return
	}

	s.cfg.UpdateCredentials(appID, secretKey, redirectURI, pin, token)
	s.cfg.UpdateAuthToken(code, token)

	logMsg := "Fyers OAuth callback received auth_code & validated Access Token automatically!"
	s.state.AddLog(logMsg)
	s.hub.BroadcastEvent("system_log", logMsg)

	http.Redirect(w, r, "/?login=success", http.StatusFound)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.cfg.ClearAuth()
	s.state.ClearSessionData()

	logMsg := "User logged out of Fyers API session"
	s.state.AddLog(logMsg)
	s.hub.BroadcastEvent("system_log", logMsg)

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": logMsg,
	})
}

// Data Manager REST Handlers
type DataSettingsPayload struct {
	StockListCSV string `json:"stock_list_csv"`
	Interval     string `json:"interval"`
}

func (s *Server) handleDataSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		sendJSON(w, http.StatusOK, s.collector.GetDataConfig().GetConfigSummary())
		return
	}

	if r.Method == http.MethodPost {
		var req DataSettingsPayload
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		s.collector.GetDataConfig().Update(req.StockListCSV, req.Interval)

		// Trigger sync for updated stock list asynchronously
		go s.collector.SyncLatestData()

		sendJSON(w, http.StatusOK, map[string]interface{}{
			"status":   "success",
			"message":  "Updated stock list configuration and initiated data sync.",
			"settings": s.collector.GetDataConfig().GetConfigSummary(),
		})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) handleDataSummary(w http.ResponseWriter, r *http.Request) {
	summary := s.collector.GetSummary()
	sendJSON(w, http.StatusOK, summary)
}

func (s *Server) handleDataSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	go func() {
		_, err := s.collector.SyncLatestData()
		if err != nil {
			s.state.AddLog("Sync error: " + err.Error())
		}
	}()

	sendJSON(w, http.StatusOK, map[string]string{
		"status":  "started",
		"message": "Data sync initiated in background.",
	})
}

func (s *Server) handleDataRedownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	go func() {
		_, err := s.collector.RedownloadFullData()
		if err != nil {
			s.state.AddLog("Redownload error: " + err.Error())
		}
	}()

	sendJSON(w, http.StatusOK, map[string]string{
		"status":  "started",
		"message": "Full 90-day re-download initiated in background.",
	})
}

func (s *Server) handleDataClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := s.collector.ClearAllData()
	if err != nil {
		sendJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	sendJSON(w, http.StatusOK, map[string]string{
		"status":  "success",
		"message": "Cleared all stored historical candle data files.",
	})
}

func sendJSON(w http.ResponseWriter, statusCode int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(payload)
}
