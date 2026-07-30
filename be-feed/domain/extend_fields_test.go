package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestExtendFieldsGetMissingKey(t *testing.T) {
	const key = "missing"

	value := ExtendFields{}.Get(key)
	if !errors.Is(value.Err, errKeyNotFound) {
		t.Fatalf("Get(%q) error = %v, want %v", key, value.Err, errKeyNotFound)
	}
	if !strings.Contains(value.Err.Error(), key) {
		t.Fatalf("Get(%q) error %q does not contain key", key, value.Err)
	}
}
