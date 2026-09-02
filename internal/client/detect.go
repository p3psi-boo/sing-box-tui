package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/p3psi-boo/sing-box-tui/internal/config"
)

const (
	apiGRPC  = "grpc"
	apiClash = "clash"
)

type clashVersion struct {
	Version string `json:"version"`
	Hello   string `json:"hello"`
	Premium bool   `json:"premium"`
	Meta    bool   `json:"meta"`
}

func httpBaseURL(address string, useTLS bool) string {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		host = address
		port = "9090"
	}
	scheme := "http"
	if useTLS {
		scheme = "https"
	}
	return scheme + "://" + net.JoinHostPort(host, port)
}

func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func clashGET(ctx context.Context, client *http.Client, base, path, secret string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	return client.Do(req)
}

func pingClash(ctx context.Context, base, secret string) (string, error) {
	client := newHTTPClient(3 * time.Second)
	resp, err := clashGET(ctx, client, base, "/version", secret)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return "", ErrInvalidSecret
	case http.StatusOK:
		var v clashVersion
		if err := json.Unmarshal(body, &v); err != nil {
			return "", fmt.Errorf("not clash api")
		}
		if v.Version == "" && !v.Meta && !v.Premium {
			return "", fmt.Errorf("not clash api")
		}
		return strings.TrimSpace(strings.TrimPrefix(v.Version, "sing-box ")), nil
	default:
		return "", fmt.Errorf("not clash api")
	}
}

func resolveAPIKind(ctx context.Context, server *config.Server, target string) (string, string, error) {
	pref := server.API
	if pref == "" {
		pref = "auto"
	}
	base := httpBaseURL(target, server.TLS)
	switch pref {
	case apiGRPC:
		return apiGRPC, "", nil
	case apiClash:
		ver, err := pingClash(ctx, base, server.Secret)
		if err != nil {
			return "", "", clashDialError(err)
		}
		return apiClash, ver, nil
	default:
		ver, err := pingClash(ctx, base, server.Secret)
		if err == nil {
			return apiClash, ver, nil
		}
		if err == ErrInvalidSecret {
			return "", "", err
		}
		if clashDialError(err).Error() == "connection refused" {
			return "", "", clashDialError(err)
		}
		return apiGRPC, "", nil
	}
}

func clashDialError(err error) error {
	if err == ErrInvalidSecret {
		return err
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "connection refused"):
		return fmt.Errorf("connection refused")
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "deadline exceeded"):
		return fmt.Errorf("timed out")
	}
	return err
}
