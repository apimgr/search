package instant

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/common/qr"
	qrcode "github.com/skip2/go-qrcode"
)

// QRHandler handles QR code generation
type QRHandler struct {
	patterns []*regexp.Regexp
}

// NewQRHandler creates a new QR code handler
func NewQRHandler() *QRHandler {
	return &QRHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^qr[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^qrcode[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^generate\s+qr[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^qr\s+code[:\s]+(.+)$`),
		},
	}
}

func (h *QRHandler) Name() string {
	return "qr"
}

func (h *QRHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *QRHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *QRHandler) HandleInstantQuery(ctx context.Context, query string) (*Answer, error) {
	// Extract text from query
	text := ""
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			text = strings.TrimSpace(matches[1])
			break
		}
	}

	if text == "" {
		return nil, nil
	}

	lang := LangFromContext(ctx)

	imageSizes := []int{128, 256, 512, 1024}
	pngImages := make(map[int]string, len(imageSizes))
	for _, size := range imageSizes {
		png, err := qr.PNG(text, "M", size)
		if err != nil {
			return &Answer{
				Type: AnswerTypeQR, Query: query, Title: i18n.T(lang, "instant.qr_generator_title"),
				Content: i18n.T(lang, "instant.qr_generate_error", err),
			}, nil
		}
		pngImages[size] = base64.StdEncoding.EncodeToString(png)
	}
	vector, err := qr.SVG(text, "M", "#000000", "#FFFFFF")
	if err != nil {
		return nil, err
	}

	mainPNG := pngImages[256]
	ascii := generateQRASCII(text)
	label := i18n.T(lang, "instant.qr_generator_title")
	var content strings.Builder
	fmt.Fprintf(&content, `<div class="qr-result"><strong>Text:</strong> %s<br><br>`, escapeHTML(text))
	if parsed, err := url.ParseRequestURI(text); err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" && parsed.User == nil {
		fmt.Fprintf(&content, `<p><a href="%s" rel="nofollow noopener noreferrer">%s</a></p>`, escapeHTML(parsed.String()), escapeHTML(parsed.String()))
	}
	fmt.Fprintf(&content, `<img src="data:image/png;base64,%s" alt="%s" class="qr-code-preview" width="256" height="256"><br>`, mainPNG, escapeHTML(label))
	fmt.Fprintf(&content, `<div class="qr-download-options"><strong>%s</strong> `, escapeHTML(i18n.T(lang, "direct.qr_download_png")))
	for _, size := range imageSizes {
		fmt.Fprintf(&content, `<a href="data:image/png;base64,%s" download="qr-%d.png">%s</a> `, pngImages[size], size, escapeHTML(i18n.T(lang, "direct.qr_size_pixels", size)))
	}
	fmt.Fprintf(&content, `<br><a href="data:image/svg+xml;base64,%s" download="qr.svg">%s</a></div>`, base64.StdEncoding.EncodeToString(vector), escapeHTML(i18n.T(lang, "direct.qr_download_svg")))
	fmt.Fprintf(&content, `<details><summary>%s</summary><pre class="qr-ascii">%s</pre></details></div>`, i18n.T(lang, "instant.qr_ascii_version"), escapeHTML(ascii))
	contentHTML := content.String()

	return &Answer{
		Type:    AnswerTypeQR,
		Query:   query,
		Title:   i18n.T(lang, "instant.qr_generator_title"),
		Content: contentHTML,
		Data: map[string]interface{}{
			"text":       text,
			"image_b64":  mainPNG,
			"image_type": "png",
			"png_sizes":  pngImages,
			"svg_b64":    base64.StdEncoding.EncodeToString(vector),
		},
	}, nil
}

// generateQRASCII creates an ASCII representation of the QR code
func generateQRASCII(text string) string {
	qr, err := qrcode.New(text, qrcode.Medium)
	if err != nil {
		return ""
	}

	bitmap := qr.Bitmap()
	var sb strings.Builder

	for _, row := range bitmap {
		for _, cell := range row {
			if cell {
				sb.WriteString("##")
			} else {
				sb.WriteString("  ")
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
