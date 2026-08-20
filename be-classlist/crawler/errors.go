package crawler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/asynccnu/ccnubox-be/be-classlist/biz"
	"github.com/asynccnu/ccnubox-be/common/pkg/httpx"
)

// 分类HTTP响应的错误
// 复用了httpx包的哨兵错误
func classifyResponseError(resp *http.Response, err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, httpx.ErrUnexpectedStatus) && resp != nil {
		statusCode := resp.StatusCode
		switch {
		case statusCode >= http.StatusMultipleChoices && statusCode < http.StatusBadRequest,
			statusCode == http.StatusUnauthorized,
			statusCode == http.StatusForbidden:
			return fmt.Errorf("%w: status=%d: %w", biz.ErrCrawlerAuthentication, statusCode, err)
		case statusCode == http.StatusRequestTimeout,
			statusCode == http.StatusTooEarly,
			statusCode == http.StatusTooManyRequests,
			statusCode >= http.StatusInternalServerError:
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

	// Body 读取时的 EOF、连接重置等错误保留原类型，由 usecase 的 net.Error/io 错误分类处理。
	return err
}
