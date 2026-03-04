package discord

import (
	"testing"
)

func TestMimeExtension(t *testing.T) {
	tests := []struct {
		mime string
		want string
	}{
		{"image/png", ".png"},
		{"image/jpeg", ".jpg"},
		{"image/gif", ".gif"},
		{"video/mp4", ".mp4"},
		{"audio/mpeg", ".mp3"},
		{"application/pdf", ".pdf"},
		{"unknown/type", ""},
	}

	for _, tt := range tests {
		t.Run(tt.mime, func(t *testing.T) {
			got := mimeExtension(tt.mime)
			if got != tt.want {
				t.Errorf("mimeExtension(%q) = %q, want %q", tt.mime, got, tt.want)
			}
		})
	}
}

func TestBase64DataURLToBytes(t *testing.T) {
	// Test valid data URL
	data, err := base64DataURLToBytes("data:text/plain;base64,SGVsbG8=")
	if err != nil {
		t.Errorf("base64DataURLToBytes() error = %v", err)
	}
	if string(data) != "Hello" {
		t.Errorf("base64DataURLToBytes() = %q, want %q", string(data), "Hello")
	}

	// Test invalid data URL
	_, err = base64DataURLToBytes("invalid")
	if err == nil {
		t.Error("base64DataURLToBytes() expected error for invalid URL")
	}
}
