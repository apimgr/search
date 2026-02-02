package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// NutritionFetcher fetches nutrition facts from USDA FoodData Central and Open Food Facts
type NutritionFetcher struct {
	httpClient *http.Client
	usdaAPIKey string
}

// NutritionData represents nutrition facts result
type NutritionData struct {
	Name         string          `json:"name"`
	BrandName    string          `json:"brand_name,omitempty"`
	Category     string          `json:"category,omitempty"`
	ServingSize  string          `json:"serving_size"`
	ServingSizes []ServingSize   `json:"serving_sizes,omitempty"`
	Calories     float64         `json:"calories"`
	Macros       MacroNutrients  `json:"macros"`
	Micros       []NutrientInfo  `json:"micros,omitempty"`
	Source       string          `json:"source"`
	FDCId        string          `json:"fdc_id,omitempty"`
}

// ServingSize represents a common serving size
type ServingSize struct {
	Description string  `json:"description"`
	Grams       float64 `json:"grams"`
	Calories    float64 `json:"calories,omitempty"`
}

// MacroNutrients represents macronutrient values
type MacroNutrients struct {
	Protein       float64 `json:"protein"`
	Carbohydrates float64 `json:"carbohydrates"`
	Fat           float64 `json:"fat"`
	Fiber         float64 `json:"fiber,omitempty"`
	Sugar         float64 `json:"sugar,omitempty"`
	SaturatedFat  float64 `json:"saturated_fat,omitempty"`
}

// NutrientInfo represents a single nutrient value
type NutrientInfo struct {
	Name   string  `json:"name"`
	Amount float64 `json:"amount"`
	Unit   string  `json:"unit"`
	DV     float64 `json:"dv,omitempty"` // Daily value percentage
}

// NewNutritionFetcher creates a new nutrition fetcher
// usdaAPIKey is optional - if empty, uses DEMO_KEY (limited requests)
func NewNutritionFetcher(usdaAPIKey string) *NutritionFetcher {
	if usdaAPIKey == "" {
		usdaAPIKey = "DEMO_KEY"
	}
	return &NutritionFetcher{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		usdaAPIKey: usdaAPIKey,
	}
}

// Query patterns for nutrition searches
var nutritionPatterns = []*regexp.Regexp{
	// "calories in banana", "calories in 2 apples"
	regexp.MustCompile(`(?i)^calories?\s+(?:in|of|for)\s+(.+)$`),
	// "banana calories"
	regexp.MustCompile(`(?i)^(.+?)\s+calories?$`),
	// "apple nutrition", "chicken breast nutrition"
	regexp.MustCompile(`(?i)^(.+?)\s+nutrition(?:al)?(?:\s+(?:facts?|info(?:rmation)?))?$`),
	// "nutrition facts apple", "nutrition info banana"
	regexp.MustCompile(`(?i)^nutrition(?:al)?\s+(?:facts?|info(?:rmation)?)\s+(?:for\s+)?(.+)$`),
	// "nutrition of apple"
	regexp.MustCompile(`(?i)^nutrition\s+(?:of|for)\s+(.+)$`),
	// "how many calories in banana"
	regexp.MustCompile(`(?i)^how\s+many\s+calories?\s+(?:in|are\s+in|does)\s+(.+?)(?:\s+have)?$`),
	// "macros for chicken breast"
	regexp.MustCompile(`(?i)^macros?\s+(?:for|in|of)\s+(.+)$`),
	// "protein in chicken"
	regexp.MustCompile(`(?i)^(?:protein|carbs?|fat)\s+(?:in|of)\s+(.+)$`),
}

// ExtractFoodItem extracts the food item from a natural language nutrition query
func ExtractFoodItem(query string) string {
	query = strings.TrimSpace(query)

	for _, pattern := range nutritionPatterns {
		if matches := pattern.FindStringSubmatch(query); len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
	}

	// If no pattern matches, return the query as-is (direct food name)
	return query
}

// IsNutritionQuery checks if a query is related to nutrition
func IsNutritionQuery(query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))

	// Check for explicit nutrition keywords
	nutritionKeywords := []string{
		"calories", "calorie", "nutrition", "nutritional",
		"macros", "macro", "protein in", "carbs in", "fat in",
		"how many calories",
	}

	for _, keyword := range nutritionKeywords {
		if strings.Contains(query, keyword) {
			return true
		}
	}

	return false
}

