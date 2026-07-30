package pdf

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestGetBytesFromLink(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("image"))
		}))
		defer server.Close()

		got, err := GetBytesFromLink(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "image" {
			t.Fatalf("GetBytesFromLink() = %q, want image", got)
		}
	})

	t.Run("status", func(t *testing.T) {
		server := httptest.NewServer(http.NotFoundHandler())
		defer server.Close()

		if _, err := GetBytesFromLink(server.URL); err == nil {
			t.Fatal("GetBytesFromLink() accepted non-200 response")
		}
	})

	t.Run("content length", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", strconv.Itoa(maxDownloadSize+1))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		if _, err := GetBytesFromLink(server.URL); err == nil {
			t.Fatal("GetBytesFromLink() accepted oversized response")
		}
	})
}
