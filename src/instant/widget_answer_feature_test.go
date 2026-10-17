package instant

import (
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/widget"
)

func TestWidgetAnswerRenderersShowAvailableIdeaDetails(t *testing.T) {
	change := 5.0
	coin := &widget.CryptoData{Coins: []widget.CoinData{{Name: "Bitcoin", Symbol: "BTC", Currency: "USD", Price: 100, Change24h: change, MarketCap: 2_000_000_000, Volume24h: 500_000_000, Chart: []float64{90, 100, 95}}, {Name: "Ethereum", Symbol: "ETH", Currency: "USD", Amount: 0.5, Price: 100, Change24h: change, Chart: []float64{80, 100, 90}}, {Name: "Litecoin", Symbol: "LTC", Currency: "EUR", Price: 20, MarketCap: 2_000_000, Volume24h: 30, Chart: []float64{19, 20}}}}
	_, crypto, _, _, ok := renderCrypto("en", coin)
	if !ok || !strings.Contains(crypto, "Market cap: $2.00B") || !strings.Contains(crypto, "24h volume: $500.00M") || !strings.Contains(crypto, "$100.00") || !strings.Contains(crypto, "($5.00)") || !strings.Contains(crypto, "0.5 ETH: $50.00") || !strings.Contains(crypto, "($2.50)") || !strings.Contains(crypto, `class="crypto-price-chart"`) || !strings.Contains(crypto, "Market cap: €2.00M") || !strings.Contains(crypto, "24h volume: €30.00") {
		t.Fatalf("crypto answer omitted available market data: %q", crypto)
	}

	stocks := &widget.StocksData{Symbols: []widget.StockQuote{{Symbol: "ACME", Price: 10, Open: 9, High: 12, Low: 8, Volume: 12345, Chart: []float64{8, 10, 9}}}}
	_, stock, _, _, ok := renderStocks("en", stocks)
	if !ok || !strings.Contains(stock, "Open: 9.00") || !strings.Contains(stock, "High: 12.00") || !strings.Contains(stock, "Low: 8.00") || !strings.Contains(stock, "Volume: 12345") || !strings.Contains(stock, `class="stock-price-chart"`) {
		t.Fatalf("stock answer omitted available quote details: %q", stock)
	}

	start := time.Date(2026, time.January, 2, 15, 4, 0, 0, time.UTC).Format(time.RFC3339)
	sports := &widget.SportsData{Games: []widget.GameData{{League: "League", Status: widget.GameStatusScheduled, StartTime: start}}}
	_, game, _, _, ok := renderSports("en", sports)
	if !ok || !strings.Contains(game, "2026-01-02 15:04 UTC") {
		t.Fatalf("sports answer omitted scheduled start time: %q", game)
	}

	nutrition := &widget.NutritionData{Name: "Apple", ServingSizes: []widget.ServingSize{{Description: "1 medium", Calories: 95}}}
	_, facts, _, _, ok := renderNutrition("en", nutrition)
	if !ok || !strings.Contains(facts, "Serving option: 1 medium (95 kcal)") {
		t.Fatalf("nutrition answer omitted available serving size: %q", facts)
	}
}

func TestFormatCompactCurrency(t *testing.T) {
	for _, tt := range []struct {
		value    float64
		currency string
		want     string
	}{{12.5, "USD", "$12.50"}, {1_250_000, "USD", "$1.25M"}, {2_500_000_000, "USD", "$2.50B"}, {3_000_000_000_000, "USD", "$3.00T"}, {12.5, "EUR", "€12.50"}, {12.5, "GBP", "£12.50"}} {
		if got := formatCompactCurrency(tt.value, tt.currency); got != tt.want {
			t.Errorf("formatCompactCurrency(%v, %s) = %q, want %q", tt.value, tt.currency, got, tt.want)
		}
	}
}

func TestTranslationAnswerHasCopyControl(t *testing.T) {
	data := &widget.TranslateData{SourceText: "hello", TranslatedText: `hola "amigo"`, Provider: "fixture"}
	_, content, _, _, ok := renderTranslate("en", data)
	if !ok || !strings.Contains(content, `data-copy="hola &quot;amigo&quot;"`) || !strings.Contains(content, "translation-result") || !strings.Contains(content, `<button type="button" class="copy-btn"`) {
		t.Fatalf("translation answer omitted safe copy control: %s", content)
	}
}
