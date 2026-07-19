package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// CurrencyFetcher fetches currency exchange rates
type CurrencyFetcher struct {
	httpClient *http.Client
	apiKey     string
}

// CurrencyData represents currency conversion result
type CurrencyData struct {
	From     string             `json:"from"`
	To       string             `json:"to"`
	Amount   float64            `json:"amount"`
	Result   float64            `json:"result"`
	Rate     float64            `json:"rate"`
	RateDate string             `json:"rate_date"`
	Rates    map[string]float64 `json:"rates,omitempty"`
}

// NewCurrencyFetcher creates a new currency fetcher
func NewCurrencyFetcher(apiKey string) *CurrencyFetcher {
	return &CurrencyFetcher{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		apiKey:     apiKey,
	}
}

// Fetch fetches currency conversion data
func (f *CurrencyFetcher) Fetch(ctx context.Context, params map[string]string) (*WidgetData, error) {
	from := strings.ToUpper(params["from"])
	to := strings.ToUpper(params["to"])
	if from == "" {
		from = "USD"
	}
	if to == "" {
		to = "EUR"
	}

	amount := 1.0
	if amtStr, ok := params["amount"]; ok {
		fmt.Sscanf(amtStr, "%f", &amount)
	}

	// Use exchangerate.host API. The access_key query param is required by
	// exchangerate.host for the /convert endpoint; requests without it are
	// rejected with success=false, so the configured apiKey must be sent.
	url := fmt.Sprintf("https://api.exchangerate.host/convert?from=%s&to=%s&amount=%.2f", from, to, amount)
	if f.apiKey != "" {
		url += "&access_key=" + f.apiKey
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &WidgetData{
			Type:      WidgetCurrency,
			Error:     fmt.Sprintf("exchangerate.host API returned status %d", resp.StatusCode),
			UpdatedAt: time.Now(),
		}, nil
	}

	var result struct {
		Success bool `json:"success"`
		Error   struct {
			Code int    `json:"code"`
			Info string `json:"info"`
		} `json:"error"`
		Query struct {
			From   string  `json:"from"`
			To     string  `json:"to"`
			Amount float64 `json:"amount"`
		} `json:"query"`
		Info struct {
			Rate float64 `json:"rate"`
		} `json:"info"`
		Result float64 `json:"result"`
		Date   string  `json:"date"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if !result.Success {
		errMsg := result.Error.Info
		if errMsg == "" {
			errMsg = "exchangerate.host API returned an unsuccessful response"
		}
		return &WidgetData{
			Type:      WidgetCurrency,
			Error:     errMsg,
			UpdatedAt: time.Now(),
		}, nil
	}

	data := &CurrencyData{
		From:     from,
		To:       to,
		Amount:   amount,
		Result:   result.Result,
		Rate:     result.Info.Rate,
		RateDate: result.Date,
	}

	return &WidgetData{
		Type:      WidgetCurrency,
		Data:      data,
		UpdatedAt: time.Now(),
	}, nil
}

// CacheDuration returns how long to cache currency data
func (f *CurrencyFetcher) CacheDuration() time.Duration {
	return 30 * time.Minute
}

// WidgetType returns the widget type
func (f *CurrencyFetcher) WidgetType() WidgetType {
	return WidgetCurrency
}

// Common currency codes for the widget UI
var CommonCurrencies = []struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Symbol string `json:"symbol"`
}{
	{"USD", "US Dollar", "$"},
	{"EUR", "Euro", "€"},
	{"GBP", "British Pound", "£"},
	{"JPY", "Japanese Yen", "¥"},
	{"CNY", "Chinese Yuan", "¥"},
	{"AUD", "Australian Dollar", "A$"},
	{"CAD", "Canadian Dollar", "C$"},
	{"CHF", "Swiss Franc", "Fr"},
	{"INR", "Indian Rupee", "₹"},
	{"MXN", "Mexican Peso", "$"},
	{"BRL", "Brazilian Real", "R$"},
	{"KRW", "South Korean Won", "₩"},
	{"RUB", "Russian Ruble", "₽"},
	{"SGD", "Singapore Dollar", "S$"},
	{"HKD", "Hong Kong Dollar", "HK$"},
	{"NZD", "New Zealand Dollar", "NZ$"},
	{"SEK", "Swedish Krona", "kr"},
	{"NOK", "Norwegian Krone", "kr"},
	{"DKK", "Danish Krone", "kr"},
	{"ZAR", "South African Rand", "R"},
}
