package engine

import "testing"

func TestImageFormatFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://images.example/photo.jpg?width=800", "jpg"},
		{"https://images.example/photo.jpeg", "jpg"},
		{"https://images.example/photo.PNG", "png"},
		{"https://images.example/photo.webp", "webp"},
		{"https://images.example/photo.gif", "gif"},
		{"https://images.example/photo.svg", ""},
		{"not a URL", ""},
	}
	for _, test := range tests {
		t.Run(test.url, func(t *testing.T) {
			if got := imageFormatFromURL(test.url); got != test.want {
				t.Errorf("imageFormatFromURL(%q) = %q, want %q", test.url, got, test.want)
			}
		})
	}
}
