package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
)

type Config struct {
	mu               sync.RWMutex
	FyersAppID       string   `json:"fyers_app_id"`
	FyersSecretKey   string   `json:"fyers_secret_key"`
	FyersRedirectURI string   `json:"fyers_redirect_uri"`
	FyersPin         string   `json:"fyers_pin"`
	FyersAuthCode    string   `json:"fyers_auth_code"`
	FyersAccessToken string   `json:"fyers_access_token"`
	Port             string   `json:"port"`
	Env              string   `json:"env"`
	Symbols          []string `json:"symbols"`
}

func LoadConfig() *Config {
	loadDotEnv(".env")

	appID := getEnv("FYERS_APP_ID", "")
	secretKey := getEnv("FYERS_SECRET_KEY", "")
	redirectURI := getEnv("FYERS_REDIRECT_URI", "http://localhost:8080/api/fyers/callback")
	pin := getEnv("FYERS_PIN", "")
	authCode := getEnv("FYERS_AUTH_CODE", "")
	accessToken := getEnv("FYERS_ACCESS_TOKEN", "")
	port := getEnv("PORT", "8080")
	env := getEnv("ENV", "development")

	symbolsRaw := getEnv("SYMBOLS", "NSE:ITC-EQ,NSE:SBIN-EQ,NSE:NIFTY50-INDEX,NSE:BANKNIFTY-INDEX,NSE:RELIANCE-EQ")
	symbolList := strings.Split(symbolsRaw, ",")
	for i := range symbolList {
		symbolList[i] = strings.TrimSpace(symbolList[i])
	}

	return &Config{
		FyersAppID:       appID,
		FyersSecretKey:   secretKey,
		FyersRedirectURI: redirectURI,
		FyersPin:         pin,
		FyersAuthCode:    authCode,
		FyersAccessToken: accessToken,
		Port:             port,
		Env:              env,
		Symbols:          symbolList,
	}
}

func (c *Config) IsAuthenticated() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.FyersAccessToken != ""
}

func (c *Config) UpdateCredentials(appID, secretKey, redirectURI, pin, accessToken string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.FyersAppID = appID
	c.FyersSecretKey = secretKey
	if redirectURI != "" {
		c.FyersRedirectURI = redirectURI
	}
	c.FyersPin = pin
	c.FyersAccessToken = accessToken

	c.saveDotEnvLocked(".env")
}

func (c *Config) UpdateAuthToken(authCode, accessToken string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.FyersAuthCode = authCode
	c.FyersAccessToken = accessToken
	c.saveDotEnvLocked(".env")
}

func (c *Config) ClearAuth() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.FyersAuthCode = ""
	c.FyersAccessToken = ""
	c.saveDotEnvLocked(".env")
}

func (c *Config) GetCredentials() (appID, secretKey, redirectURI, pin, authCode, accessToken string, isAuthenticated bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.FyersAppID, c.FyersSecretKey, c.FyersRedirectURI, c.FyersPin, c.FyersAuthCode, c.FyersAccessToken, c.FyersAccessToken != ""
}

func (c *Config) saveDotEnvLocked(filepath string) error {
	content := fmt.Sprintf(`# Fyers API v3 Configuration
FYERS_APP_ID=%s
FYERS_SECRET_KEY=%s
FYERS_REDIRECT_URI=%s
FYERS_PIN=%s
FYERS_AUTH_CODE=%s
FYERS_ACCESS_TOKEN=%s

# HTTP Server Configuration
PORT=%s
ENV=%s

# Target Trading Symbols (Comma-separated)
SYMBOLS=%s
`, c.FyersAppID, c.FyersSecretKey, c.FyersRedirectURI, c.FyersPin, c.FyersAuthCode, c.FyersAccessToken,
		c.Port, c.Env, strings.Join(c.Symbols, ","))

	return os.WriteFile(filepath, []byte(content), 0644)
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultVal
}

func loadDotEnv(filepath string) {
	file, err := os.Open(filepath)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			if _, exists := os.LookupEnv(key); !exists {
				os.Setenv(key, val)
			}
		}
	}
}
