package qr

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"golang.org/x/image/draw"
	"image"
	"image/color"
	"image/png"
	"regexp"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
	_ "image/jpeg"
)

var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// RecoveryLevel maps the public QR recovery-level names to the encoder setting.
func RecoveryLevel(level string) (qrcode.RecoveryLevel, error) {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "L", "LOW":
		return qrcode.Low, nil
	case "M", "MEDIUM", "":
		return qrcode.Medium, nil
	case "Q", "QUARTILE":
		return qrcode.High, nil
	case "H", "HIGH":
		return qrcode.Highest, nil
	default:
		return 0, fmt.Errorf("unsupported QR error-correction level %q", level)
	}
}

// PNG creates a bounded PNG QR code using one of the supported recovery levels.
func PNG(text, level string, size int) ([]byte, error) {
	return PNGWithColors(text, level, size, "#000000", "#FFFFFF")
}

// PNGWithColors creates a bounded PNG QR code with validated six-digit RGB colors.
func PNGWithColors(text, level string, size int, foreground, background string) ([]byte, error) {
	if len(text) == 0 || len(text) > 4096 {
		return nil, fmt.Errorf("QR text must contain between 1 and 4096 bytes")
	}
	if size < 64 || size > 2048 {
		return nil, fmt.Errorf("QR image size must be between 64 and 2048 pixels")
	}
	if !ValidColor(foreground) || !ValidColor(background) {
		return nil, fmt.Errorf("QR colors must be six-digit RGB hex values")
	}
	recovery, err := RecoveryLevel(level)
	if err != nil {
		return nil, err
	}
	code, err := qrcode.New(text, recovery)
	if err != nil {
		return nil, err
	}
	code.ForegroundColor = parseColor(foreground)
	code.BackgroundColor = parseColor(background)
	return code.PNG(size)
}

func parseColor(value string) color.Color {
	decoded, _ := hex.DecodeString(value[1:])
	return color.RGBA{R: decoded[0], G: decoded[1], B: decoded[2], A: 0xff}
}

// PNGWithLogo adds a bounded PNG or JPEG logo to the center of a QR code.
// The logo is decoded and re-encoded before composition, so uploaded metadata
// and active image formats are never copied into the result.
func PNGWithLogo(text, level string, size int, foreground, background string, logo []byte) ([]byte, error) {
	qrPNG, err := PNGWithColors(text, level, size, foreground, background)
	if err != nil || len(logo) == 0 {
		return qrPNG, err
	}
	canvas, err := png.Decode(bytes.NewReader(qrPNG))
	if err != nil {
		return nil, fmt.Errorf("decode generated QR image: %w", err)
	}
	logoImage, err := decodeLogo(logo)
	if err != nil {
		return nil, err
	}
	logoPNG, err := composeLogo(canvas, logoImage, parseColor(background))
	return logoPNG, err
}

// SVGWithLogo embeds a sanitized PNG logo inside the generated vector image.
func SVGWithLogo(text, level, foreground, background string, logo []byte) ([]byte, error) {
	vector, err := SVG(text, level, foreground, background)
	if err != nil || len(logo) == 0 {
		return vector, err
	}
	logoImage, err := decodeLogo(logo)
	if err != nil {
		return nil, err
	}
	recovery, err := RecoveryLevel(level)
	if err != nil {
		return nil, err
	}
	qrCode, err := qrcode.New(text, recovery)
	if err != nil {
		return nil, err
	}
	modules := len(qrCode.Bitmap())
	if modules == 0 || modules > 2048 {
		return nil, fmt.Errorf("QR module matrix has an unsupported size")
	}
	logoWidth := modules / 5
	if logoWidth < 1 {
		return nil, fmt.Errorf("QR module matrix is too small for a logo")
	}
	logoPNG, err := encodeLogo(logoImage, logoWidth)
	if err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(logoPNG)
	pad := logoWidth / 12
	if pad < 1 {
		pad = 1
	}
	x := (modules - logoWidth) / 2
	y := x
	imageMarkup := fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" fill="#ffffff"/><image x="%d" y="%d" width="%d" height="%d" href="data:image/png;base64,%s" preserveAspectRatio="xMidYMid meet"/>`, x-pad, y-pad, logoWidth+2*pad, logoWidth+2*pad, x, y, logoWidth, logoWidth, encoded)
	return []byte(strings.Replace(string(vector), "</svg>", imageMarkup+"</svg>", 1)), nil
}

// ValidateLogo checks the size, format, and dimensions accepted for QR logos.
func ValidateLogo(data []byte) error {
	_, err := decodeLogo(data)
	return err
}

