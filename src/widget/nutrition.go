package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// NutritionFetcher fetches nutrition facts
type NutritionFetcher struct {
	httpClient *http.Client
	apiKey     string
}

// NutritionData represents nutrition facts result
type NutritionData struct {
	Name        string            `json:"name"`
	BrandName   string            `json:"brand_name,omitempty"`
	ServingSize string            `json:"serving_size"`
	Nutrients   []NutrientInfo    `json:"nutrients"`
	Category    string            `json:"category,omitempty"`
}

// NutrientInfo represents a single nutrient value
type NutrientInfo struct {
	Name   string  `json:"name"`
	Amount float64 `json:"amount"`
	Unit   string  `json:"unit"`
}

// NewNutritionFetcher creates a new nutrition fetcher
func NewNutritionFetcher(apiKey string) *NutritionFetcher {
	return &NutritionFetcher{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		apiKey:     apiKey,
	}
}

// Fetch fetches nutrition facts
func (f *NutritionFetcher) Fetch(ctx context.Context, params map[string]string) (*WidgetData, error) {
	query := params["query"]
	if query == "" {
		return &WidgetData{
			Type:      WidgetNutrition,
			Error:     "query parameter required",
			UpdatedAt: time.Now(),
		}, nil
	}

	// Use Open Food Facts API (free, no API key required)
	apiURL := fmt.Sprintf("https://world.openfoodfacts.org/cgi/search.pl?search_terms=%s&search_simple=1&action=process&json=1&page_size=1",
		url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Search/1.0 (privacy-focused search engine)")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Products []struct {
			ProductName string `json:"product_name"`
			Brands      string `json:"brands"`
			ServingSize string `json:"serving_size"`
			Categories  string `json:"categories"`
			Nutriments  struct {
				EnergyKcal100g       float64 `json:"energy-kcal_100g"`
				Fat100g              float64 `json:"fat_100g"`
				SaturatedFat100g     float64 `json:"saturated-fat_100g"`
				Carbohydrates100g    float64 `json:"carbohydrates_100g"`
				Sugars100g           float64 `json:"sugars_100g"`
				Fiber100g            float64 `json:"fiber_100g"`
				Proteins100g         float64 `json:"proteins_100g"`
				Salt100g             float64 `json:"salt_100g"`
				Sodium100g           float64 `json:"sodium_100g"`
				Calcium100g          float64 `json:"calcium_100g"`
				Iron100g             float64 `json:"iron_100g"`
				VitaminA100g         float64 `json:"vitamin-a_100g"`
				VitaminC100g         float64 `json:"vitamin-c_100g"`
				Cholesterol100g      float64 `json:"cholesterol_100g"`
			} `json:"nutriments"`
		} `json:"products"`
		Count int `json:"count"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result.Count == 0 || len(result.Products) == 0 {
		return &WidgetData{
			Type:      WidgetNutrition,
			Error:     "no nutrition data found",
			UpdatedAt: time.Now(),
		}, nil
	}

	product := result.Products[0]
	n := product.Nutriments

	data := &NutritionData{
		Name:        product.ProductName,
		BrandName:   product.Brands,
		ServingSize: product.ServingSize,
		Category:    product.Categories,
		Nutrients: []NutrientInfo{
			{Name: "Calories", Amount: n.EnergyKcal100g, Unit: "kcal"},
			{Name: "Total Fat", Amount: n.Fat100g, Unit: "g"},
			{Name: "Saturated Fat", Amount: n.SaturatedFat100g, Unit: "g"},
			{Name: "Carbohydrates", Amount: n.Carbohydrates100g, Unit: "g"},
			{Name: "Sugars", Amount: n.Sugars100g, Unit: "g"},
			{Name: "Fiber", Amount: n.Fiber100g, Unit: "g"},
			{Name: "Protein", Amount: n.Proteins100g, Unit: "g"},
			{Name: "Sodium", Amount: n.Sodium100g, Unit: "mg"},
			{Name: "Cholesterol", Amount: n.Cholesterol100g, Unit: "mg"},
			{Name: "Calcium", Amount: n.Calcium100g, Unit: "mg"},
			{Name: "Iron", Amount: n.Iron100g, Unit: "mg"},
			{Name: "Vitamin A", Amount: n.VitaminA100g, Unit: "IU"},
			{Name: "Vitamin C", Amount: n.VitaminC100g, Unit: "mg"},
		},
	}

	// Filter out zero values
	var filteredNutrients []NutrientInfo
	for _, nutrient := range data.Nutrients {
		if nutrient.Amount > 0 {
			filteredNutrients = append(filteredNutrients, nutrient)
		}
	}
	if len(filteredNutrients) > 0 {
		data.Nutrients = filteredNutrients
	}

	return &WidgetData{
		Type:      WidgetNutrition,
		Data:      data,
		UpdatedAt: time.Now(),
	}, nil
}

// CacheDuration returns how long to cache nutrition data
func (f *NutritionFetcher) CacheDuration() time.Duration {
	return 24 * time.Hour
}

// WidgetType returns the widget type
func (f *NutritionFetcher) WidgetType() WidgetType {
	return WidgetNutrition
}
