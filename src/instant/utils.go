package instant

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	mrand "math/rand"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// TimeHandler handles time/date queries
type TimeHandler struct {
	patterns []*regexp.Regexp
}

func NewTimeHandler() *TimeHandler {
	return &TimeHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^time\s*$`),
			regexp.MustCompile(`(?i)^current\s+time\s*$`),
			regexp.MustCompile(`(?i)^what\s+time\s+is\s+it\s*\??$`),
			regexp.MustCompile(`(?i)^now\s*$`),
			regexp.MustCompile(`(?i)^date\s*$`),
			regexp.MustCompile(`(?i)^today\s*$`),
			regexp.MustCompile(`(?i)^current\s+date\s*$`),
			regexp.MustCompile(`(?i)^timestamp\s*$`),
			regexp.MustCompile(`(?i)^unix\s*time(?:stamp)?\s*$`),
			regexp.MustCompile(`(?i)^epoch\s*$`),
		},
	}
}

func (h *TimeHandler) Name() string         { return "time" }
func (h *TimeHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *TimeHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *TimeHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	now := time.Now()
	utc := now.UTC()

	content := fmt.Sprintf(`<div class="time-result">
<strong>Local Time:</strong> %s<br>
<strong>UTC Time:</strong> %s<br>
<strong>Unix Timestamp:</strong> %d<br>
<strong>ISO 8601:</strong> %s<br>
<strong>Day of Year:</strong> %d<br>
<strong>Week of Year:</strong> %d
</div>`,
		now.Format("Monday, January 2, 2006 3:04:05 PM MST"),
		utc.Format("Monday, January 2, 2006 15:04:05 UTC"),
		now.Unix(),
		now.Format(time.RFC3339),
		now.YearDay(),
		getWeekOfYear(now),
	)

	return &Answer{
		Type:    AnswerTypeTime,
		Query:   query,
		Title:   "Current Time",
		Content: content,
		Data: map[string]interface{}{
			"local":     now.Format(time.RFC3339),
			"utc":       utc.Format(time.RFC3339),
			"timestamp": now.Unix(),
		},
	}, nil
}

func getWeekOfYear(t time.Time) int {
	_, week := t.ISOWeek()
	return week
}

// HashHandler handles hash generation
type HashHandler struct {
	patterns []*regexp.Regexp
}

func NewHashHandler() *HashHandler {
	return &HashHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^md5[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^sha1[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^sha256[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^sha512[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^hash[:\s]+(.+)$`),
		},
	}
}

func (h *HashHandler) Name() string         { return "hash" }
func (h *HashHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *HashHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *HashHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	lowerQuery := strings.ToLower(query)
	var text string
	var hashType string

	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			text = matches[1]
			if strings.HasPrefix(lowerQuery, "md5") {
				hashType = "md5"
			} else if strings.HasPrefix(lowerQuery, "sha1") {
				hashType = "sha1"
			} else if strings.HasPrefix(lowerQuery, "sha256") {
				hashType = "sha256"
			} else if strings.HasPrefix(lowerQuery, "sha512") {
				hashType = "sha512"
			} else {
				hashType = "all"
			}
			break
		}
	}

	if text == "" {
		return nil, nil
	}

	data := []byte(text)
	var content strings.Builder
	content.WriteString(fmt.Sprintf("<strong>Input:</strong> %s<br><br>", text))

	if hashType == "md5" || hashType == "all" {
		hash := md5.Sum(data)
		content.WriteString(fmt.Sprintf("<strong>MD5:</strong> <code>%s</code><br>", hex.EncodeToString(hash[:])))
	}
	if hashType == "sha1" || hashType == "all" {
		hash := sha1.Sum(data)
		content.WriteString(fmt.Sprintf("<strong>SHA1:</strong> <code>%s</code><br>", hex.EncodeToString(hash[:])))
	}
	if hashType == "sha256" || hashType == "all" {
		hash := sha256.Sum256(data)
		content.WriteString(fmt.Sprintf("<strong>SHA256:</strong> <code>%s</code><br>", hex.EncodeToString(hash[:])))
	}
	if hashType == "sha512" || hashType == "all" {
		hash := sha512.Sum512(data)
		content.WriteString(fmt.Sprintf("<strong>SHA512:</strong> <code>%s</code><br>", hex.EncodeToString(hash[:])))
	}

	return &Answer{
		Type:    AnswerTypeHash,
		Query:   query,
		Title:   "Hash Generator",
		Content: content.String(),
	}, nil
}