func decodeLogo(data []byte) (image.Image, error) {
	if len(data) == 0 || len(data) > 1<<20 {
		return nil, fmt.Errorf("QR logo must be between 1 byte and 1 MiB")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg") {
		return nil, fmt.Errorf("QR logo must be a valid PNG or JPEG image")
	}
	if config.Width < 1 || config.Height < 1 || config.Width > 512 || config.Height > 512 {
		return nil, fmt.Errorf("QR logo dimensions must not exceed 512 by 512 pixels")
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode QR logo: %w", err)
	}
	return decoded, nil
}

func composeLogo(qrImage image.Image, logo image.Image, background color.Color) ([]byte, error) {
	bounds := qrImage.Bounds()
	canvas := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(canvas, canvas.Bounds(), qrImage, bounds.Min, draw.Src)
	logoWidth, logoHeight := fitDimensions(logo.Bounds().Dx(), logo.Bounds().Dy(), bounds.Dx()/5)
	if logoWidth < 1 || logoHeight < 1 {
		return nil, fmt.Errorf("QR logo dimensions are unsupported")
	}
	pad := bounds.Dx() / 80
	if pad < 2 {
		pad = 2
	}
	centerX, centerY := bounds.Dx()/2, bounds.Dy()/2
	box := image.Rect(centerX-logoWidth/2-pad, centerY-logoHeight/2-pad, centerX+(logoWidth+1)/2+pad, centerY+(logoHeight+1)/2+pad)
	draw.Draw(canvas, box, &image.Uniform{C: background}, image.Point{}, draw.Src)
	logoRect := image.Rect(centerX-logoWidth/2, centerY-logoHeight/2, centerX+(logoWidth+1)/2, centerY+(logoHeight+1)/2)
	draw.CatmullRom.Scale(canvas, logoRect, logo, logo.Bounds(), draw.Over, nil)
	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil {
		return nil, fmt.Errorf("encode QR image with logo: %w", err)
	}
	return output.Bytes(), nil
}

func fitDimensions(width, height, maximumSide int) (int, int) {
	if width < 1 || height < 1 || maximumSide < 1 {
		return 0, 0
	}
	scale := float64(maximumSide) / float64(max(width, height))
	fitWidth := max(1, int(float64(width)*scale))
	fitHeight := max(1, int(float64(height)*scale))
	return fitWidth, fitHeight
}

func encodeLogo(logo image.Image, maximumSide int) ([]byte, error) {
	width, height := fitDimensions(logo.Bounds().Dx(), logo.Bounds().Dy(), maximumSide)
	if width < 1 || height < 1 {
		return nil, fmt.Errorf("QR logo dimensions are unsupported")
	}
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(canvas, canvas.Bounds(), logo, logo.Bounds(), draw.Over, nil)
	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil {
		return nil, fmt.Errorf("encode sanitized QR logo: %w", err)
	}
	return output.Bytes(), nil
}

// SVG creates a vector QR image from a generated module matrix. Colors are
// restricted to six-digit RGB values so user input cannot inject SVG markup.
func SVG(text, level, foreground, background string) ([]byte, error) {
	if len(text) == 0 || len(text) > 4096 {
		return nil, fmt.Errorf("QR text must contain between 1 and 4096 bytes")
	}
	if !colorPattern.MatchString(foreground) || !colorPattern.MatchString(background) {
		return nil, fmt.Errorf("QR colors must be six-digit RGB hex values")
	}
	recovery, err := RecoveryLevel(level)
	if err != nil {
		return nil, err
	}
	code, err := qrcode.New(text, recovery)
	if err != nil {
		return nil, err
	}
	bitmap := code.Bitmap()
	if len(bitmap) == 0 || len(bitmap) > 2048 {
		return nil, fmt.Errorf("QR module matrix has an unsupported size")
	}
	var svg bytes.Buffer
	width := len(bitmap)
	fmt.Fprintf(&svg, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges"><rect width="100%%" height="100%%" fill="%s"/><path fill="%s" d="`, width, width, strings.ToLower(background), strings.ToLower(foreground))
	for y, row := range bitmap {
		for x, dark := range row {
			if dark {
				fmt.Fprintf(&svg, "M%d %dh1v1h-1z", x, y)
			}
		}
	}
	svg.WriteString(`"/></svg>`)
	return svg.Bytes(), nil
}

// ValidColor reports whether a color can safely be used by SVG().
func ValidColor(value string) bool {
	if !colorPattern.MatchString(value) {
		return false
	}
	decoded := make([]byte, 3)
	_, err := hex.Decode(decoded, []byte(value[1:]))
	return err == nil
}
