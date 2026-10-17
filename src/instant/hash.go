package instant

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"regexp"
	"strings"

	"github.com/apimgr/search/src/common/i18n"
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
			regexp.MustCompile(`(?i)^sha384[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^sha512[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^crc32[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^hash[:\s]+(.+)$`),
		},
	}
}

func (h *HashHandler) Name() string               { return "hash" }
func (h *HashHandler) Patterns() []*regexp.Regexp { return h.patterns }

func (h *HashHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *HashHandler) HandleInstantQuery(ctx context.Context, query string) (*Answer, error) {
	lowerQuery := strings.ToLower(query)
	var text string
	var hashType string

	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			text = matches[1]
			if strings.HasPrefix(lowerQuery, "md5") {
				hashType = "md5"
			} else if strings.HasPrefix(lowerQuery, "sha512") {
				hashType = "sha512"
			} else if strings.HasPrefix(lowerQuery, "sha384") {
				hashType = "sha384"
			} else if strings.HasPrefix(lowerQuery, "crc32") {
				hashType = "crc32"
			} else if strings.HasPrefix(lowerQuery, "sha256") {
				hashType = "sha256"
			} else if strings.HasPrefix(lowerQuery, "sha1") {
				hashType = "sha1"
			} else {
				// "hash: ..." prefix — check if captured text names a hash algorithm
				hashType = "all"
				lowerText := strings.ToLower(text)
				for _, algo := range []struct{ prefix, name, trim string }{
					{"md5 ", "md5", "md5 "},
					{"md5: ", "md5", "md5: "},
					{"sha512 ", "sha512", "sha512 "},
					{"sha512: ", "sha512", "sha512: "},
					{"sha384 ", "sha384", "sha384 "},
					{"sha384: ", "sha384", "sha384: "},
					{"crc32 ", "crc32", "crc32 "},
					{"crc32: ", "crc32", "crc32: "},
					{"sha256 ", "sha256", "sha256 "},
					{"sha256: ", "sha256", "sha256: "},
					{"sha1 ", "sha1", "sha1 "},
					{"sha1: ", "sha1", "sha1: "},
				} {
					if strings.HasPrefix(lowerText, algo.prefix) {
						hashType = algo.name
						text = text[len(algo.trim):]
						break
					}
				}
			}
			break
		}
	}

	if text == "" {
		return nil, nil
	}

	data := []byte(text)
	var content strings.Builder
	fmt.Fprintf(&content, "<strong>Input:</strong> %s<br><br>", escapeHTML(text))

	if hashType == "md5" || hashType == "all" {
		hash := md5.Sum(data)
		digest := hex.EncodeToString(hash[:])
		fmt.Fprintf(&content, `<strong>MD5:</strong> <code>%s</code> <button class="copy-btn" data-copy="%s">Copy</button><br>`, digest, digest)
	}
	if hashType == "sha1" || hashType == "all" {
		hash := sha1.Sum(data)
		digest := hex.EncodeToString(hash[:])
		fmt.Fprintf(&content, `<strong>SHA1:</strong> <code>%s</code> <button class="copy-btn" data-copy="%s">Copy</button><br>`, digest, digest)
	}
	if hashType == "sha256" || hashType == "all" {
		hash := sha256.Sum256(data)
		digest := hex.EncodeToString(hash[:])
		fmt.Fprintf(&content, `<strong>SHA256:</strong> <code>%s</code> <button class="copy-btn" data-copy="%s">Copy</button><br>`, digest, digest)
	}
	if hashType == "sha384" || hashType == "all" {
		hash := sha512.Sum384(data)
		digest := hex.EncodeToString(hash[:])
		fmt.Fprintf(&content, `<strong>SHA384:</strong> <code>%s</code> <button class="copy-btn" data-copy="%s">Copy</button><br>`, digest, digest)
	}
	if hashType == "sha512" || hashType == "all" {
		hash := sha512.Sum512(data)
		digest := hex.EncodeToString(hash[:])
		fmt.Fprintf(&content, `<strong>SHA512:</strong> <code>%s</code> <button class="copy-btn" data-copy="%s">Copy</button><br>`, digest, digest)
	}

	if hashType == "crc32" || hashType == "all" {
		hash := crc32.ChecksumIEEE(data)
		digest := fmt.Sprintf("%08x", hash)
		fmt.Fprintf(&content, `<strong>CRC32:</strong> <code>%s</code> <button class="copy-btn" data-copy="%s">Copy</button><br>`, digest, digest)
	}

	return &Answer{
		Type:    AnswerTypeHash,
		Query:   query,
		Title:   i18n.T(LangFromContext(ctx), "instant.hash_generator_title"),
		Content: content.String(),
	}, nil
}
