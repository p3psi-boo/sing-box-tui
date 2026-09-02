package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

var byteUnits = []string{"B", "KB", "MB", "GB", "TB", "PB"}

func formatUnits(value float64, base float64) string {
	if value < 0 || value != value {
		value = 0
	}
	unitIndex := 0
	for value >= base && unitIndex < len(byteUnits)-1 {
		value /= base
		unitIndex++
	}
	if unitIndex == 0 {
		return fmt.Sprintf("%d %s", int(value), byteUnits[unitIndex])
	}
	return fmt.Sprintf("%.1f %s", value, byteUnits[unitIndex])
}

func FormatBytes(value uint64) string {
	return formatUnits(float64(value), 1000)
}

func FormatMemory(value uint64) string {
	return formatUnits(float64(value), 1024)
}

func FormatBitrate(bps int64) string {
	switch {
	case bps >= 1_000_000_000:
		return fmt.Sprintf("%.1f Gbps", float64(bps)/1_000_000_000)
	case bps >= 1_000_000:
		return fmt.Sprintf("%.1f Mbps", float64(bps)/1_000_000)
	case bps >= 1_000:
		return fmt.Sprintf("%.1f kbps", float64(bps)/1_000)
	default:
		return fmt.Sprintf("%d bps", bps)
	}
}

func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	seconds := d.Seconds()
	if seconds < 60 {
		if seconds < 10 {
			return fmt.Sprintf("%.1fs", seconds)
		}
		return fmt.Sprintf("%.0fs", seconds)
	}
	minutes := int(seconds) / 60
	secs := int(seconds) % 60
	if minutes < 60 {
		return fmt.Sprintf("%dm %ds", minutes, secs)
	}
	hours := minutes / 60
	minutes %= 60
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

func FormatUptime(startedAt time.Time) string {
	return FormatDuration(time.Since(startedAt))
}

func FormatUptimeShort(startedAt time.Time) string {
	return FormatDurationShort(time.Since(startedAt))
}

func FormatDurationShort(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	seconds := int(d.Seconds())
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	minutes := seconds / 60
	secs := seconds % 60
	if minutes < 60 {
		if secs == 0 {
			return fmt.Sprintf("%dm", minutes)
		}
		return fmt.Sprintf("%dm%ds", minutes, secs)
	}
	hours := minutes / 60
	minutes %= 60
	if hours < 48 {
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
	days := hours / 24
	hours %= 24
	return fmt.Sprintf("%dd%dh", days, hours)
}

func FormatBitrateShort(bps int64) string {
	if bps < 0 {
		bps = 0
	}
	switch {
	case bps >= 1_000_000_000:
		return shortRate(float64(bps)/1_000_000_000, "Gb")
	case bps >= 1_000_000:
		return shortRate(float64(bps)/1_000_000, "Mb")
	case bps >= 1_000:
		return shortRate(float64(bps)/1_000, "kb")
	default:
		return fmt.Sprintf("%db", bps)
	}
}

func shortRate(v float64, suffix string) string {
	if v >= 10 {
		return fmt.Sprintf("%.0f%s", v, suffix)
	}
	return fmt.Sprintf("%.1f%s", v, suffix)
}

func Sparkline(values []int64, width int) string {
	if width <= 0 || len(values) == 0 {
		return ""
	}
	if len(values) > width {
		values = values[len(values)-width:]
	}
	blocks := []rune("▁▂▃▄▅▆▇█")
	max := int64(0)
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	var b strings.Builder
	for _, v := range values {
		if v < 0 {
			v = 0
		}
		idx := 0
		if max > 0 {
			idx = int(int64(len(blocks)-1) * v / max)
			if idx >= len(blocks) {
				idx = len(blocks) - 1
			}
		}
		b.WriteRune(blocks[idx])
	}
	return b.String()
}

func Truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= width {
		return s
	}
	return ansi.Truncate(s, width, "…")
}

func Divider(width int) string {
	if width <= 0 {
		return ""
	}
	return DimStyle.Render(strings.Repeat("─", width))
}

func FitRow(left, right string, width int) string {
	if width <= 0 {
		return ""
	}
	rw := ansi.StringWidth(right)
	if rw >= width {
		return Truncate(right, width)
	}
	left = Truncate(left, width-rw-1)
	gap := width - ansi.StringWidth(left) - rw
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func FormatDelay(ms int32) string {
	if ms <= 0 {
		return "-"
	}
	return fmt.Sprintf("%dms", ms)
}

func ProxyTypeLabel(t string) string {
	labels := map[string]string{
		"direct": "Direct", "block": "Block", "dns": "DNS",
		"socks": "SOCKS", "http": "HTTP", "shadowsocks": "Shadowsocks",
		"vmess": "VMess", "trojan": "Trojan", "wireguard": "WireGuard",
		"hysteria": "Hysteria", "vless": "VLESS", "tuic": "TUIC",
		"hysteria2": "Hysteria2", "selector": "Selector", "urltest": "URLTest",
	}
	if label, ok := labels[t]; ok {
		return label
	}
	return t
}

func ServiceStatusLabel(status int32) string {
	switch status {
	case 1:
		return "Starting"
	case 2:
		return "Started"
	case 3:
		return "Stopping"
	case 4:
		return "Fatal"
	default:
		return "Idle"
	}
}

func LogLevelLabel(level int32) string {
	switch level {
	case 0:
		return "PANIC"
	case 1:
		return "FATAL"
	case 2:
		return "ERROR"
	case 3:
		return "WARN"
	case 4:
		return "INFO"
	case 5:
		return "DEBUG"
	case 6:
		return "TRACE"
	default:
		return "?"
	}
}
