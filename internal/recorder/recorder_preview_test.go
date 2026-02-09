package recorder

import (
	"testing"
)

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"Valid HTTPS", "https://google.com", false},
		{"Valid HTTP", "http://example.com", false},
		{"Localhost", "http://localhost", true},
		{"Localhost IP", "http://127.0.0.1", true},
		{"Private IP 10.x", "http://10.0.0.1", true},
		{"Private IP 192.168.x", "http://192.168.1.1", true},
		{"Private IP 172.16.x", "http://172.16.0.1", true},
		{"IPv6 Loopback", "http://[::1]", true},
		{"File Scheme", "file:///etc/passwd", true},
		{"Invalid Scheme", "ftp://example.com", true},
		{"Invalid URL", "not-a-url", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}
