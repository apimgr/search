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
)

const maxCurrencyResponseBytes = 1 << 20

// CurrencyFetcher fetches current and recent daily exchange rates from Frankfurter.
type CurrencyFetcher struct {
	httpClient *http.Client
}

// CurrencyHistoryPoint represents a dated exchange rate used by the seven-day chart.
type CurrencyHistoryPoint struct {
	Date string  `json:"date"`
	Rate float64 `json:"rate"`
}

// CurrencyData represents a currency conversion and its recent rate history.
type CurrencyData struct {
	From     string                 `json:"from"`
	To       string                 `json:"to"`
	Amount   float64                `json:"amount"`
	Result   float64                `json:"result"`
	Rate     float64                `json:"rate"`
	RateDate string                 `json:"rate_date"`
	Rates    map[string]float64     `json:"rates,omitempty"`
	History  []CurrencyHistoryPoint `json:"history,omitempty"`
}

// NewCurrencyFetcher creates a currency fetcher that needs no API credentials.
func NewCurrencyFetcher() *CurrencyFetcher {
	return &CurrencyFetcher{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Fetch returns a conversion calculated locally so the amount never leaves this server.
func (f *CurrencyFetcher) Fetch(ctx context.Context, params map[string]string) (*WidgetData, error) {
	from := strings.ToUpper(params["from"])
	to := strings.ToUpper(params["to"])
	if from == "" {
		from = "USD"
	}
	if to == "" {
		to = "EUR"
	}
	if !validCurrencyCode(from) || !validCurrencyCode(to) {
		return nil, fmt.Errorf("currency codes must be three ASCII letters")
	}
	amount := 1.0
	if amountText := strings.TrimSpace(params["amount"]); amountText != "" {
		parsed, err := strconv.ParseFloat(amountText, 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed < 0 || parsed > 1e12 {
			return nil, fmt.Errorf("amount must be a finite number between 0 and 1000000000000")
		}
		amount = parsed
	}

	if from == to {
		today := time.Now().UTC()
		history := make([]CurrencyHistoryPoint, 0, 7)
		for offset := 6; offset >= 0; offset-- {
			history = append(history, CurrencyHistoryPoint{Date: today.AddDate(0, 0, -offset).Format("2006-01-02"), Rate: 1})
		}
		return currencyWidgetData(from, to, amount, 1, today.Format("2006-01-02"), history), nil
	}

	var current struct {
		Date  string  `json:"date"`
		Base  string  `json:"base"`
		Quote string  `json:"quote"`
		Rate  float64 `json:"rate"`
	}
	currentURL := fmt.Sprintf("https://api.frankfurter.dev/v2/rate/%s/%s", from, to)
	if err := f.fetchCurrencyJSON(ctx, currentURL, &current); err != nil {
		return nil, err
	}
	if current.Base != from || current.Quote != to || !validISODate(current.Date) || !validRate(current.Rate) {
		return nil, fmt.Errorf("frankfurter returned an invalid current rate")
	}

	today := time.Now().UTC()
	query := url.Values{
		"base":   {from},
		"quotes": {to},
		"from":   {today.AddDate(0, 0, -6).Format("2006-01-02")},
		"to":     {today.Format("2006-01-02")},
	}
	var history []struct {
		Date  string  `json:"date"`
		Base  string  `json:"base"`
		Quote string  `json:"quote"`
		Rate  float64 `json:"rate"`
	}
	if err := f.fetchCurrencyJSON(ctx, "https://api.frankfurter.dev/v2/rates?"+query.Encode(), &history); err != nil {
		return nil, err
	}
	if len(history) > 8 {
		return nil, fmt.Errorf("frankfurter returned too many historical rates")
	}
	startDate := today.AddDate(0, 0, -6).Format("2006-01-02")
	endDate := today.Format("2006-01-02")
	points := make([]CurrencyHistoryPoint, 0, len(history)+1)
	previousDate := ""
	for _, point := range history {
		if point.Base != from || point.Quote != to || !validISODate(point.Date) || point.Date < startDate || point.Date > endDate || point.Date <= previousDate || !validRate(point.Rate) {
			return nil, fmt.Errorf("frankfurter returned an invalid historical rate")
		}
		previousDate = point.Date
		points = append(points, CurrencyHistoryPoint{Date: point.Date, Rate: point.Rate})
	}
	if len(points) == 0 || points[len(points)-1].Date != current.Date {
		points = append(points, CurrencyHistoryPoint{Date: current.Date, Rate: current.Rate})
	}
	return currencyWidgetData(from, to, amount, current.Rate, current.Date, points), nil
}

func (f *CurrencyFetcher) fetchCurrencyJSON(ctx context.Context, target string, result interface{}) error {
	if f == nil || f.httpClient == nil {
		return fmt.Errorf("currency HTTP client is unavailable")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Search currency widget")
	response, err := f.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			log.Printf("failed to close currency response: %v", err)
		}
	}()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("frankfurter returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxCurrencyResponseBytes+1))
	if err != nil {
		return err
	}
	if len(body) > maxCurrencyResponseBytes {
		return fmt.Errorf("frankfurter response exceeds the size limit")
	}
	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("decode Frankfurter response: %w", err)
	}
	return nil
}

func currencyWidgetData(from, to string, amount, rate float64, rateDate string, history []CurrencyHistoryPoint) *WidgetData {
	data := &CurrencyData{
		From: from, To: to, Amount: amount, Result: amount * rate, Rate: rate, RateDate: rateDate, History: history,
	}
	return &WidgetData{Type: WidgetCurrency, Data: data, UpdatedAt: time.Now()}
}

func validCurrencyCode(code string) bool {
	if len(code) != 3 {
		return false
	}
	for _, character := range code {
		if character < 'A' || character > 'Z' {
			return false
		}
	}
	return true
}

func validISODate(date string) bool {
	parsed, err := time.Parse("2006-01-02", date)
	return err == nil && parsed.Format("2006-01-02") == date
}

func validRate(rate float64) bool {
	return rate > 0 && !math.IsNaN(rate) && !math.IsInf(rate, 0)
}

// CacheDuration returns how long to cache the current and historical rates.
func (f *CurrencyFetcher) CacheDuration() time.Duration {
	return 30 * time.Minute
}

// WidgetType returns the currency widget type.
func (f *CurrencyFetcher) WidgetType() WidgetType {
	return WidgetCurrency
}

// CommonCurrencies provides display names and symbols for frequently used currencies.
// Any valid ISO three-letter code is also accepted and verified by Frankfurter.
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
