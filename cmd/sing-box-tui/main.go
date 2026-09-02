package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/p3psi-boo/sing-box-tui/internal/app"
	"github.com/p3psi-boo/sing-box-tui/internal/client"
	"github.com/p3psi-boo/sing-box-tui/internal/config"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	path := *configPath
	if path == "" {
		var err error
		path, err = config.DefaultPath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "config path: %v\n", err)
			os.Exit(1)
		}
	}

	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	if len(flag.Args()) > 0 && flag.Args()[0] == "connect" {
		runConnect(cfg, flag.Args()[1:])
		return
	}

	m := app.NewModel(path, cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func runConnect(cfg *config.Config, args []string) {
	var server *config.Server
	if len(args) > 0 {
		server = cfg.ServerByID(args[0])
		if server == nil {
			fmt.Fprintf(os.Stderr, "server %q not found\n", args[0])
			os.Exit(1)
		}
	} else {
		server = cfg.ActiveServer()
	}
	if server == nil {
		fmt.Fprintf(os.Stderr, "no server configured\n")
		os.Exit(1)
	}

	mgr := client.NewManager()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := mgr.Connect(ctx, server); err != nil {
		fmt.Fprintf(os.Stderr, "connect failed: %v\n", err)
		os.Exit(1)
	}
	defer mgr.Close()

	s := mgr.Session()
	snap := s.Snapshot()
	fmt.Printf("connected to %s\n", server.DisplayName())
	fmt.Printf("version: %s (api %d)\n", snap.Version, snap.APIVersion)
}
