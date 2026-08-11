package fyers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	AppID       string
	AccessToken string
	HTTPClient  *http.Client
}

func NewClient(appID, accessToken string) *Client {
	return &Client{
		AppID:       appID,
		AccessToken: accessToken,
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func GenerateAuthURL(appID, redirectURI string) string {
	if redirectURI == "" {
		redirectURI = "http://localhost:8080/api/fyers/callback"
	}
	return fmt.Sprintf("https://api-t1.fyers.in/api/v3/generate-authcode?client_id=%s&redirect_uri=%s&response_type=code&state=sample_state",
		url.QueryEscape(appID), url.QueryEscape(redirectURI))
}

type AuthCodeValidateRequest struct {
	GrantType string `json:"grant_type"`
	AppIdHash string `json:"appIdHash"`
	Code      string `json:"code"`
}

type AuthCodeValidateResponse struct {
	S           string `json:"s"`
	Code        int    `json:"code"`
	Message     string `json:"message"`
	AccessToken string `json:"access_token"`
}

func ValidateAuthCode(appID, secretKey, authCode string) (string, error) {
	// SHA-256 Hash of app_id:secret_key
	hash := sha256.New()
	hash.Write([]byte(fmt.Sprintf("%s:%s", appID, secretKey)))
	appIdHash := hex.EncodeToString(hash.Sum(nil))

	reqBody := AuthCodeValidateRequest{
		GrantType: "authorization_code",
		AppIdHash: appIdHash,
		Code:      authCode,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	resp, err := http.Post("https://api-t1.fyers.in/api/v3/validate-authcode", "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var res AuthCodeValidateResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return "", fmt.Errorf("failed parsing fyers auth response: %v", err)
	}

	if res.S != "ok" || res.AccessToken == "" {
		return "", fmt.Errorf("fyers auth error: %s (code %d)", res.Message, res.Code)
	}

	return res.AccessToken, nil
}

func (c *Client) GetHistoricalData(symbol string, resolution string, fromTime, toTime int64) ([]*Candle, error) {
	if c.AccessToken == "" || c.AppID == "" {
		return nil, fmt.Errorf("Fyers API authentication required: Access token is missing")
	}

	reqURL := fmt.Sprintf("https://api-t1.fyers.in/data/history?symbol=%s&resolution=%s&date_format=0&range_from=%d&range_to=%d&cont_flag=1",
		url.QueryEscape(symbol), resolution, fromTime, toTime)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("%s:%s", c.AppID, c.AccessToken))

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fyers api returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var res FyersHistoricalResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}

	if res.S != "ok" || len(res.Candles) == 0 {
		return nil, fmt.Errorf("no candles returned from fyers api")
	}

	candles := make([]*Candle, 0, len(res.Candles))
	for _, raw := range res.Candles {
		if len(raw) < 6 {
			continue
		}
		ts := int64(raw[0].(float64))
		open := raw[1].(float64)
		high := raw[2].(float64)
		low := raw[3].(float64)
		closePrice := raw[4].(float64)
		volume := int64(raw[5].(float64))

		periodStr := "1m"
		if resolution == "5" {
			periodStr = "5m"
		}

		candles = append(candles, &Candle{
			Symbol:    symbol,
			Timestamp: ts,
			Open:      open,
			High:      high,
			Low:       low,
			Close:     closePrice,
			Volume:    volume,
			IsClosed:  true,
			Period:    periodStr,
			TimeStr:   time.Unix(ts, 0).Format("15:04"),
		})
	}

	return candles, nil
}

func (c *Client) PlaceOrder(order *Order) error {
	if c.AccessToken == "" || c.AppID == "" {
		return fmt.Errorf("Fyers API authentication required: Access token is missing")
	}

	// Fyers v3 Place Order REST Payload
	sideVal := 1
	if order.Side == "SELL" {
		sideVal = -1
	}

	payloadMap := map[string]interface{}{
		"symbol":      order.Symbol,
		"qty":         order.Qty,
		"type":        2, // 1=Limit, 2=Market
		"side":        sideVal,
		"productType": "INTRADAY",
		"limitPrice":  order.LimitPrice,
		"stopPrice":   0,
		"validity":    "DAY",
		"disclosedQty": 0,
		"offlineOrder": false,
	}

	bodyData, _ := json.Marshal(payloadMap)
	req, err := http.NewRequest("POST", "https://api-t1.fyers.in/api/v3/orders/sync", bytes.NewBuffer(bodyData))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", fmt.Sprintf("%s:%s", c.AppID, c.AccessToken))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fyers order api status: %d", resp.StatusCode)
	}

	order.Status = "FILLED"
	order.ExecPrice = order.LimitPrice
	order.Timestamp = time.Now()
	return nil
}
