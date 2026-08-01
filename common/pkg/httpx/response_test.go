package httpx

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestReadResponse(t *testing.T) {
	tests := []struct {
		name    string
		resp    *http.Response
		options []ResponseOption
		want    string
		wantErr error
	}{
		{name: "successful response", resp: response(http.StatusOK, "application/json; charset=utf-8", `{"ok":true}`), want: `{"ok":true}`},
		{name: "status is rejected before parsing", resp: response(http.StatusBadGateway, "text/plain", "upstream failed"), wantErr: ErrUnexpectedStatus},
		{name: "streamed body is bounded", resp: response(http.StatusOK, "application/json", "12345"), options: []ResponseOption{WithMaxBodyBytes(4)}, wantErr: ErrBodyTooLarge},
		{name: "media type parameters are ignored", resp: response(http.StatusOK, "application/json; charset=utf-8", `{}`), options: []ResponseOption{WithAcceptedMediaTypes("application/json")}, want: `{}`},
		{name: "unexpected media type is rejected", resp: response(http.StatusOK, "text/html", "<html></html>"), options: []ResponseOption{WithAcceptedMediaTypes("application/json")}, wantErr: ErrUnexpectedMediaType},
		{name: "custom status policy", resp: response(http.StatusNotModified, "", "cached"), options: []ResponseOption{WithAcceptedStatuses(func(statusCode int) bool { return statusCode == http.StatusNotModified })}, want: "cached"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadResponse(tt.resp, tt.options...)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ReadResponse() error = %v, want %v", err, tt.wantErr)
			}
			if string(got) != tt.want {
				t.Fatalf("ReadResponse() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadLimitedAcceptsExactLimit(t *testing.T) {
	got, err := ReadLimited(strings.NewReader("1234"), 4)
	if err != nil {
		t.Fatalf("ReadLimited() error = %v", err)
	}
	if string(got) != "1234" {
		t.Fatalf("ReadLimited() = %q", got)
	}
}

func TestReadResponseRejectsDeclaredOversizeBody(t *testing.T) {
	resp := response(http.StatusOK, "application/json", "{}")
	resp.ContentLength = 10
	_, err := ReadResponse(resp, WithMaxBodyBytes(4))
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("ReadResponse() error = %v, want %v", err, ErrBodyTooLarge)
	}
}

func TestReadResponseRejectsInvalidResponse(t *testing.T) {
	_, err := ReadResponse(nil)
	if !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("ReadResponse(nil) error = %v", err)
	}
}

func response(status int, contentType, body string) *http.Response {
	header := make(http.Header)
	if contentType != "" {
		header.Set("Content-Type", contentType)
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body))}
}
