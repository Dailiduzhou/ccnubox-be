package reptile

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestReptileRejectsOversizedHTML(t *testing.T) {
	oversizedHTML := bytes.Repeat([]byte{'x'}, maxHTMLSize+1)
	originalClient := client
	client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(oversizedHTML)),
				Request:    req,
			}, nil
		}),
	}
	t.Cleanup(func() {
		client = originalClient
	})

	r := NewReptile()
	if _, err := r.GetCalendarLink(); err == nil {
		t.Fatal("GetCalendarLink() accepted oversized HTML")
	}
	if _, _, err := r.FetchPDFOrImageLinksFromPage("https://example.com"); err == nil {
		t.Fatal("FetchPDFOrImageLinksFromPage() accepted oversized HTML")
	}
}
