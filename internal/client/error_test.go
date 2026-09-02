package client

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestClassifyError(t *testing.T) {
	err := status.Error(codes.Unavailable, `connection error: desc = "error reading server preface: http2: frame too large"`)
	got := ClassifyError(err)
	if got == nil || got.Error() != "not a sing-box API (wrong protocol or port)" {
		t.Fatalf("got %v", got)
	}
	err = status.Error(codes.Unauthenticated, "nope")
	got = ClassifyError(err)
	if got == nil || got.Error() != "invalid secret" {
		t.Fatalf("auth got %v", got)
	}
}
