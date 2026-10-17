package qr

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func TestPNGCreatesRequestedDimensions(t *testing.T) {
	for _, size := range []int{128, 256, 512, 1024} {
		data, err := PNG("https://example.com", "M", size)
		if err != nil {
			t.Fatalf("PNG(%d): %v", size, err)
		}
		image, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("decode PNG(%d): %v", size, err)
		}
		if image.Bounds().Dx() != size || image.Bounds().Dy() != size {
			t.Errorf("PNG(%d) dimensions = %v", size, image.Bounds())
		}
	}
}

func TestSVGUsesGeneratedMatrixAndSafeColors(t *testing.T) {
	data, err := SVG("WIFI:T:WPA;S:Network;P:secret;;", "H", "#123abc", "#ffffff")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, fragment := range []string{`<svg `, `viewBox=`, `fill="#ffffff"`, `fill="#123abc"`, `M`} {
		if !strings.Contains(text, fragment) {
			t.Errorf("SVG missing %q", fragment)
		}
	}
	if strings.Contains(text, "secret") {
		t.Fatal("SVG should encode content in modules, not expose it as markup text")
	}
}

func TestRejectsInvalidLevelSizeAndColors(t *testing.T) {
	if _, err := PNG("text", "X", 256); err == nil {
		t.Fatal("unsupported recovery level accepted")
	}
	if _, err := PNG("text", "M", 4096); err == nil {
		t.Fatal("oversized image accepted")
	}
	for _, colors := range [][2]string{{`#000000"><script>`, "#ffffff"}, {"#000000", "white"}} {
		if _, err := SVG("text", "M", colors[0], colors[1]); err == nil {
			t.Errorf("unsafe colors accepted: %#v", colors)
		}
	}
	if _, err := SVG(strings.Repeat("x", 4097), "M", "#000000", "#ffffff"); err == nil {
		t.Fatal("oversized QR text accepted")
	}
}

func TestPNGWithColorsUsesRequestedPalette(t *testing.T) {
	data, err := PNGWithColors("palette fixture", "Q", 128, "#123ABC", "#F0E1D2")
	if err != nil {
		t.Fatal(err)
	}
	image, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if got := image.At(0, 0); got != (color.RGBA{R: 240, G: 225, B: 210, A: 255}) {
		t.Errorf("background pixel = %#v, want requested background", got)
	}
}

func TestPNGWithColorsRejectsUnsafePalette(t *testing.T) {
	if _, err := PNGWithColors("text", "M", 128, `#000000"><svg>`, "#FFFFFF"); err == nil {
		t.Fatal("unsafe foreground color accepted")
	}
}

func TestPNGWithColorsRejectsEmptyAndOversizedText(t *testing.T) {
	for _, text := range []string{"", strings.Repeat("x", 4097)} {
		if _, err := PNGWithColors(text, "M", 128, "#000000", "#FFFFFF"); err == nil {
			t.Errorf("invalid text length %d was accepted", len(text))
		}
	}
}

func TestPNGWithLogoEmbedsBoundedRasterImage(t *testing.T) {
	logo := testLogoPNG()
	data, err := PNGWithLogo("logo fixture", "H", 256, "#000000", "#FFFFFF", logo)
	if err != nil {
		t.Fatal(err)
	}
	image, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if image.Bounds().Dx() != 256 || image.Bounds().Dy() != 256 {
		t.Fatalf("QR image dimensions = %v, want 256x256", image.Bounds())
	}
	center := color.RGBAModel.Convert(image.At(128, 128)).(color.RGBA)
	if center.R < 200 || center.G > 80 || center.B > 80 {
		t.Fatalf("center pixel = %#v; expected sanitized red logo", center)
	}
}

func TestSVGWithLogoEmbedsReencodedPNG(t *testing.T) {
	data, err := SVGWithLogo("logo fixture", "H", "#000000", "#FFFFFF", testLogoPNG())
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `data:image/png;base64,`) || !strings.Contains(text, `<image `) {
		t.Fatal("SVG did not embed the sanitized raster logo")
	}
	if strings.Contains(text, "<script") || strings.Contains(text, "testLogoPNG") {
		t.Fatal("SVG output contains untrusted source markup")
	}
}

func TestLogoRejectsUnsupportedAndOversizedImages(t *testing.T) {
	if _, err := PNGWithLogo("text", "H", 128, "#000000", "#FFFFFF", []byte("not an image")); err == nil {
		t.Fatal("invalid image accepted as QR logo")
	}
	if _, err := PNGWithLogo("text", "H", 128, "#000000", "#FFFFFF", bytes.Repeat([]byte{0}, 1<<20+1)); err == nil {
		t.Fatal("oversized logo accepted")
	}
}

func testLogoPNG() []byte {
	canvas := image.NewRGBA(image.Rect(0, 0, 20, 12))
	for y := 0; y < 12; y++ {
		for x := 0; x < 20; x++ {
			canvas.SetRGBA(x, y, color.RGBA{R: 240, G: 20, B: 20, A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, canvas); err != nil {
		panic(err)
	}
	return buffer.Bytes()
}
