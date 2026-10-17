package instant

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
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
			regexp.MustCompile(`(?i)^random\s+(\d+)\s*(?:-|to)\s*(\d+)\s*$`),
			regexp.MustCompile(`(?i)^random\s+between\s+(\d+)\s+(?:and\s+)?(\d+)\s*$`),
			regexp.MustCompile(`(?i)^pick\s+random\s+(\d+)\s*(?:-|to)\s*(\d+)\s*$`),
			regexp.MustCompile(`(?i)^roll\s+dice\s*$`),
			regexp.MustCompile(`(?i)^roll\s+(\d+)d(\d+)\s*$`),
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

func (h *RandomHandler) HandleInstantQuery(ctx context.Context, query string) (*Answer, error) {
	lowerQuery := strings.ToLower(query)

	// Coin flip
	if strings.Contains(lowerQuery, "coin") || strings.Contains(lowerQuery, "flip") {
		result := "Heads"
		flip, err := randomInt(2)
		if err != nil {
			return nil, err
		}
		if flip == 1 {
			result = "Tails"
		}
		return &Answer{
			Type:    AnswerTypeRandom,
			Query:   query,
			Title:   "Coin Flip",
			Content: fmt.Sprintf(`<div class="random-result" data-random-kind="coin"><strong class="random-value">%s</strong> <button type="button" data-action="roll-again">Roll again</button></div>`, result),
			Data:    map[string]interface{}{"result": result},
		}, nil
	}

	// Dice roll
	if strings.Contains(lowerQuery, "dice") || strings.Contains(lowerQuery, "roll d") || regexp.MustCompile(`(?i)^roll\s+\d+d\d+\s*$`).MatchString(query) {
		diceCount, sides := 1, 6
		multipleDicePattern := regexp.MustCompile(`(?i)^roll\s+(\d+)d(\d+)\s*$`)
		dicePattern := regexp.MustCompile(`(?i)d(\d+)`)
		if matches := multipleDicePattern.FindStringSubmatch(query); len(matches) == 3 {
			var err error
			diceCount, err = strconv.Atoi(matches[1])
			if err != nil || diceCount < 1 || diceCount > 100 {
				return nil, fmt.Errorf("dice count must be between 1 and 100")
			}
			sides, err = strconv.Atoi(matches[2])
			if err != nil {
				return nil, fmt.Errorf("dice sides are invalid")
			}
		} else if matches := dicePattern.FindStringSubmatch(query); len(matches) > 1 {
			var err error
			sides, err = strconv.Atoi(matches[1])
			if err != nil {
				return nil, fmt.Errorf("dice sides are invalid")
			}
		}
		if sides < 1 || uint64(sides) > uint64(^uint32(0)) {
			return nil, fmt.Errorf("dice sides must be between 1 and %d", uint64(^uint32(0)))
		}
		rolls := make([]int, diceCount)
		total := 0
		for i := range rolls {
			roll, err := randomInt(sides)
			if err != nil {
				return nil, err
			}
			rolls[i] = roll + 1
			total += rolls[i]
		}
		resultText := strconv.Itoa(total)
		if diceCount > 1 {
			resultText = fmt.Sprintf("%s (total %d)", strings.Trim(strings.ReplaceAll(fmt.Sprint(rolls), " ", ", "), "[]"), total)
		}
		return &Answer{
			Type:    AnswerTypeRandom,
			Query:   query,
			Title:   fmt.Sprintf("Roll %dd%d", diceCount, sides),
			Content: fmt.Sprintf(`<div class="random-result" data-random-kind="dice" data-count="%d" data-min="1" data-max="%d"><strong class="random-value">%s</strong> <button type="button" data-action="roll-again">Roll again</button></div>`, diceCount, sides, resultText),
			Data:    map[string]interface{}{"result": total, "rolls": rolls, "sides": sides},
		}, nil
	}

	// Random number in range
	min, max := 1, 100
	for _, pattern := range h.patterns {
		if matches := pattern.FindStringSubmatch(query); len(matches) == 3 {
			var err error
			min, err = strconv.Atoi(matches[1])
			if err != nil {
				return nil, fmt.Errorf("random range lower bound is invalid")
			}
			max, err = strconv.Atoi(matches[2])
			if err != nil {
				return nil, fmt.Errorf("random range upper bound is invalid")
			}
			break
		}
	}

	if min > max {
		min, max = max, min
	}
	if int64(max)-int64(min) > int64(^uint32(0))-1 {
		return nil, fmt.Errorf("random range may span at most %d values", uint64(^uint32(0)))
	}
	roll, err := randomInt(max - min + 1)
	if err != nil {
		return nil, err
	}
	result := roll + min

	return &Answer{
		Type:    AnswerTypeRandom,
		Query:   query,
		Title:   fmt.Sprintf("Random Number (%d-%d)", min, max),
		Content: fmt.Sprintf(`<div class="random-result" data-random-kind="number" data-min="%d" data-max="%d"><strong class="random-value">%d</strong> <button type="button" data-action="roll-again">Roll again</button></div>`, min, max, result),
		Data:    map[string]interface{}{"result": result, "min": min, "max": max},
	}, nil
}

func randomInt(max int) (int, error) {
	if max < 1 {
		return 0, fmt.Errorf("random range must be positive")
	}
	value, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, fmt.Errorf("generate secure random value: %w", err)
	}
	return int(value.Int64()), nil
}
