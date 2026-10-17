package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/closeutil"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/version"
)

// CryptoFetcher fetches cryptocurrency prices from CoinGecko API
const maximumCoinGeckoResponseBytes = 2 << 20

// CryptoFetcher fetches cryptocurrency data from CoinGecko.
type CryptoFetcher struct {
	client *http.Client
	config *config.CryptoWidgetConfig
}

// CryptoData represents crypto widget data
type CryptoData struct {
	Coins []CoinData `json:"coins"`
}

// CoinData represents data for a single cryptocurrency
type CoinData struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Symbol    string    `json:"symbol"`
	Currency  string    `json:"currency"`
	Amount    float64   `json:"amount,omitempty"`
	Price     float64   `json:"price"`
	Change24h float64   `json:"change_24h"`
	MarketCap float64   `json:"market_cap,omitempty"`
	Volume24h float64   `json:"volume_24h,omitempty"`
	Chart     []float64 `json:"chart,omitempty"`
}

// CoinGeckoResponse represents CoinGecko API response
type CoinGeckoResponse map[string]struct {
	USD          float64 `json:"usd"`
	EUR          float64 `json:"eur"`
	GBP          float64 `json:"gbp"`
	USDChange24h float64 `json:"usd_24h_change"`
	EURChange24h float64 `json:"eur_24h_change"`
	GBPChange24h float64 `json:"gbp_24h_change"`
	USDMarketCap float64 `json:"usd_market_cap"`
	EURMarketCap float64 `json:"eur_market_cap"`
	GBPMarketCap float64 `json:"gbp_market_cap"`
	USDVolume24h float64 `json:"usd_24h_vol"`
	EURVolume24h float64 `json:"eur_24h_vol"`
	GBPVolume24h float64 `json:"gbp_24h_vol"`
}

// NewCryptoFetcher creates a new crypto fetcher
func NewCryptoFetcher(cfg *config.CryptoWidgetConfig) *CryptoFetcher {
	return &CryptoFetcher{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		config: cfg,
	}
}

// WidgetType returns the widget type
func (f *CryptoFetcher) WidgetType() WidgetType {
	return WidgetCrypto
}

// CacheDuration returns how long to cache the data
func (f *CryptoFetcher) CacheDuration() time.Duration {
	return 5 * time.Minute
}

// Fetch fetches crypto prices
func (f *CryptoFetcher) Fetch(ctx context.Context, params map[string]string) (*WidgetData, error) {
	// Get coins from params or config
	coinsStr := params["coins"]
	var coins []string
	if coinsStr != "" {
		coins = strings.Split(coinsStr, ",")
		for i := range coins {
			coins[i] = strings.TrimSpace(strings.ToLower(coins[i]))
		}
	} else {
		coins = f.config.DefaultCoins
	}

	if len(coins) == 0 {
		coins = []string{"bitcoin", "ethereum"}
	}

	// Get currency
	currency := params["currency"]
	if currency == "" {
		currency = f.config.Currency
	}
	if currency == "" {
		currency = "usd"
	}
	switch strings.ToLower(currency) {
	case "eur", "gbp", "usd":
		currency = strings.ToLower(currency)
	default:
		currency = "usd"
	}

	amount := 1.0
	if rawAmount := strings.TrimSpace(params["amount"]); rawAmount != "" {
		parsedAmount, err := strconv.ParseFloat(rawAmount, 64)
		if err != nil || parsedAmount <= 0 || parsedAmount > 1e15 {
			return nil, fmt.Errorf("invalid cryptocurrency amount")
		}
		amount = parsedAmount
	}

	// Fetch from CoinGecko
	coinIDs := url.QueryEscape(strings.Join(coins, ","))
	apiURL := fmt.Sprintf("https://api.coingecko.com/api/v3/simple/price?ids=%s&vs_currencies=%s&include_24hr_change=true&include_market_cap=true&include_24hr_vol=true",
		coinIDs, currency)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return &WidgetData{
			Type:      WidgetCrypto,
			Error:     err.Error(),
			UpdatedAt: time.Now(),
		}, nil
	}

	// Set user agent to avoid rate limiting
	req.Header.Set("User-Agent", version.BrowserUserAgent)

	resp, err := f.client.Do(req)
	if err != nil {
		return &WidgetData{
			Type:      WidgetCrypto,
			Error:     err.Error(),
			UpdatedAt: time.Now(),
		}, nil
	}
	defer closeutil.Close(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return &WidgetData{
			Type:      WidgetCrypto,
			Error:     fmt.Sprintf("API returned status %d", resp.StatusCode),
			UpdatedAt: time.Now(),
		}, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maximumCoinGeckoResponseBytes+1))
	if err != nil {
		return &WidgetData{Type: WidgetCrypto, Error: err.Error(), UpdatedAt: time.Now()}, nil
	}
	if len(body) > maximumCoinGeckoResponseBytes {
		return &WidgetData{Type: WidgetCrypto, Error: "cryptocurrency response exceeds the size limit", UpdatedAt: time.Now()}, nil
	}
	var cgResp CoinGeckoResponse
	if err := json.Unmarshal(body, &cgResp); err != nil {
		return &WidgetData{Type: WidgetCrypto, Error: err.Error(), UpdatedAt: time.Now()}, nil
	}

	// Convert response to our format
	cryptoData := &CryptoData{
		Coins: make([]CoinData, 0, len(coins)),
	}

	for _, coinID := range coins {
		if data, ok := cgResp[coinID]; ok {
			var price, change, marketCap, volume float64

			switch strings.ToLower(currency) {
			case "eur":
				price = data.EUR
				change = data.EURChange24h
				marketCap = data.EURMarketCap
				volume = data.EURVolume24h
			case "gbp":
				price = data.GBP
				change = data.GBPChange24h
				marketCap = data.GBPMarketCap
				volume = data.GBPVolume24h
			// usd
			default:
				price = data.USD
				change = data.USDChange24h
				marketCap = data.USDMarketCap
				volume = data.USDVolume24h
			}

			coinData := CoinData{
				ID:        coinID,
				Name:      formatCoinName(coinID),
				Symbol:    coinIDToSymbol(coinID),
				Currency:  strings.ToUpper(currency),
				Amount:    amount,
				Price:     price,
				Change24h: change,
				MarketCap: marketCap,
				Volume24h: volume,
			}
			if params["include_chart"] == "true" {
				coinData.Chart = f.fetchPriceChart(ctx, coinID, currency)
			}
			cryptoData.Coins = append(cryptoData.Coins, coinData)
		}
	}

	return &WidgetData{
		Type:      WidgetCrypto,
		Data:      cryptoData,
		UpdatedAt: time.Now(),
	}, nil
}

