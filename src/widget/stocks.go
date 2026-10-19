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
	"strings"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/version"
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
	Symbol        string    `json:"symbol"`
	Name          string    `json:"name"`
	Price         float64   `json:"price"`
	Change        float64   `json:"change"`
	ChangePercent float64   `json:"change_percent"`
	Open          float64   `json:"open,omitempty"`
	High          float64   `json:"high,omitempty"`
	Low           float64   `json:"low,omitempty"`
	Volume        int64     `json:"volume,omitempty"`
	MarketCap     float64   `json:"market_cap,omitempty"`
	Chart         []float64 `json:"chart,omitempty"`
}

// YahooFinanceResponse represents Yahoo Finance API response
type YahooFinanceResponse struct {
	QuoteResponse struct {
		Result []struct {
			Symbol                     string  `json:"symbol"`
			ShortName                  string  `json:"shortName"`
			LongName                   string  `json:"longName"`
			RegularMarketPrice         float64 `json:"regularMarketPrice"`
			RegularMarketChange        float64 `json:"regularMarketChange"`
			RegularMarketChangePercent float64 `json:"regularMarketChangePercent"`
			RegularMarketOpen          float64 `json:"regularMarketOpen"`
			RegularMarketDayHigh       float64 `json:"regularMarketDayHigh"`
			RegularMarketDayLow        float64 `json:"regularMarketDayLow"`
			RegularMarketVolume        int64   `json:"regularMarketVolume"`
			MarketCap                  float64 `json:"marketCap"`
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
		if f.config != nil {
			symbols = f.config.DefaultSymbols
		}
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
	quotes := make([]StockQuote, 0, len(symbols))
	for _, symbol := range symbols {
		quote, err := f.fetchQuote(ctx, symbol)
		if err != nil {
			return nil, err
		}
		quotes = append(quotes, quote)
	}
	return quotes, nil
}

func (f *StocksFetcher) fetchQuote(ctx context.Context, symbol string) (StockQuote, error) {
	query := url.Values{"range": {"1d"}, "interval": {"5m"}}
	apiURL := "https://query1.finance.yahoo.com/v8/finance/chart/" + url.PathEscape(symbol) + "?" + query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return StockQuote{}, err
	}
	request.Header.Set("User-Agent", version.BrowserUserAgent)
	request.Header.Set("Accept", "application/json")
	response, err := f.client.Do(request)
	if err != nil {
		return StockQuote{}, err
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			log.Printf("failed to close stock response: %v", err)
		}
	}()
	if response.StatusCode != http.StatusOK {
		return StockQuote{}, fmt.Errorf("API returned status %d", response.StatusCode)
	}
	const maximumResponseBytes = 2 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes+1))
	if err != nil {
		return StockQuote{}, err
	}
	if len(body) > maximumResponseBytes {
		return StockQuote{}, fmt.Errorf("stock market response exceeds the size limit")
	}
	var payload struct {
		Chart struct {
			Result []struct {
				Meta struct {
					Symbol             string  `json:"symbol"`
					ShortName          string  `json:"shortName"`
					LongName           string  `json:"longName"`
					RegularMarketPrice float64 `json:"regularMarketPrice"`
					ChartPreviousClose float64 `json:"chartPreviousClose"`
					PreviousClose      float64 `json:"previousClose"`
				} `json:"meta"`
				Indicators struct {
					Quote []struct {
						Open   []*float64 `json:"open"`
						High   []*float64 `json:"high"`
						Low    []*float64 `json:"low"`
						Close  []*float64 `json:"close"`
						Volume []*int64   `json:"volume"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
			Error *struct {
				Code        string `json:"code"`
				Description string `json:"description"`
			} `json:"error"`
		} `json:"chart"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return StockQuote{}, err
	}
	if payload.Chart.Error != nil {
		return StockQuote{}, fmt.Errorf("market provider rejected symbol")
	}
	if len(payload.Chart.Result) == 0 {
		return StockQuote{}, fmt.Errorf("market provider returned no quote")
	}
	result := payload.Chart.Result[0]
	if len(result.Indicators.Quote) == 0 {
		return StockQuote{}, fmt.Errorf("market provider returned no price history")
	}
	data := result.Indicators.Quote[0]
	price := result.Meta.RegularMarketPrice
	if price <= 0 {
		price = lastStockPrice(data.Close)
	}
	if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return StockQuote{}, fmt.Errorf("market provider returned an invalid price")
	}
	previousClose := result.Meta.ChartPreviousClose
	if previousClose <= 0 {
		previousClose = result.Meta.PreviousClose
	}
	change, changePercent := 0.0, 0.0
	if previousClose > 0 {
		change = price - previousClose
		changePercent = change / previousClose * 100
	}
	name := result.Meta.LongName
	if name == "" {
		name = result.Meta.ShortName
	}
	if name == "" {
		name = result.Meta.Symbol
	}
	if name == "" {
		name = symbol
	}
	quote := StockQuote{
		Symbol: result.Meta.Symbol, Name: name, Price: price,
		Change: change, ChangePercent: changePercent,
		Open: firstStockPrice(data.Open), High: maximumStockPrice(data.High),
		Low: minimumStockPrice(data.Low), Volume: totalStockVolume(data.Volume),
		Chart: make([]float64, 0, len(data.Close)),
	}
	if quote.Symbol == "" {
		quote.Symbol = symbol
	}
	for _, close := range data.Close {
		if close != nil && *close > 0 && !math.IsNaN(*close) && !math.IsInf(*close, 0) {
			quote.Chart = append(quote.Chart, *close)
		}
	}
	if len(quote.Chart) > 288 {
		quote.Chart = quote.Chart[len(quote.Chart)-288:]
	}
	return quote, nil
}

func firstStockPrice(values []*float64) float64 {
	for _, value := range values {
		if value != nil && *value > 0 && !math.IsNaN(*value) && !math.IsInf(*value, 0) {
			return *value
		}
	}
	return 0
}

func lastStockPrice(values []*float64) float64 {
	for index := len(values) - 1; index >= 0; index-- {
		value := values[index]
		if value != nil && *value > 0 && !math.IsNaN(*value) && !math.IsInf(*value, 0) {
			return *value
		}
	}
	return 0
}

func maximumStockPrice(values []*float64) float64 {
	maximum := 0.0
	for _, value := range values {
		if value != nil && *value > maximum && !math.IsNaN(*value) && !math.IsInf(*value, 0) {
			maximum = *value
		}
	}
	return maximum
}

func minimumStockPrice(values []*float64) float64 {
	minimum := math.Inf(1)
	for _, value := range values {
		if value != nil && *value > 0 && *value < minimum && !math.IsNaN(*value) && !math.IsInf(*value, 0) {
			minimum = *value
		}
	}
	if math.IsInf(minimum, 1) {
		return 0
	}
	return minimum
}

func totalStockVolume(values []*int64) int64 {
	var total int64
	for _, value := range values {
		if value != nil && *value > 0 && *value <= int64(^uint64(0)>>1)-total {
			total += *value
		}
	}
	return total
}
