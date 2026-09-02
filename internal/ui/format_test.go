package ui_test

import (
	"testing"

	"github.com/p3psi-boo/sing-box-tui/internal/ui"
)

func TestFormatBytes(t *testing.T) {
	if got := ui.FormatBytes(1500); got != "1.5 KB" {
		t.Fatalf("got %q", got)
	}
	if got := ui.FormatBytes(500); got != "500 B" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatDelay(t *testing.T) {
	if got := ui.FormatDelay(0); got != "-" {
		t.Fatalf("got %q", got)
	}
	if got := ui.FormatDelay(120); got != "120ms" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatBitrateShort(t *testing.T) {
	if got := ui.FormatBitrateShort(1_200_000); got != "1.2Mb" {
		t.Fatalf("got %q", got)
	}
	if got := ui.FormatBitrateShort(800); got != "800b" {
		t.Fatalf("got %q", got)
	}
}

func TestSparkline(t *testing.T) {
	got := ui.Sparkline([]int64{0, 1, 2, 4}, 4)
	if len([]rune(got)) != 4 {
		t.Fatalf("got %q", got)
	}
}

func TestFitRow(t *testing.T) {
	got := ui.FitRow("left", "right", 12)
	if got != "left   right" {
		t.Fatalf("got %q", got)
	}
	got = ui.Truncate("hello world", 8)
	if got != "hello w…" {
		t.Fatalf("truncate %q", got)
	}
}

func TestCleanConnectError(t *testing.T) {
	raw := `rpc error: code = Unavailable desc = "error reading server preface: http2: frame too large"`
	if got := ui.CleanConnectError(raw); got != "not a sing-box API (wrong protocol or port)" {
		t.Fatalf("got %q", got)
	}
}
