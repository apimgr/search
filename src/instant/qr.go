package instant

import (
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"

	"github.com/skip2/go-qrcode"
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

func (h *QRHandler) Handle(ctx context.Context, query string) (*Answer, error) {
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

	// Generate QR code as PNG
	png, err := qrcode.Encode(text, qrcode.Medium, 256)
	if err != nil {
		return &Answer{
			Type:    AnswerTypeQR,
			Query:   query,
			Title:   "QR Code Generator",
			Content: fmt.Sprintf("Error generating QR code: %v", err),
		}, nil
	}

	// Encode as base64 for display
	b64 := base64.StdEncoding.EncodeToString(png)

	// Generate ASCII representation
	ascii := generateQRASCII(text)

	content := fmt.Sprintf(`<div class="qr-result">
<strong>Text:</strong> %s<br><br>
<img src="data:image/png;base64,%s" alt="QR Code" style="image-rendering: pixelated;"><br><br>
<details>
<summary>ASCII Version</summary>
<pre style="font-family: monospace; line-height: 1; font-size: 8px;">%s</pre>
</details>
</div>`, text, b64, ascii)

	return &Answer{
		Type:    AnswerTypeQR,
		Query:   query,
		Title:   "QR Code Generator",
		Content: content,
		Data: map[string]interface{}{
			"text":       text,
			"image_b64":  b64,
			"image_type": "png",
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
