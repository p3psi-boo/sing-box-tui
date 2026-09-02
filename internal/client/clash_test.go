package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/p3psi-boo/sing-box-tui/gen/daemon"
	"github.com/p3psi-boo/sing-box-tui/internal/config"
)

func TestGroupsFromProxies(t *testing.T) {
	proxies := map[string]clashProxy{
		"jp-1": {Type: "VMess", Name: "jp-1", History: []clashDelayHist{{Delay: 80}}},
		"us-1": {Type: "Trojan", Name: "us-1"},
		"proxy": {
			Type: "Selector",
			Name: "proxy",
			Now:  "jp-1",
			All:  []string{"jp-1", "us-1"},
		},
		"GLOBAL": {
			Type: "Fallback",
			Name: "GLOBAL",
			Now:  "proxy",
			All:  []string{"proxy", "jp-1"},
		},
	}
	groups := groupsFromProxies(proxies)
	if len(groups) != 2 {
		t.Fatalf("groups = %d", len(groups))
	}
	if groups[0].Tag != "proxy" || !groups[0].Selectable || groups[0].Selected != "jp-1" {
		t.Fatalf("proxy group: %+v", groups[0])
	}
	if groups[0].Items[0].UrlTestDelay != 80 {
		t.Fatalf("delay = %d", groups[0].Items[0].UrlTestDelay)
	}
	if groups[1].Tag != "GLOBAL" || groups[1].Selectable {
		t.Fatalf("GLOBAL: %+v", groups[1])
	}
}

func TestMergeClashConnections(t *testing.T) {
	first := clashConnSnapshot{
		Connections: []clashConn{{
			ID:       "a",
			Upload:   10,
			Download: 100,
			Chains:   []string{"jp-1", "proxy"},
			Metadata: struct {
				Network         string `json:"network"`
				Type            string `json:"type"`
				SourceIP        string `json:"sourceIP"`
				DestinationIP   string `json:"destinationIP"`
				SourcePort      string `json:"sourcePort"`
				DestinationPort string `json:"destinationPort"`
				Host            string `json:"host"`
				ProcessPath     string `json:"processPath"`
			}{Network: "tcp", Host: "example.com", DestinationIP: "1.2.3.4", DestinationPort: "443"},
		}},
	}
	rows := mergeClashConnections(nil, first, 1)
	if rows["a"].Connection.Domain != "example.com" || rows["a"].Connection.Outbound != "jp-1" {
		t.Fatalf("row = %+v", rows["a"].Connection)
	}
	second := clashConnSnapshot{
		Connections: []clashConn{{
			ID:       "a",
			Upload:   20,
			Download: 400,
			Chains:   []string{"jp-1", "proxy"},
		}},
	}
	rows = mergeClashConnections(rows, second, 2)
	if rows["a"].UplinkRate != 10 || rows["a"].DownlinkRate != 300 {
		t.Fatalf("rates up=%d down=%d", rows["a"].UplinkRate, rows["a"].DownlinkRate)
	}
	rows = mergeClashConnections(rows, clashConnSnapshot{}, 3)
	if rows["a"].ClosedAt != 3 {
		t.Fatalf("closed at %d", rows["a"].ClosedAt)
	}
}

func TestClashLogLevel(t *testing.T) {
	if clashLogLevel("warning") != daemon.LogLevel_WARN {
		t.Fatal("warn")
	}
	if clashLogLevel("ERROR") != daemon.LogLevel_ERROR {
		t.Fatal("error")
	}
}

func TestPingClash(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/version" {
			t.Errorf("path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer s3cret" {
			t.Errorf("auth %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "sing-box 1.12.0", "meta": true, "premium": true})
	}))
	defer ts.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ver, err := pingClash(ctx, ts.URL, "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if ver != "1.12.0" {
		t.Fatalf("version %q", ver)
	}
}

func TestPingClashUnauthorized(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Unauthorized"}`))
	}))
	defer ts.Close()
	_, err := pingClash(context.Background(), ts.URL, "nope")
	if err != ErrInvalidSecret {
		t.Fatalf("err = %v", err)
	}
}

func TestClashSessionGroups(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "sing-box 1.11.0", "meta": true})
	})
	mux.HandleFunc("/configs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"mode": "Rule", "mode-list": []string{"Rule", "Global", "Direct"}})
	})
	mux.HandleFunc("/proxies", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"proxies": map[string]any{
				"jp-1": map[string]any{"type": "VMess", "name": "jp-1", "history": []map[string]any{{"delay": 42}}},
				"proxy": map[string]any{
					"type": "Selector", "name": "proxy", "now": "jp-1", "all": []string{"jp-1"},
				},
			},
		})
	})
	mux.HandleFunc("/connections", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"connections": []any{}, "memory": 1024})
	})
	mux.HandleFunc("/traffic", func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, `{"up":100,"down":200}`+"\n")
		if flusher != nil {
			flusher.Flush()
		}
		<-r.Context().Done()
	})
	mux.HandleFunc("/logs", func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, `{"type":"info","payload":"hello"}`+"\n")
		if flusher != nil {
			flusher.Flush()
		}
		<-r.Context().Done()
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	addr := strings.TrimPrefix(ts.URL, "http://")
	s := NewSession(&config.Server{Type: "direct", Address: addr, API: "clash"})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap := s.Snapshot()
		if snap.Connected && snap.API == "clash" && snap.GroupsLoaded && len(snap.Groups) == 1 && snap.ClashMode == "Rule" {
			if snap.Groups[0].Tag != "proxy" || snap.Groups[0].Items[0].UrlTestDelay != 42 {
				t.Fatalf("group %+v", snap.Groups[0])
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("snapshot %#v", s.Snapshot())
}

func TestClashSelectAndMode(t *testing.T) {
	var selected, mode string
	mux := http.NewServeMux()
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "sing-box 1.10.0", "meta": true})
	})
	mux.HandleFunc("/configs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			var body struct {
				Mode string `json:"mode"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mode = body.Mode
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"mode": "Rule", "mode-list": []string{"Rule", "Global"}})
	})
	mux.HandleFunc("/proxies/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			var body struct {
				Name string `json:"name"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			selected = body.Name
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/proxies", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"proxies": map[string]any{}})
	})
	mux.HandleFunc("/connections", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"connections": []any{}})
	})
	mux.HandleFunc("/traffic", func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	mux.HandleFunc("/logs", func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	ts := httptest.NewServer(mux)
	defer ts.Close()
	addr := strings.TrimPrefix(ts.URL, "http://")
	s := NewSession(&config.Server{Type: "direct", Address: addr})
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.kind != apiClash {
		t.Fatalf("kind %s", s.kind)
	}
	if err := s.SelectOutbound("proxy", "jp-1"); err != nil {
		t.Fatal(err)
	}
	if selected != "jp-1" {
		t.Fatalf("selected %q", selected)
	}
	if err := s.SetClashMode("Global"); err != nil {
		t.Fatal(err)
	}
	if mode != "Global" {
		t.Fatalf("mode %q", mode)
	}
}

func TestResolveAPIKindAutoClash(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "sing-box 1.10.0", "meta": true})
	}))
	defer ts.Close()
	addr := strings.TrimPrefix(ts.URL, "http://")
	kind, ver, err := resolveAPIKind(context.Background(), &config.Server{Type: "direct", Address: addr}, addr)
	if err != nil {
		t.Fatal(err)
	}
	if kind != apiClash || ver != "1.10.0" {
		t.Fatalf("kind=%s ver=%s", kind, ver)
	}
}
