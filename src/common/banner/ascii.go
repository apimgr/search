package banner

// ASCII art templates for various terminal sizes
// Generated with figlet/toilet or manual design

// ArtLarge is the large ASCII art (80+ cols)
var ArtLarge = []string{
	"  ___  ___  __ _ _ __ ___| |__  ",
	" / __|/ _ \\/ _` | '__/ __| '_ \\ ",
	" \\__ \\  __/ (_| | | | (__| | | |",
	" |___/\\___|\\__,_|_|  \\___|_| |_|",
}

// ArtMedium is the medium ASCII art (60-79 cols)
var ArtMedium = []string{
	" ___ ___ __ _ _ __ ___| |__ ",
	"/ __|/ _ \\ _` | '__/ __| '_ \\",
	"\\__ \\  __/ (_| | | | (__| | | |",
	"|___/\\___|\\__,_|_|  \\___|_| |_|",
}

// ArtSmall is the small ASCII art (40-59 cols)
var ArtSmall = []string{
	"  ___ ___ __ _ _ __ ___| |__ ",
	" / __|/ _ \\ _` | '__/ __| '_ \\",
	" \\__ \\  __/_| | | (__| | | |",
	" |___/\\___|_| |_|  \\___|_| |_|",
}

// GetArt returns the appropriate ASCII art for the given width
func GetArt(cols int) []string {
	switch {
	case cols >= 80:
		return ArtLarge
	case cols >= 60:
		return ArtMedium
	case cols >= 40:
		return ArtSmall
	default:
		return nil
	}
}

// GetArtWidth returns the width of the ASCII art
func GetArtWidth(art []string) int {
	maxWidth := 0
	for _, line := range art {
		if len(line) > maxWidth {
			maxWidth = len(line)
		}
	}
	return maxWidth
}

// CenterArt centers ASCII art within the given width
func CenterArt(art []string, width int) []string {
	artWidth := GetArtWidth(art)
	if artWidth >= width {
		return art
	}

	padding := (width - artWidth) / 2
	padStr := ""
	for i := 0; i < padding; i++ {
		padStr += " "
	}

	centered := make([]string, len(art))
	for i, line := range art {
		centered[i] = padStr + line
	}
	return centered
}
