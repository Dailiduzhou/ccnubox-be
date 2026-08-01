package httpx

import (
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"slices"
)

const (
	// DefaultMaxBodyBytes is deliberately large enough for course and classroom
	// responses while still preventing an upstream from exhausting process memory.
	DefaultMaxBodyBytes int64 = 8 << 20
)

var (
	ErrBodyTooLarge        = errors.New("http response body exceeds limit")
	ErrUnexpectedStatus    = errors.New("unexpected HTTP response status")
	ErrUnexpectedMediaType = errors.New("unexpected HTTP response media type")
	ErrInvalidResponse     = errors.New("invalid HTTP response")
)

type responseConfig struct {
	maxBodyBytes     int64
	acceptedStatuses func(int) bool
	acceptedTypes    []string
}

// ResponseOption adjusts validation without changing the default crawler
// behaviour. Media type checks are opt-in because several campus systems send
// valid JSON or HTML with missing or incorrect Content-Type headers.
type ResponseOption func(*responseConfig)

func WithMaxBodyBytes(maxBytes int64) ResponseOption {
	return func(cfg *responseConfig) {
		cfg.maxBodyBytes = maxBytes
	}
}

func WithAcceptedStatuses(accept func(statusCode int) bool) ResponseOption {
	return func(cfg *responseConfig) {
		cfg.acceptedStatuses = accept
	}
}

func WithAcceptedMediaTypes(mediaTypes ...string) ResponseOption {
	return func(cfg *responseConfig) {
		cfg.acceptedTypes = append([]string(nil), mediaTypes...)
	}
}

// ReadResponse validates an HTTP response and reads at most the configured
// number of bytes. The caller still owns resp.Body and must close it.
func ReadResponse(resp *http.Response, options ...ResponseOption) ([]byte, error) {
	cfg := responseConfig{
		maxBodyBytes: DefaultMaxBodyBytes,
		acceptedStatuses: func(statusCode int) bool {
			return statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices
		},
	}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}

	if resp == nil || resp.Body == nil {
		return nil, ErrInvalidResponse
	}
	if cfg.maxBodyBytes <= 0 {
		return nil, fmt.Errorf("%w: body limit must be positive", ErrInvalidResponse)
	}
	if cfg.acceptedStatuses != nil && !cfg.acceptedStatuses(resp.StatusCode) {
		return nil, fmt.Errorf("%w: %d", ErrUnexpectedStatus, resp.StatusCode)
	}
	if resp.ContentLength > cfg.maxBodyBytes {
		return nil, fmt.Errorf("%w: content-length=%d limit=%d", ErrBodyTooLarge, resp.ContentLength, cfg.maxBodyBytes)
	}
	if len(cfg.acceptedTypes) > 0 {
		mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
		if err != nil || !slices.Contains(cfg.acceptedTypes, mediaType) {
			return nil, fmt.Errorf("%w: %q", ErrUnexpectedMediaType, resp.Header.Get("Content-Type"))
		}
	}

	return ReadLimited(resp.Body, cfg.maxBodyBytes)
}

// ReadLimited handles both known Content-Length and streamed/chunked bodies.
// Reading one extra byte is required to distinguish an exactly-full response
// from a truncated one.
func ReadLimited(reader io.Reader, maxBytes int64) ([]byte, error) {
	if reader == nil || maxBytes <= 0 {
		return nil, fmt.Errorf("%w: reader and positive body limit are required", ErrInvalidResponse)
	}
	if maxBytes == math.MaxInt64 {
		return nil, fmt.Errorf("%w: body limit is too large", ErrInvalidResponse)
	}

	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%w: limit=%d", ErrBodyTooLarge, maxBytes)
	}
	return data, nil
}