// Fetch fetches nutrition facts
func (f *NutritionFetcher) Fetch(ctx context.Context, params map[string]string) (*WidgetData, error) {
	query := params["query"]
	if query == "" {
		query = params["food"] // Alternative param name
	}
	if query == "" {
		return &WidgetData{
			Type:      WidgetNutrition,
			Error:     "food item required (use 'query' or 'food' parameter)",
			UpdatedAt: time.Now(),
		}, nil
	}

	// Extract food item from natural language query
	foodItem := ExtractFoodItem(query)
	if foodItem == "" {
		foodItem = query
	}

	// Try USDA FoodData Central first (better for whole foods)
	data, err := f.fetchFromUSDA(ctx, foodItem)
	if err == nil && data != nil {
		return &WidgetData{
			Type:      WidgetNutrition,
			Data:      data,
			UpdatedAt: time.Now(),
		}, nil
	}

	// Fall back to Open Food Facts (better for packaged products)
	data, err = f.fetchFromOpenFoodFacts(ctx, foodItem)
	if err != nil {
		return nil, err
	}

	if data == nil {
		return &WidgetData{
			Type:      WidgetNutrition,
			Error:     fmt.Sprintf("no nutrition data found for '%s'", foodItem),
			UpdatedAt: time.Now(),
		}, nil
	}

	return &WidgetData{
		Type:      WidgetNutrition,
		Data:      data,
		UpdatedAt: time.Now(),
	}, nil
}

// fetchFromUSDA fetches nutrition data from USDA FoodData Central API
func (f *NutritionFetcher) fetchFromUSDA(ctx context.Context, foodItem string) (*NutritionData, error) {
	apiURL := fmt.Sprintf("https://api.nal.usda.gov/fdc/v1/foods/search?api_key=%s&query=%s&pageSize=1&dataType=Foundation,SR Legacy",
		f.usdaAPIKey, url.QueryEscape(foodItem))

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

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("USDA API returned status %d", resp.StatusCode)
	}

	var result usdaSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result.TotalHits == 0 || len(result.Foods) == 0 {
		return nil, nil // No results, try fallback
	}

	food := result.Foods[0]
	data := &NutritionData{
		Name:        food.Description,
		Category:    food.FoodCategory,
		ServingSize: "100g",
		Source:      "USDA FoodData Central",
		FDCId:       fmt.Sprintf("%d", food.FDCId),
	}

	// Extract nutrients
	for _, n := range food.FoodNutrients {
		switch n.NutrientID {
		case 1008: // Energy (kcal)
			data.Calories = n.Value
		case 1003: // Protein
			data.Macros.Protein = n.Value
		case 1005: // Carbohydrates
			data.Macros.Carbohydrates = n.Value
		case 1004: // Total lipid (fat)
			data.Macros.Fat = n.Value
		case 1079: // Fiber
			data.Macros.Fiber = n.Value
		case 2000: // Sugars
			data.Macros.Sugar = n.Value
		case 1258: // Saturated fat
			data.Macros.SaturatedFat = n.Value
		case 1087: // Calcium
			data.Micros = append(data.Micros, NutrientInfo{
				Name: "Calcium", Amount: n.Value, Unit: "mg",
			})
		case 1089: // Iron
			data.Micros = append(data.Micros, NutrientInfo{
				Name: "Iron", Amount: n.Value, Unit: "mg",
			})
		case 1162: // Vitamin C
			data.Micros = append(data.Micros, NutrientInfo{
				Name: "Vitamin C", Amount: n.Value, Unit: "mg",
			})
		case 1106: // Vitamin A
			data.Micros = append(data.Micros, NutrientInfo{
				Name: "Vitamin A", Amount: n.Value, Unit: "mcg",
			})
		case 1093: // Sodium
			data.Micros = append(data.Micros, NutrientInfo{
				Name: "Sodium", Amount: n.Value, Unit: "mg",
			})
		case 1092: // Potassium
			data.Micros = append(data.Micros, NutrientInfo{
				Name: "Potassium", Amount: n.Value, Unit: "mg",
			})
		case 1253: // Cholesterol
			data.Micros = append(data.Micros, NutrientInfo{
				Name: "Cholesterol", Amount: n.Value, Unit: "mg",
			})
		}
	}

	// Extract serving sizes from food portions if available
	for _, portion := range food.FoodPortions {
		if portion.GramWeight > 0 {
			serving := ServingSize{
				Description: portion.PortionDescription,
				Grams:       portion.GramWeight,
			}
			if portion.PortionDescription == "" {
				serving.Description = portion.Modifier
			}
			if serving.Description != "" {
				// Calculate calories for this serving size
				serving.Calories = data.Calories * (portion.GramWeight / 100)
				data.ServingSizes = append(data.ServingSizes, serving)
			}
		}
	}

	// Filter out zero-value micronutrients
	var filteredMicros []NutrientInfo
	for _, micro := range data.Micros {
		if micro.Amount > 0 {
			filteredMicros = append(filteredMicros, micro)
		}
	}
	data.Micros = filteredMicros

	return data, nil
}

