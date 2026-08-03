package repository

import (
	"strings"
	"testing"
)

func TestCrawlerIndexStaysOutsideFilebeatNamespace(t *testing.T) {
	if strings.HasPrefix(freeClassroomCrawlerIndex, "ccnubox-") {
		t.Fatalf("crawler index %q conflicts with the ccnubox-* Filebeat data-stream template", freeClassroomCrawlerIndex)
	}
}
