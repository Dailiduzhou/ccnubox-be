package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	b_errorx "github.com/asynccnu/ccnubox-be/bff/pkg/errorx"
)

func TestFindCustomErrorTraversesEntireChain(t *testing.T) {
	want := b_errorx.New(500, 50101, "banner failed")
	wrapped := fmt.Errorf("outer: %w", fmt.Errorf("middle: %w", want))

	got, ok := findCustomError(wrapped)
	if !ok {
		t.Fatal("CustomError was not found in a nested error chain")
	}
	var expected *b_errorx.CustomError
	if !errors.As(want, &expected) {
		t.Fatal("test setup did not create a CustomError")
	}
	if got != expected {
		t.Fatalf("got CustomError %p, want %p", got, expected)
	}
}

func TestRedactedHeadersDoesNotLeakCredentials(t *testing.T) {
	headers := http.Header{
		"Authorization":       []string{"Bearer secret-token"},
		"Cookie":              []string{"session=secret"},
		"Proxy-Authorization": []string{"Basic secret"},
		"X-Request-Id":        []string{"request-1"},
	}

	got := redactedHeaders(headers)
	for _, name := range []string{"Authorization", "Cookie", "Proxy-Authorization"} {
		if value := got.Get(name); value != "[REDACTED]" {
			t.Fatalf("%s got %q, want redacted", name, value)
		}
	}
	if got.Get("X-Request-Id") != "request-1" {
		t.Fatal("non-sensitive header was unexpectedly changed")
	}
	if headers.Get("Authorization") != "Bearer secret-token" {
		t.Fatal("source headers were mutated")
	}
}

func TestFindCustomErrorRejectsUnknownError(t *testing.T) {
	if got, ok := findCustomError(errors.New("unknown")); ok || got != nil {
		t.Fatalf("findCustomError() = (%v, %v), want (nil, false)", got, ok)
	}
}
