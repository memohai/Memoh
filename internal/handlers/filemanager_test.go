package handlers

import "testing"

func TestIsContainerMediaPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "/data/media", want: true},
		{path: "/data/media/0f/demo.jpg", want: true},
		{path: "data/media/0f/demo.jpg", want: true},
		{path: "/data/mediakit/demo.jpg", want: false},
		{path: "/etc/passwd", want: false},
	}

	for _, tt := range tests {
		if got := isContainerMediaPath(tt.path); got != tt.want {
			t.Fatalf("isContainerMediaPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
