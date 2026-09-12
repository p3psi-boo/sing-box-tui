package client

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/p3psi-boo/sing-box-tui/gen/daemon"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestParseListeningPorts(t *testing.T) {
	input := "  sl local_address rem_address st\n 0: 00000000:2382 00000000:0000 0A\n 1: 0100007F:C000 0100007F:2382 01\n 2: 00000000000000000000000000000000:FFFF 00:0000 0A\n"
	got, err := parseListeningPorts(strings.NewReader(input))
	if err != nil || !reflect.DeepEqual(got, []int{9090, 65535}) {
		t.Fatalf("ports=%v err=%v", got, err)
	}
}

func TestLocalDiscoveryRequiresAPIEvidence(t *testing.T) {
	cases := []struct {
		name                  string
		handler               http.HandlerFunc
		found, verified, auth bool
	}{
		{"clash", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/version" {
				_, _ = w.Write([]byte(`{"version":"sing-box 1.14.0"}`))
			} else {
				_, _ = w.Write([]byte(`{"hello":"clash"}`))
			}
		}, true, true, false},
		{"ordinary version endpoint", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"version":"1.0"}`)) }, false, false, false},
		{"json authentication candidate", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"message":"Unauthorized"}`))
		}, true, false, true},
		{"ordinary unauthorized", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(401)
			_, _ = w.Write([]byte("login required"))
		}, false, false, false},
		{"html", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("<html>hello</html>")) }, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(tc.handler)
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			got, ok := probeLocalAPI(ctx, strings.TrimPrefix(server.URL, "http://"))
			if ok != tc.found || got.Verified != tc.verified || got.NeedsSecret != tc.auth {
				t.Fatalf("found=%v result=%+v", ok, got)
			}
		})
	}
}

type discoveryGRPC struct {
	daemon.UnimplementedStartedServiceServer
	authenticated bool
}

func (s discoveryGRPC) GetVersion(context.Context, *emptypb.Empty) (*daemon.Version, error) {
	if s.authenticated {
		return nil, status.Error(codes.Unauthenticated, "secret required")
	}
	return &daemon.Version{Version: "1.14.0", ApiVersion: 1}, nil
}

func TestDiscoveryGRPCAndIPv6(t *testing.T) {
	for _, network := range []string{"tcp4", "tcp6"} {
		t.Run(network, func(t *testing.T) {
			host := "127.0.0.1:0"
			if network == "tcp6" {
				host = "[::1]:0"
			}
			listener, err := net.Listen(network, host)
			if err != nil {
				t.Skip(err)
			}
			server := grpc.NewServer()
			daemon.RegisterStartedServiceServer(server, discoveryGRPC{})
			go func() { _ = server.Serve(listener) }()
			defer server.Stop()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			got, ok := probeLocalAPI(ctx, listener.Addr().String())
			if !ok || !got.Verified || got.API != "grpc" {
				t.Fatalf("found=%v result=%+v", ok, got)
			}
		})
	}
}

func TestDiscoveryDoesNotFollowRedirects(t *testing.T) {
	calls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer redirect.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, ok := probeLocalAPI(ctx, strings.TrimPrefix(redirect.URL, "http://"))
	if ok || calls != 0 {
		t.Fatalf("followed redirect: calls=%d found=%v", calls, ok)
	}
}

func TestDiscoveryCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	_, err := discoverPorts(ctx, []int{9090, 9091, 65535})
	if err != context.Canceled || time.Since(start) > time.Second {
		t.Fatalf("cancellation err=%v duration=%v", err, time.Since(start))
	}
}
