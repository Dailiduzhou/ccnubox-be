package tool

import (
	"errors"
	"strings"
)

const CCNUAccountInitializationRequiredMarker = "CCNU_ACCOUNT_INITIALIZATION_REQUIRED"

var ErrCCNUAccountInitializationRequired = errors.New(CCNUAccountInitializationRequiredMarker)

func IsCCNUAccountInitializationRequired(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrCCNUAccountInitializationRequired) ||
		strings.Contains(err.Error(), CCNUAccountInitializationRequiredMarker)
}
