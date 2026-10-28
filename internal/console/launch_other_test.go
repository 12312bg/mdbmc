//go:build !windows

package console

import (
	"strings"
	"testing"
	"time"

	"github.com/12312bg/mdbmc/internal/bmc"
)

// Off Windows the child exits with an error, which exercises the path that
// turns a failed window into an error for the caller instead of a false
// "opened".
func TestStartWindowReportsChildFailure(t *testing.T) {
	_, err := startWindow(Request{Title: "t", Spec: bmc.LaunchSpec{URL: "https://127.0.0.1/"}}, 10*time.Second)
	if err == nil || !strings.Contains(err.Error(), "only supported on Windows") {
		t.Fatalf("want the child's error, got %v", err)
	}
}
