package instant

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ConvertHandler handles unit conversions
type ConvertHandler struct {
	patterns []*regexp.Regexp
}

// NewConvertHandler creates a new conversion handler
func NewConvertHandler() *ConvertHandler {
	return &ConvertHandler{
		patterns: []*regexp.Regexp{
			// "convert X unit to unit" or "X unit to unit" or "X unit in unit"
			regexp.MustCompile(`(?i)^(?:convert\s+)?(\d+(?:\.\d+)?)\s*([a-zA-Z°]+)\s+(?:to|in|->)\s+([a-zA-Z°]+)$`),
			// "X unit = ? unit"
			regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s*([a-zA-Z°]+)\s*=\s*\?\s*([a-zA-Z°]+)$`),
		},
	}
}

func (h *ConvertHandler) Name() string {
	return "convert"
}

func (h *ConvertHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *ConvertHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *ConvertHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	var value float64
	var fromUnit, toUnit string

	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) == 4 {
			value, _ = strconv.ParseFloat(matches[1], 64)
			fromUnit = strings.ToLower(matches[2])
			toUnit = strings.ToLower(matches[3])
			break
		}
	}

	if fromUnit == "" || toUnit == "" {
		return nil, nil
	}

	// Normalize unit names
	fromUnit = normalizeUnit(fromUnit)
	toUnit = normalizeUnit(toUnit)

	// Perform conversion
	result, err := convert(value, fromUnit, toUnit)
	if err != nil {
		return &Answer{
			Type:    AnswerTypeConvert,
			Query:   query,
			Title:   "Unit Conversion",
			Content: fmt.Sprintf("Cannot convert %s to %s: %v", fromUnit, toUnit, err),
		}, nil
	}

	return &Answer{
		Type:  AnswerTypeConvert,
		Query: query,
		Title: "Unit Conversion",
		Content: fmt.Sprintf("<div class=\"conversion-result\">%s %s = <strong>%s %s</strong></div>",
			formatNumber(value), fromUnit, formatNumber(result), toUnit),
		Data: map[string]interface{}{
			"value":    value,
			"fromUnit": fromUnit,
			"toUnit":   toUnit,
			"result":   result,
		},
	}, nil
}

// normalizeUnit normalizes unit names to standard form
func normalizeUnit(unit string) string {
	aliases := map[string]string{
		// Length
		"m":           "meters",
		"meter":       "meters",
		"metre":       "meters",
		"metres":      "meters",
		"km":          "kilometers",
		"kilometer":   "kilometers",
		"kilometre":   "kilometers",
		"kilometres":  "kilometers",
		"cm":          "centimeters",
		"centimeter":  "centimeters",
		"centimetre":  "centimeters",
		"centimetres": "centimeters",
		"mm":          "millimeters",
		"millimeter":  "millimeters",
		"millimetre":  "millimeters",
		"millimetres": "millimeters",
		"mi":          "miles",
		"mile":        "miles",
		"ft":          "feet",
		"foot":        "feet",
		"in":          "inches",
		"inch":        "inches",
		"yd":          "yards",
		"yard":        "yards",

		// Weight/Mass
		"kg":        "kilograms",
		"kilogram":  "kilograms",
		"g":         "grams",
		"gram":      "grams",
		"mg":        "milligrams",
		"milligram": "milligrams",
		"lb":        "pounds",
		"lbs":       "pounds",
		"pound":     "pounds",
		"oz":        "ounces",
		"ounce":     "ounces",
		"t":         "tons",
		"ton":       "tons",
		"tonne":     "tonnes",

		// Temperature
		"c":          "celsius",
		"°c":         "celsius",
		"f":          "fahrenheit",
		"°f":         "fahrenheit",
		"k":          "kelvin",
		"°k":         "kelvin",

		// Volume
		"l":          "liters",
		"liter":      "liters",
		"litre":      "liters",
		"litres":     "liters",
		"ml":         "milliliters",
		"milliliter": "milliliters",
		"millilitre": "milliliters",
		"gal":        "gallons",
		"gallon":     "gallons",
		"qt":         "quarts",
		"quart":      "quarts",
		"pt":         "pints",
		"pint":       "pints",
		"cup":        "cups",

		// Time
		"s":       "seconds",
		"sec":     "seconds",
		"second":  "seconds",
		"min":     "minutes",
		"minute":  "minutes",
		"h":       "hours",
		"hr":      "hours",
		"hour":    "hours",
		"d":       "days",
		"day":     "days",
		"wk":      "weeks",
		"week":    "weeks",
		"mo":      "months",
		"month":   "months",
		"yr":      "years",
		"year":    "years",

		// Data
		"b":  "bytes",
		"kb": "kilobytes",
		"mb": "megabytes",
		"gb": "gigabytes",
		"tb": "terabytes",
	}

	if normalized, ok := aliases[unit]; ok {
		return normalized
	}
	return unit
}

// convert performs unit conversion
func convert(value float64, from, to string) (float64, error) {
	// Check if same unit
	if from == to {
		return value, nil
	}

	// Convert to base unit first, then to target
	// Length (base: meters)
	lengthToMeters := map[string]float64{
		"meters":      1,
		"kilometers":  1000,
		"centimeters": 0.01,
		"millimeters": 0.001,
		"miles":       1609.344,
		"feet":        0.3048,
		"inches":      0.0254,
		"yards":       0.9144,
	}

	// Weight (base: grams)
	weightToGrams := map[string]float64{
		"grams":      1,
		"kilograms":  1000,
		"milligrams": 0.001,
		"pounds":     453.592,
		"ounces":     28.3495,
		"tons":       907185,
		"tonnes":     1000000,
	}

	// Volume (base: liters)
	volumeToLiters := map[string]float64{
		"liters":      1,
		"milliliters": 0.001,
		"gallons":     3.78541,
		"quarts":      0.946353,
		"pints":       0.473176,
		"cups":        0.236588,
	}

	// Time (base: seconds)
	timeToSeconds := map[string]float64{
		"seconds": 1,
		"minutes": 60,
		"hours":   3600,
		"days":    86400,
		"weeks":   604800,
		"months":  2629746, // average
		"years":   31556952,
	}

	// Data (base: bytes)
	dataToBytes := map[string]float64{
		"bytes":     1,
		"kilobytes": 1024,
		"megabytes": 1048576,
		"gigabytes": 1073741824,
		"terabytes": 1099511627776,
	}

	// Temperature (special handling)
	if (from == "celsius" || from == "fahrenheit" || from == "kelvin") &&
		(to == "celsius" || to == "fahrenheit" || to == "kelvin") {
		return convertTemperature(value, from, to), nil
	}

	// Try each conversion table
	tables := []map[string]float64{lengthToMeters, weightToGrams, volumeToLiters, timeToSeconds, dataToBytes}

	for _, table := range tables {
		fromFactor, fromOk := table[from]
		toFactor, toOk := table[to]
		if fromOk && toOk {
			// Convert: value * fromFactor / toFactor
			return value * fromFactor / toFactor, nil
		}
	}

	return 0, fmt.Errorf("unknown conversion")
}

// convertTemperature handles temperature conversions
func convertTemperature(value float64, from, to string) float64 {
	// Convert to Celsius first
	var celsius float64
	switch from {
	case "celsius":
		celsius = value
	case "fahrenheit":
		celsius = (value - 32) * 5 / 9
	case "kelvin":
		celsius = value - 273.15
	default:
		// Unknown from unit, return original value
		return value
	}

	// Convert from Celsius to target
	switch to {
	case "celsius":
		return celsius
	case "fahrenheit":
		return celsius*9/5 + 32
	case "kelvin":
		return celsius + 273.15
	default:
		// Unknown to unit, return original value
		return value
	}
}
