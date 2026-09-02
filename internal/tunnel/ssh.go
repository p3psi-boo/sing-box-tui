package tunnel

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/p3psi-boo/sing-box-tui/internal/config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type Tunnel struct {
	localAddr string
	listener  net.Listener
	client    *ssh.Client
	wg        sync.WaitGroup
	closed    chan struct{}
}

func Start(server *config.Server) (*Tunnel, error) {
	if err := server.Validate(); err != nil {
		return nil, err
	}
	if server.SSH.Port == 0 {
		server.SSH.Port = 22
	}

	keyPath, err := config.ExpandPath(server.SSH.IdentityFile)
	if err != nil {
		return nil, err
	}
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read identity file: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("parse identity file (check permissions): %w", err)
	}

	sshConfig := &ssh.ClientConfig{
		User: server.SSH.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	if server.SSH.KnownHostsFile != "" {
		knownPath, err := config.ExpandPath(server.SSH.KnownHostsFile)
		if err != nil {
			return nil, err
		}
		callback, err := knownhosts.New(knownPath)
		if err != nil {
			return nil, fmt.Errorf("known_hosts: %w", err)
		}
		sshConfig.HostKeyCallback = callback
	}

	addr := fmt.Sprintf("%s:%d", server.SSH.Host, server.SSH.Port)
	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("ssh dial: %w", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		client.Close()
		return nil, err
	}

	remote := server.RemoteAddress
	if remote == "" {
		remote = "127.0.0.1:9090"
	}

	t := &Tunnel{
		localAddr: listener.Addr().String(),
		listener:  listener,
		client:    client,
		closed:    make(chan struct{}),
	}

	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-t.closed:
					return
				default:
				}
				continue
			}
			go t.forward(conn, remote)
		}
	}()

	return t, nil
}

func (t *Tunnel) LocalAddr() string {
	return t.localAddr
}

func (t *Tunnel) Close() error {
	select {
	case <-t.closed:
		return nil
	default:
		close(t.closed)
	}
	_ = t.listener.Close()
	_ = t.client.Close()
	t.wg.Wait()
	return nil
}

func (t *Tunnel) forward(local net.Conn, remote string) {
	defer local.Close()
	remoteConn, err := t.client.Dial("tcp", remote)
	if err != nil {
		return
	}
	defer remoteConn.Close()
	go func() { _, _ = io.Copy(remoteConn, local) }()
	_, _ = io.Copy(local, remoteConn)
}

func DefaultIdentityPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	for _, name := range []string{"id_ed25519", "id_rsa"} {
		path := filepath.Join(home, ".ssh", name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}
