package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/apimgr/search/src/config"
)

// StocksFetcher fetches stock prices
type StocksFetcher struct {
	client *http.Client
	config *config.StocksWidgetConfig
}

// StocksData represents stocks widget data
type StocksData struct {
	Symbols []StockQuote `json:"symbols"`
}

// StockQuote represents data for a single stock
type StockQuote struct {
	Symbol        string  `json:"symbol"`
	Name          string  `json:"name"`
	Price         float64 `json:"price"`
	Change        float64 `json:"change"`
	ChangePercent float64 `json:"change_percent"`
	Volume        int64   `json:"volume,omitempty"`
	MarketCap     float64 `json:"market_cap,omitempty"`
}

// YahooFinanceResponse represents Yahoo Finance API response
type YahooFinanceResponse struct {
	QuoteResponse struct {
		Result []struct {
			Symbol             string  `json:"symbol"`
			ShortName          string  `json:"shortName"`
			LongName           string  `json:"longName"`
			RegularMarketPrice float64 `json:"regularMarketPrice"`
			RegularMarketChange float64 `json:"regularMarketChange"`
			RegularMarketChangePercent float64 `json:"regularMarketChangePercent"`
			RegularMarketVolume int64   `json:"regularMarketVolume"`
			MarketCap          float64 `json:"marketCap"`
		} `json:"result"`
		Error interface{} `json:"error"`
	} `json:"quoteResponse"`
}

// NewStocksFetcher creates a new stocks fetcher
func NewStocksFetcher(cfg *config.StocksWidgetConfig) *StocksFetcher {
	return &StocksFetcher{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		config: cfg,
	}
}

// WidgetType returns the widget type
func (f *StocksFetcher) WidgetType() WidgetType {
	return WidgetStocks
}

// CacheDuration returns how long to cache the data
func (f *StocksFetcher) CacheDuration() time.Duration {
	return 5 * time.Minute
}

// Fetch fetches stock prices
func (f *StocksFetcher) Fetch(ctx context.Context, params map[string]string) (*WidgetData, error) {
	// Get symbols from params or config
	symbolsStr := params["symbols"]
	var symbols []string
	if symbolsStr != "" {
		symbols = strings.Split(symbolsStr, ",")
		for i := range symbols {
			symbols[i] = strings.TrimSpace(strings.ToUpper(symbols[i]))
		}
	} else {
		symbols = f.config.DefaultSymbols
	}

	if len(symbols) == 0 {
		symbols = []string{"AAPL", "GOOGL", "MSFT"}
	}

	// Fetch from Yahoo Finance
	quotes, err := f.fetchQuotes(ctx, symbols)
	if err != nil {
		return &WidgetData{
			Type:      WidgetStocks,
			Error:     err.Error(),
			UpdatedAt: time.Now(),
		}, nil
	}

	return &WidgetData{
		Type:      WidgetStocks,
		Data:      &StocksData{Symbols: quotes},
		UpdatedAt: time.Now(),
	}, nil
}

// fetchQuotes fetches stock quotes from Yahoo Finance
func (f *StocksFetcher) fetchQuotes(ctx context.Context, symbols []string) ([]StockQuote, error) {
	// Yahoo Finance API endpoint
	symbolList := strings.Join(symbols, ",")
	apiURL := fmt.Sprintf("https://query1.finance.yahoo.com/v7/finance/quote?symbols=%s&fields=symbol,shortName,longName,regularMarketPrice,regularMarketChange,regularMarketChangePercent,regularMarketVolume,marketCap",
		symbolList)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	// Set headers to appear as browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0")
	req.Header.Set("Accept", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var yfResp YahooFinanceResponse
	if err := json.NewDecoder(resp.Body).Decode(&yfResp); err != nil {
		return nil, err
	}

	if yfResp.QuoteResponse.Error != nil {
		return nil, fmt.Errorf("API error: %v", yfResp.QuoteResponse.Error)
	}

	// Convert to our format
	quotes := make([]StockQuote, 0, len(yfResp.QuoteResponse.Result))
	for _, result := range yfResp.QuoteResponse.Result {
		name := result.ShortName
		if name == "" {
			name = result.LongName
		}
		if name == "" {
			name = result.Symbol
		}

		quotes = append(quotes, StockQuote{
			Symbol:        result.Symbol,
			Name:          name,
			Price:         result.RegularMarketPrice,
			Change:        result.RegularMarketChange,
			ChangePercent: result.RegularMarketChangePercent,
			Volume:        result.RegularMarketVolume,
			MarketCap:     result.MarketCap,
		})
	}

	return quotes, nil
}
