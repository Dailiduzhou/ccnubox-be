package crawler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/asynccnu/ccnubox-be/be-classlist/biz"
	"github.com/asynccnu/ccnubox-be/common/pkg/httpx"
)

const maxAuthenticationPageInspectionBytes = 64 << 10

// 分类HTTP响应的错误
// 复用了httpx包的哨兵错误
func classifyResponseError(resp *http.Response, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	if errors.Is(err, httpx.ErrUnexpectedStatus) && resp != nil {
		statusCode := resp.StatusCode
		switch {
		case statusCode == http.StatusUnauthorized,
			statusCode == http.StatusForbidden:
			return fmt.Errorf("%w: status=%d: %w", biz.ErrCrawlerAuthentication, statusCode, err)
		case isHTTPRedirect(statusCode) && isAuthenticationRedirect(resp):
			return fmt.Errorf("%w: status=%d: %w", biz.ErrCrawlerAuthentication, statusCode, err)
		case statusCode == http.StatusProxyAuthRequired,
			statusCode == http.StatusRequestTimeout,
			statusCode == http.StatusMisdirectedRequest,
			statusCode == http.StatusTooEarly,
			statusCode == http.StatusTooManyRequests,
			statusCode >= http.StatusInternalServerError && statusCode <= 599:
			return fmt.Errorf("%w: status=%d: %w", biz.ErrCrawlerTemporary, statusCode, err)
		default:
			return fmt.Errorf("%w: status=%d: %w", biz.ErrCrawlerProtocol, statusCode, err)
		}
	}

	if errors.Is(err, httpx.ErrBodyTooLarge) ||
		errors.Is(err, httpx.ErrUnexpectedMediaType) ||
		errors.Is(err, httpx.ErrInvalidResponse) {
		return fmt.Errorf("%w: %w", biz.ErrCrawlerProtocol, err)
	}

	return fmt.Errorf("%w: %w", biz.ErrCrawlerTemporary, err)
}

func classifyPayloadError(body []byte, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, biz.ErrCrawlerAuthentication) ||
		errors.Is(err, biz.ErrCrawlerProtocol) ||
		errors.Is(err, biz.ErrCrawlerEmptyResult) {
		return err
	}
	if looksLikeAuthenticationPage(body) {
		return fmt.Errorf("%w: upstream returned an authentication page: %w", biz.ErrCrawlerAuthentication, err)
	}
	return fmt.Errorf("%w: %w", biz.ErrCrawlerProtocol, err)
}

func isHTTPRedirect(statusCode int) bool {
	return statusCode >= http.StatusMultipleChoices && statusCode < http.StatusBadRequest
}

func isAuthenticationRedirect(resp *http.Response) bool {
	if resp == nil || !isHTTPRedirect(resp.StatusCode) {
		return false
	}

	location := strings.ToLower(strings.TrimSpace(resp.Header.Get("Location")))
	if location == "" {
		return resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusSeeOther
	}
	for _, marker := range []string{"login", "authserver", "passport", "/cas", "sso"} {
		if strings.Contains(location, marker) {
			return true
		}
	}
	return false
}

func looksLikeAuthenticationPage(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	if len(body) > maxAuthenticationPageInspectionBytes {
		body = body[:maxAuthenticationPageInspectionBytes]
	}
	sample := bytes.ToLower(body)
	if !bytes.Contains(sample, []byte("<html")) &&
		!bytes.Contains(sample, []byte("<!doctype html")) &&
		!bytes.Contains(sample, []byte("<form")) {
		return false
	}

	markers := [][]byte{
		[]byte("login"),
		[]byte("/cas"),
		[]byte("sso"),
		[]byte("name=\"password\""),
		[]byte("name='password'"),
		[]byte("统一身份认证"),
		[]byte("登录"),
	}
	for _, marker := range markers {
		if bytes.Contains(sample, marker) {
			return true
		}
	}
	return false
}
