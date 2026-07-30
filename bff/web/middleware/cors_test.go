package middleware

import "testing"

func TestIsAllowedOrigin(t *testing.T) {
	tests := []struct {
		name   string
		origin string
		want   bool
	}{
		{name: "localhost", origin: "http://localhost:3000", want: true},
		{name: "loopback", origin: "http://127.0.0.1:5173", want: true},
		{name: "root domain", origin: "https://muxixyz.com", want: true},
		{name: "subdomain", origin: "https://ccnubox.muxixyz.com", want: true},
		{name: "lookalike suffix", origin: "https://evil-muxixyz.com", want: false},
		{name: "lookalike prefix", origin: "https://muxixyz.com.evil.example", want: false},
		{name: "arbitrary origin", origin: "https://example.com", want: false},
		{name: "missing scheme", origin: "muxixyz.com", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAllowedOrigin(tt.origin); got != tt.want {
				t.Fatalf("isAllowedOrigin(%q) = %v, want %v", tt.origin, got, tt.want)
			}
		})
	}
}
