package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type SSHConfig struct {
	Host           string `yaml:"host"`
	Port           int    `yaml:"port"`
	User           string `yaml:"user"`
	IdentityFile   string `yaml:"identity_file"`
	KnownHostsFile string `yaml:"known_hosts_file"`
}

type Server struct {
	ID            string    `yaml:"id"`
	Name          string    `yaml:"name"`
	Type          string    `yaml:"type"` // direct or ssh
	API           string    `yaml:"api"`  // auto, clash, or grpc
	Address       string    `yaml:"address"`
	Secret        string    `yaml:"secret"`
	TLS           bool      `yaml:"tls"`
	SSH           SSHConfig `yaml:"ssh"`
	RemoteAddress string    `yaml:"remote_address"`
}

type Config struct {
	Active  string   `yaml:"active"`
	Servers []Server `yaml:"servers"`
}

func DefaultPath() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "sing-box-tui", "config.yaml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "sing-box-tui", "config.yaml"), nil
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func Save(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (c *Config) ActiveServer() *Server {
	for i := range c.Servers {
		if c.Servers[i].ID == c.Active {
			return &c.Servers[i]
		}
	}
	if len(c.Servers) > 0 {
		return &c.Servers[0]
	}
	return nil
}

func (c *Config) ServerByID(id string) *Server {
	for i := range c.Servers {
		if c.Servers[i].ID == id {
			return &c.Servers[i]
		}
	}
	return nil
}

func (s *Server) DisplayName() string {
	if s.Name != "" {
		return s.Name
	}
	if s.Type == "ssh" {
		return s.SSH.Host
	}
	return s.Address
}

func (s *Server) Validate() error {
	switch s.API {
	case "", "auto", "clash", "grpc":
	default:
		return fmt.Errorf("server %q: unknown api %q (want auto, clash, or grpc)", s.ID, s.API)
	}
	switch s.Type {
	case "direct", "":
		if s.Address == "" {
			return fmt.Errorf("server %q: address is required for direct connection", s.ID)
		}
	case "ssh":
		if s.SSH.Host == "" {
			return fmt.Errorf("server %q: ssh.host is required", s.ID)
		}
		if s.SSH.User == "" {
			return fmt.Errorf("server %q: ssh.user is required", s.ID)
		}
		if s.RemoteAddress == "" {
			s.RemoteAddress = "127.0.0.1:9090"
		}
	default:
		return fmt.Errorf("server %q: unknown type %q", s.ID, s.Type)
	}
	return nil
}

func ExpandPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if len(path) == 1 || path[1] == '/' {
			return home + path[1:], nil
		}
	}
	return path, nil
}
