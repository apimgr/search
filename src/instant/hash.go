package instant

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

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

func (h *HashHandler) Name() string                { return "hash" }
func (h *HashHandler) Patterns() []*regexp.Regexp  { return h.patterns }

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