// usdaSearchResponse represents USDA FoodData Central search response
type usdaSearchResponse struct {
	TotalHits int `json:"totalHits"`
	Foods     []struct {
		FDCId        int    `json:"fdcId"`
		Description  string `json:"description"`
		DataType     string `json:"dataType"`
		FoodCategory string `json:"foodCategory"`
		FoodNutrients []struct {
			NutrientID   int     `json:"nutrientId"`
			NutrientName string  `json:"nutrientName"`
			Value        float64 `json:"value"`
			UnitName     string  `json:"unitName"`
		} `json:"foodNutrients"`
		FoodPortions []struct {
			GramWeight         float64 `json:"gramWeight"`
			PortionDescription string  `json:"portionDescription"`
			Modifier           string  `json:"modifier"`
		} `json:"foodPortions"`
	} `json:"foods"`
}

// fetchFromOpenFoodFacts fetches nutrition data from Open Food Facts API
func (f *NutritionFetcher) fetchFromOpenFoodFacts(ctx context.Context, foodItem string) (*NutritionData, error) {
	apiURL := fmt.Sprintf("https://world.openfoodfacts.org/cgi/search.pl?search_terms=%s&search_simple=1&action=process&json=1&page_size=1",
		url.QueryEscape(foodItem))

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
			ProductName   string `json:"product_name"`
			Brands        string `json:"brands"`
			ServingSize   string `json:"serving_size"`
			Categories    string `json:"categories"`
			ServingQuantity float64 `json:"serving_quantity"`
			Nutriments    struct {
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
				Potassium100g        float64 `json:"potassium_100g"`
				// Per serving values
				EnergyKcalServing    float64 `json:"energy-kcal_serving"`
			} `json:"nutriments"`
		} `json:"products"`
		Count json.Number `json:"count"` // Can be string or int from API
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// Parse count (API sometimes returns string, sometimes int)
	count, _ := result.Count.Int64()
	if count == 0 || len(result.Products) == 0 {
		return nil, nil
	}

	product := result.Products[0]
	n := product.Nutriments

	data := &NutritionData{
		Name:        product.ProductName,
		BrandName:   product.Brands,
		ServingSize: "100g",
		Category:    product.Categories,
		Calories:    n.EnergyKcal100g,
		Macros: MacroNutrients{
			Protein:       n.Proteins100g,
			Carbohydrates: n.Carbohydrates100g,
			Fat:           n.Fat100g,
			Fiber:         n.Fiber100g,
			Sugar:         n.Sugars100g,
			SaturatedFat:  n.SaturatedFat100g,
		},
		Source: "Open Food Facts",
	}

	// Add serving size info if available
	if product.ServingSize != "" {
		data.ServingSizes = append(data.ServingSizes, ServingSize{
			Description: product.ServingSize,
			Grams:       product.ServingQuantity,
			Calories:    n.EnergyKcalServing,
		})
	}

	// Add micronutrients
	micronutrients := []struct {
		name   string
		amount float64
		unit   string
	}{
		{"Sodium", n.Sodium100g, "mg"},
		{"Calcium", n.Calcium100g, "mg"},
		{"Iron", n.Iron100g, "mg"},
		{"Potassium", n.Potassium100g, "mg"},
		{"Cholesterol", n.Cholesterol100g, "mg"},
		{"Vitamin A", n.VitaminA100g, "IU"},
		{"Vitamin C", n.VitaminC100g, "mg"},
	}

	for _, micro := range micronutrients {
		if micro.amount > 0 {
			data.Micros = append(data.Micros, NutrientInfo{
				Name:   micro.name,
				Amount: micro.amount,
				Unit:   micro.unit,
			})
		}
	}

	return data, nil
}

// CacheDuration returns how long to cache nutrition data (24 hours since nutritional data is static)
func (f *NutritionFetcher) CacheDuration() time.Duration {
	return 24 * time.Hour
}

// WidgetType returns the widget type
func (f *NutritionFetcher) WidgetType() WidgetType {
	return WidgetNutrition
}