// Base64Handler handles base64 encoding/decoding
type Base64Handler struct {
	patterns []*regexp.Regexp
}

func NewBase64Handler() *Base64Handler {
	return &Base64Handler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^base64\s+encode[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^base64\s+decode[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^b64\s+encode[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^b64\s+decode[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^encode\s+base64[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^decode\s+base64[:\s]+(.+)$`),
		},
	}
}

func (h *Base64Handler) Name() string         { return "base64" }
func (h *Base64Handler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *Base64Handler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *Base64Handler) Handle(ctx context.Context, query string) (*Answer, error) {
	lowerQuery := strings.ToLower(query)
	isDecode := strings.Contains(lowerQuery, "decode")

	var text string
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			text = matches[1]
			break
		}
	}

	if text == "" {
		return nil, nil
	}

	var result, operation string
	if isDecode {
		decoded, err := base64.StdEncoding.DecodeString(text)
		if err != nil {
			return &Answer{
				Type:    AnswerTypeBase64,
				Query:   query,
				Title:   "Base64 Decoder",
				Content: fmt.Sprintf("Error: Invalid base64 string"),
			}, nil
		}
		result = string(decoded)
		operation = "Decoded"
	} else {
		result = base64.StdEncoding.EncodeToString([]byte(text))
		operation = "Encoded"
	}

	return &Answer{
		Type:    AnswerTypeBase64,
		Query:   query,
		Title:   fmt.Sprintf("Base64 %s", operation),
		Content: fmt.Sprintf("<strong>Input:</strong> %s<br><br><strong>%s:</strong> <code>%s</code>", text, operation, result),
		Data: map[string]interface{}{
			"input":     text,
			"output":    result,
			"operation": strings.ToLower(operation),
		},
	}, nil
}

// URLHandler handles URL encoding/decoding and parsing
type URLHandler struct {
	patterns []*regexp.Regexp
}

func NewURLHandler() *URLHandler {
	return &URLHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^url\s+encode[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^url\s+decode[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^urlencode[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^urldecode[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^parse\s+url[:\s]+(.+)$`),
		},
	}
}

func (h *URLHandler) Name() string         { return "url" }
func (h *URLHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *URLHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *URLHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	lowerQuery := strings.ToLower(query)
	isDecode := strings.Contains(lowerQuery, "decode")
	isParse := strings.Contains(lowerQuery, "parse")

	var text string
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			text = matches[1]
			break
		}
	}

	if text == "" {
		return nil, nil
	}

	if isParse {
		parsed, err := url.Parse(text)
		if err != nil {
			return &Answer{
				Type:    AnswerTypeURL,
				Query:   query,
				Title:   "URL Parser",
				Content: fmt.Sprintf("Error: Invalid URL"),
			}, nil
		}

		var content strings.Builder
		content.WriteString(fmt.Sprintf("<strong>URL:</strong> %s<br><br>", text))
		content.WriteString(fmt.Sprintf("<strong>Scheme:</strong> %s<br>", parsed.Scheme))
		content.WriteString(fmt.Sprintf("<strong>Host:</strong> %s<br>", parsed.Host))
		content.WriteString(fmt.Sprintf("<strong>Path:</strong> %s<br>", parsed.Path))
		if parsed.RawQuery != "" {
			content.WriteString(fmt.Sprintf("<strong>Query:</strong> %s<br>", parsed.RawQuery))
		}
		if parsed.Fragment != "" {
			content.WriteString(fmt.Sprintf("<strong>Fragment:</strong> %s<br>", parsed.Fragment))
		}

		return &Answer{
			Type:    AnswerTypeURL,
			Query:   query,
			Title:   "URL Parser",
			Content: content.String(),
		}, nil
	}

	var result, operation string
	if isDecode {
		decoded, err := url.QueryUnescape(text)
		if err != nil {
			result = text
		} else {
			result = decoded
		}
		operation = "Decoded"
	} else {
		result = url.QueryEscape(text)
		operation = "Encoded"
	}

	return &Answer{
		Type:    AnswerTypeURL,
		Query:   query,
		Title:   fmt.Sprintf("URL %s", operation),
		Content: fmt.Sprintf("<strong>Input:</strong> %s<br><br><strong>%s:</strong> <code>%s</code>", text, operation, result),
	}, nil
}

