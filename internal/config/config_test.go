package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/p3psi-boo/sing-box-tui/internal/config"
)

func TestLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	original := &config.Config{
		Active: "home",
		Servers: []config.Server{
			{
				ID:      "home",
				Name:    "Home",
				Type:    "direct",
				Address: "192.168.1.1:9090",
				Secret:  "secret",
			},
			{
				ID:   "vps",
				Name: "VPS",
				Type: "ssh",
				SSH: config.SSHConfig{
					Host:         "example.com",
					Port:         22,
					User:         "root",
					IdentityFile: "~/.ssh/id_ed25519",
				},
				RemoteAddress: "127.0.0.1:9090",
			},
		},
	}
	if err := config.Save(path, original); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Active != "home" {
		t.Fatalf("active = %q", loaded.Active)
	}
	if len(loaded.Servers) != 2 {
		t.Fatalf("servers = %d", len(loaded.Servers))
	}
}

func TestServerValidate(t *testing.T) {
	tests := []struct {
		name    string
		server  config.Server
		wantErr bool
	}{
		{
			name:    "direct ok",
			server:  config.Server{ID: "a", Type: "direct", Address: "127.0.0.1:9090"},
			wantErr: false,
		},
		{
			name:    "clash api ok",
			server:  config.Server{ID: "a", Type: "direct", Address: "127.0.0.1:9090", API: "clash"},
			wantErr: false,
		},
		{
			name:    "unknown api",
			server:  config.Server{ID: "a", Type: "direct", Address: "127.0.0.1:9090", API: "v2ray"},
			wantErr: true,
		},
		{
			name:    "direct missing address",
			server:  config.Server{ID: "a", Type: "direct"},
			wantErr: true,
		},
		{
			name: "ssh ok",
			server: config.Server{
				ID:   "a",
				Type: "ssh",
				SSH:  config.SSHConfig{Host: "h", User: "u", IdentityFile: "/tmp/key"},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.server.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err := config.ExpandPath("~/.ssh/id_ed25519")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".ssh", "id_ed25519")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