func (f *CryptoFetcher) fetchPriceChart(ctx context.Context, coinID, currency string) []float64 {
	query := url.Values{"vs_currency": {currency}, "days": {"1"}}
	endpoint := "https://api.coingecko.com/api/v3/coins/" + url.PathEscape(coinID) + "/market_chart?" + query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil
	}
	request.Header.Set("User-Agent", version.BrowserUserAgent)
	response, err := f.client.Do(request)
	if err != nil {
		return nil
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			log.Printf("failed to close CoinGecko chart response: %v", err)
		}
	}()
	if response.StatusCode != http.StatusOK {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumCoinGeckoResponseBytes+1))
	if err != nil || len(body) > maximumCoinGeckoResponseBytes {
		return nil
	}
	var payload struct {
		Prices [][]float64 `json:"prices"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return nil
	}
	points := make([]float64, 0, len(payload.Prices))
	for _, point := range payload.Prices {
		if len(point) < 2 || math.IsNaN(point[1]) || math.IsInf(point[1], 0) || point[1] <= 0 {
			continue
		}
		points = append(points, point[1])
	}
	if len(points) > 288 {
		points = points[len(points)-288:]
	}
	return points
}

// formatCoinName converts coin ID to display name
func formatCoinName(id string) string {
	names := map[string]string{
		"bitcoin":     "Bitcoin",
		"ethereum":    "Ethereum",
		"tether":      "Tether",
		"binancecoin": "BNB",
		"ripple":      "XRP",
		"usd-coin":    "USD Coin",
		"solana":      "Solana",
		"cardano":     "Cardano",
		"dogecoin":    "Dogecoin",
		"polkadot":    "Polkadot",
		"shiba-inu":   "Shiba Inu",
		"litecoin":    "Litecoin",
		"avalanche-2": "Avalanche",
		"chainlink":   "Chainlink",
		"stellar":     "Stellar",
		"monero":      "Monero",
		"algorand":    "Algorand",
	}
	if name, ok := names[id]; ok {
		return name
	}
	// Capitalize first letter
	if len(id) > 0 {
		return strings.ToUpper(id[:1]) + id[1:]
	}
	return id
}

// coinIDToSymbol converts coin ID to symbol
func coinIDToSymbol(id string) string {
	symbols := map[string]string{
		"bitcoin":     "BTC",
		"ethereum":    "ETH",
		"tether":      "USDT",
		"binancecoin": "BNB",
		"ripple":      "XRP",
		"usd-coin":    "USDC",
		"solana":      "SOL",
		"cardano":     "ADA",
		"dogecoin":    "DOGE",
		"polkadot":    "DOT",
		"shiba-inu":   "SHIB",
		"litecoin":    "LTC",
		"avalanche-2": "AVAX",
		"chainlink":   "LINK",
		"stellar":     "XLM",
		"monero":      "XMR",
		"algorand":    "ALGO",
	}
	if symbol, ok := symbols[id]; ok {
		return symbol
	}
	return strings.ToUpper(id)
}