// ColorHandler handles color conversions
type ColorHandler struct {
	patterns []*regexp.Regexp
}

func NewColorHandler() *ColorHandler {
	return &ColorHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^color[:\s]+#?([0-9a-fA-F]{6}|[0-9a-fA-F]{3})$`),
			regexp.MustCompile(`(?i)^rgb[:\s]+\(?(\d{1,3})[,\s]+(\d{1,3})[,\s]+(\d{1,3})\)?$`),
			regexp.MustCompile(`(?i)^#([0-9a-fA-F]{6}|[0-9a-fA-F]{3})$`),
		},
	}
}

func (h *ColorHandler) Name() string         { return "color" }
func (h *ColorHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *ColorHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *ColorHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	var r, g, b int
	var hexColor string

	// Try to parse hex
	hexPattern := regexp.MustCompile(`(?i)#?([0-9a-fA-F]{6}|[0-9a-fA-F]{3})`)
	if matches := hexPattern.FindStringSubmatch(query); len(matches) > 1 {
		hexColor = matches[1]
		if len(hexColor) == 3 {
			hexColor = string(hexColor[0]) + string(hexColor[0]) +
				string(hexColor[1]) + string(hexColor[1]) +
				string(hexColor[2]) + string(hexColor[2])
		}
		rVal, _ := strconv.ParseInt(hexColor[0:2], 16, 64)
		gVal, _ := strconv.ParseInt(hexColor[2:4], 16, 64)
		bVal, _ := strconv.ParseInt(hexColor[4:6], 16, 64)
		r, g, b = int(rVal), int(gVal), int(bVal)
	}

	// Try to parse RGB
	rgbPattern := regexp.MustCompile(`(?i)rgb[:\s]+\(?(\d{1,3})[,\s]+(\d{1,3})[,\s]+(\d{1,3})\)?`)
	if matches := rgbPattern.FindStringSubmatch(query); len(matches) == 4 {
		r, _ = strconv.Atoi(matches[1])
		g, _ = strconv.Atoi(matches[2])
		b, _ = strconv.Atoi(matches[3])
		hexColor = fmt.Sprintf("%02x%02x%02x", r, g, b)
	}

	if hexColor == "" {
		return nil, nil
	}

	content := fmt.Sprintf(`<div class="color-result">
<div class="color-preview" style="background-color: #%s; width: 100px; height: 100px; border: 1px solid #333; display: inline-block;"></div>
<br><br>
<strong>HEX:</strong> #%s<br>
<strong>RGB:</strong> rgb(%d, %d, %d)<br>
<strong>HSL:</strong> %s
</div>`,
		hexColor, strings.ToUpper(hexColor), r, g, b, rgbToHSL(r, g, b))

	return &Answer{
		Type:    AnswerTypeColor,
		Query:   query,
		Title:   "Color Converter",
		Content: content,
	}, nil
}

func rgbToHSL(r, g, b int) string {
	rf := float64(r) / 255
	gf := float64(g) / 255
	bf := float64(b) / 255

	max := rf
	if gf > max {
		max = gf
	}
	if bf > max {
		max = bf
	}
	min := rf
	if gf < min {
		min = gf
	}
	if bf < min {
		min = bf
	}

	l := (max + min) / 2

	var h, s float64
	if max == min {
		h, s = 0, 0
	} else {
		d := max - min
		if l > 0.5 {
			s = d / (2 - max - min)
		} else {
			s = d / (max + min)
		}

		switch max {
		case rf:
			h = (gf - bf) / d
			if gf < bf {
				h += 6
			}
		case gf:
			h = (bf-rf)/d + 2
		case bf:
			h = (rf-gf)/d + 4
		}
		h /= 6
	}

	return fmt.Sprintf("hsl(%.0f, %.0f%%, %.0f%%)", h*360, s*100, l*100)
}

// UUIDHandler generates UUIDs
type UUIDHandler struct {
	patterns []*regexp.Regexp
}

