package crawler

import (
	"encoding/json"
	"net/http"

	"github.com/asynccnu/ccnubox-be/common/pkg/errorx"
	"github.com/asynccnu/ccnubox-be/common/pkg/httpx"
)

func readSuccessfulResponse(resp *http.Response) ([]byte, error) {
	body, err := httpx.ReadResponse(resp)
	if err != nil {
		return nil, errorx.Errorf("read upstream response: %w", err)
	}

	var envelope Response
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, errorx.Errorf("decode upstream response: %w", err)
	}
	if !envelope.Status || (envelope.Code != 0 && envelope.Code != http.StatusOK) {
		return nil, errorx.Errorf("upstream rejected request, code=%d message=%s", envelope.Code, envelope.Message)
	}
	return body, nil
}
