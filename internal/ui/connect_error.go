package ui

import (
	"strings"

	"github.com/p3psi-boo/sing-box-tui/internal/config"
)

func ServerTarget(server *config.Server) string {
	if server == nil {
		return ""
	}
	if server.Type == "ssh" {
		if server.SSH.Host != "" {
			return server.SSH.Host
		}
		return server.DisplayName()
	}
	if server.Address != "" {
		return server.Address
	}
	return server.DisplayName()
}

func CleanConnectError(err string) string {
	err = strings.TrimSpace(err)
	if err == "" {
		return ""
	}
	lower := strings.ToLower(err)
	switch {
	case strings.Contains(lower, "invalid secret"), strings.Contains(lower, "unauthenticated"):
		return "invalid secret"
	case strings.Contains(lower, "permission denied"):
		return "permission denied"
	case strings.Contains(lower, "frame too large"):
		return "not a sing-box API (wrong protocol or port)"
	case strings.Contains(lower, "connection refused"):
		return "connection refused"
	case strings.Contains(lower, "deadline exceeded"), strings.Contains(lower, "timed out"), strings.Contains(lower, "timeout"):
		return "timed out"
	case strings.Contains(lower, "not a sing-box"):
		if strings.Contains(lower, "wrong protocol") {
			return "not a sing-box API (wrong protocol or port)"
		}
		return "not a sing-box API"
	}
	if i := strings.Index(err, "rpc error:"); i >= 0 {
		if desc := after(err, `desc = "`); desc != "" {
			return desc
		}
		if desc := after(err, "desc = "); desc != "" {
			return desc
		}
	}
	return err
}

func after(s, sep string) string {
	i := strings.Index(s, sep)
	if i < 0 {
		return ""
	}
	out := s[i+len(sep):]
	out = strings.TrimSuffix(out, `"`)
	return strings.TrimSpace(out)
}