func NewUUIDHandler() *UUIDHandler {
	return &UUIDHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^uuid\s*$`),
			regexp.MustCompile(`(?i)^generate\s+uuid\s*$`),
			regexp.MustCompile(`(?i)^new\s+uuid\s*$`),
			regexp.MustCompile(`(?i)^guid\s*$`),
		},
	}
}

func (h *UUIDHandler) Name() string         { return "uuid" }
func (h *UUIDHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *UUIDHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *UUIDHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	id := uuid.New()

	return &Answer{
		Type:  AnswerTypeUUID,
		Query: query,
		Title: "UUID Generator",
		Content: fmt.Sprintf(`<div class="uuid-result">
<strong>UUID v4:</strong> <code>%s</code><br>
<strong>Uppercase:</strong> <code>%s</code><br>
<strong>No dashes:</strong> <code>%s</code>
</div>`,
			id.String(),
			strings.ToUpper(id.String()),
			strings.ReplaceAll(id.String(), "-", "")),
		Data: map[string]interface{}{
			"uuid": id.String(),
		},
	}, nil
}

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

func (h *RandomHandler) Name() string         { return "random" }
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

// PasswordHandler generates secure passwords
type PasswordHandler struct {
	patterns []*regexp.Regexp
}

func NewPasswordHandler() *PasswordHandler {
	return &PasswordHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^password\s*$`),
			regexp.MustCompile(`(?i)^generate\s+password\s*$`),
			regexp.MustCompile(`(?i)^password\s+(\d+)\s*$`),
			regexp.MustCompile(`(?i)^random\s+password\s*$`),
			regexp.MustCompile(`(?i)^secure\s+password\s*$`),
		},
	}
}

func (h *PasswordHandler) Name() string         { return "password" }
func (h *PasswordHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *PasswordHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *PasswordHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	length := 16
	lengthPattern := regexp.MustCompile(`(\d+)`)
	if matches := lengthPattern.FindStringSubmatch(query); len(matches) > 1 {
		length, _ = strconv.Atoi(matches[1])
		if length < 8 {
			length = 8
		}
		if length > 128 {
			length = 128
		}
	}

	password := generatePassword(length)
	passwordNoSpecial := generatePasswordNoSpecial(length)

	return &Answer{
		Type:  AnswerTypePassword,
		Query: query,
		Title: "Password Generator",
		Content: fmt.Sprintf(`<div class="password-result">
<strong>Secure Password (%d chars):</strong><br>
<code>%s</code><br><br>
<strong>Alphanumeric Only:</strong><br>
<code>%s</code>
</div>`, length, password, passwordNoSpecial),
		Data: map[string]interface{}{
			"password": password,
			"length":   length,
		},
	}, nil
}

func generatePassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()-_=+[]{}|;:,.<>?"
	result := make([]byte, length)
	for i := range result {
		num, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		result[i] = charset[num.Int64()]
	}
	return string(result)
}

func generatePasswordNoSpecial(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		num, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		result[i] = charset[num.Int64()]
	}
	return string(result)
}

// IPHandler handles IP address lookups
type IPHandler struct {
	patterns []*regexp.Regexp
}

func NewIPHandler() *IPHandler {
	return &IPHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^my\s+ip\s*$`),
			regexp.MustCompile(`(?i)^what\s+is\s+my\s+ip\s*\??$`),
			regexp.MustCompile(`(?i)^ip\s+address\s*$`),
			regexp.MustCompile(`(?i)^ip\s+info\s*$`),
		},
	}
}

func (h *IPHandler) Name() string         { return "ip" }
func (h *IPHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *IPHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *IPHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	// Get local IPs
	var ips []string
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					ips = append(ips, ipnet.IP.String())
				}
			}
		}
	}

	var content strings.Builder
	content.WriteString("<strong>Local IP Addresses:</strong><br>")
	if len(ips) > 0 {
		for _, ip := range ips {
			content.WriteString(fmt.Sprintf("• %s<br>", ip))
		}
	} else {
		content.WriteString("Unable to determine local IP<br>")
	}
	content.WriteString("<br><em>Note: Your public IP is visible to websites you visit</em>")

	return &Answer{
		Type:    AnswerTypeIP,
		Query:   query,
		Title:   "IP Address Information",
		Content: content.String(),
		Data: map[string]interface{}{
			"local_ips": ips,
		},
	}, nil
}
