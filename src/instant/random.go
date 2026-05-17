package instant

import (
	"context"
	"fmt"
	mrand "math/rand"
	"regexp"
	"strconv"
	"strings"
)

// RandomHandler generates random numbers
type RandomHandler struct {
	patterns []*regexp.Regexp
}

func NewRandomHandler() *RandomHandler {
	return &RandomHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^random(?:\s+number)?\s*$`),
			regexp.MustCompile(`(?i)^random\s+(\d+)\s*-\s*(\d+)\s*$`),
			regexp.MustCompile(`(?i)^random\s+between\s+(\d+)\s+(?:and\s+)?(\d+)\s*$`),
			regexp.MustCompile(`(?i)^roll\s+dice\s*$`),
			regexp.MustCompile(`(?i)^roll\s+d(\d+)\s*$`),
			regexp.MustCompile(`(?i)^flip\s+coin\s*$`),
			regexp.MustCompile(`(?i)^coin\s+flip\s*$`),
		},
	}
}

func (h *RandomHandler) Name() string               { return "random" }
func (h *RandomHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *RandomHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *RandomHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	lowerQuery := strings.ToLower(query)

	// Coin flip
	if strings.Contains(lowerQuery, "coin") || strings.Contains(lowerQuery, "flip") {
		result := "Heads"
		if mrand.Intn(2) == 1 {
			result = "Tails"
		}
		return &Answer{
			Type:    AnswerTypeRandom,
			Query:   query,
			Title:   "Coin Flip",
			Content: fmt.Sprintf("<div class=\"random-result\"><strong>%s</strong></div>", result),
			Data:    map[string]interface{}{"result": result},
		}, nil
	}

	// Dice roll
	if strings.Contains(lowerQuery, "dice") || strings.Contains(lowerQuery, "roll d") {
		sides := 6
		dicePattern := regexp.MustCompile(`(?i)d(\d+)`)
		if matches := dicePattern.FindStringSubmatch(query); len(matches) > 1 {
			sides, _ = strconv.Atoi(matches[1])
		}
		result := mrand.Intn(sides) + 1
		return &Answer{
			Type:    AnswerTypeRandom,
			Query:   query,
			Title:   fmt.Sprintf("Roll d%d", sides),
			Content: fmt.Sprintf("<div class=\"random-result\"><strong>%d</strong></div>", result),
			Data:    map[string]interface{}{"result": result, "sides": sides},
		}, nil
	}

	// Random number in range
	min, max := 1, 100
	rangePattern := regexp.MustCompile(`(?i)(\d+)\s*[-to]+\s*(\d+)`)
	if matches := rangePattern.FindStringSubmatch(query); len(matches) == 3 {
		min, _ = strconv.Atoi(matches[1])
		max, _ = strconv.Atoi(matches[2])
	}

	result := mrand.Intn(max-min+1) + min

	return &Answer{
		Type:    AnswerTypeRandom,
		Query:   query,
		Title:   fmt.Sprintf("Random Number (%d-%d)", min, max),
		Content: fmt.Sprintf("<div class=\"random-result\"><strong>%d</strong></div>", result),
		Data:    map[string]interface{}{"result": result, "min": min, "max": max},
	}, nil
}
