package lmsdk

import (
	"errors"
	"testing"
)

func TestErrorExports(t *testing.T) {
	if ErrSessionNotFound == nil || ErrUnsupportedControl == nil || ErrClientClosed == nil {
		t.Fatal("expected exported sentinel errors")
	}

	var sdkErr SDKError = &MessageParseError{}
	if !errors.Is(ErrUnsupportedControl, ErrUnsupportedControl) {
		t.Fatal("expected sentinel equality")
	}
	if !sdkErr.IsSDKError() {
		t.Fatal("expected sdk error marker")
	}
}
