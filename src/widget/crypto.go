package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/version"
	"github.com/apimgr/search/src/config"
)

// CryptoFetcher fetches cryptocurrency prices from CoinGecko API
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
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Symbol     string  `json:"symbol"`
	Price      float64 `json:"price"`
	Change24h  float64 `json:"change_24h"`
	MarketCap  float64 `json:"market_cap,omitempty"`
	Volume24h  float64 `json:"volume_24h,omitempty"`
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
	USDVolume24h float64 `json:"usd_24h_vol"`
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &WidgetData{
			Type:      WidgetCrypto,
			Error:     fmt.Sprintf("API returned status %d", resp.StatusCode),
			UpdatedAt: time.Now(),
		}, nil
	}

	var cgResp CoinGeckoResponse
	if err := json.NewDecoder(resp.Body).Decode(&cgResp); err != nil {
		return &WidgetData{
			Type:      WidgetCrypto,
			Error:     err.Error(),
			UpdatedAt: time.Now(),
		}, nil
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
			case "gbp":
				price = data.GBP
				change = data.GBPChange24h
			default: // usd
				price = data.USD
				change = data.USDChange24h
				marketCap = data.USDMarketCap
				volume = data.USDVolume24h
			}

			cryptoData.Coins = append(cryptoData.Coins, CoinData{
				ID:        coinID,
				Name:      formatCoinName(coinID),
				Symbol:    coinIDToSymbol(coinID),
				Price:     price,
				Change24h: change,
				MarketCap: marketCap,
				Volume24h: volume,
			})
		}
	}

	return &WidgetData{
		Type:      WidgetCrypto,
		Data:      cryptoData,
		UpdatedAt: time.Now(),
	}, nil
}

// formatCoinName converts coin ID to display name
func formatCoinName(id string) string {
	names := map[string]string{
		"bitcoin":      "Bitcoin",
		"ethereum":     "Ethereum",
		"tether":       "Tether",
		"binancecoin":  "BNB",
		"ripple":       "XRP",
		"usd-coin":     "USD Coin",
		"solana":       "Solana",
		"cardano":      "Cardano",
		"dogecoin":     "Dogecoin",
		"polkadot":     "Polkadot",
		"shiba-inu":    "Shiba Inu",
		"litecoin":     "Litecoin",
		"avalanche-2":  "Avalanche",
		"chainlink":    "Chainlink",
		"stellar":      "Stellar",
		"monero":       "Monero",
		"algorand":     "Algorand",
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
		"bitcoin":      "BTC",
		"ethereum":     "ETH",
		"tether":       "USDT",
		"binancecoin":  "BNB",
		"ripple":       "XRP",
		"usd-coin":     "USDC",
		"solana":       "SOL",
		"cardano":      "ADA",
		"dogecoin":     "DOGE",
		"polkadot":     "DOT",
		"shiba-inu":    "SHIB",
		"litecoin":     "LTC",
		"avalanche-2":  "AVAX",
		"chainlink":    "LINK",
		"stellar":      "XLM",
		"monero":       "XMR",
		"algorand":     "ALGO",
	}
	if symbol, ok := symbols[id]; ok {
		return symbol
	}
	return strings.ToUpper(id)
}
