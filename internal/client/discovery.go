package client

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/p3psi-boo/sing-box-tui/gen/daemon"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// DiscoveredServer is an API endpoint, not a saved configuration. Authentication
// responses are only candidates: they do not prove the identity of the service.
type DiscoveredServer struct {
	Address, API, Version string
	NeedsSecret           bool
	Verified              bool
}

func DiscoverLocal(ctx context.Context) ([]DiscoveredServer, error) {
	ports, ok := localListeningPorts()
	if !ok {
		for p := 1; p <= 65535; p++ {
			ports = append(ports, p)
		}
	}
	return discoverPorts(ctx, ports)
}

// Linux exposes the listening port set without a connect scan. Other systems
// fall back to bounded concurrent TCP probes of the entire loopback port range.
func localListeningPorts() ([]int, bool) {
	seen := map[int]bool{}
	readable := false
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		ports, err := parseListeningPorts(file)
		_ = file.Close()
		if err != nil {
			continue
		}
		readable = true
		for _, p := range ports {
			seen[p] = true
		}
	}
	ports := make([]int, 0, len(seen))
	for p := range seen {
		ports = append(ports, p)
	}
	sort.Ints(ports)
	return ports, readable
}

func parseListeningPorts(r io.Reader) ([]int, error) {
	var ports []int
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 || fields[3] != "0A" {
			continue
		}
		_, hex, ok := strings.Cut(fields[1], ":")
		if !ok {
			continue
		}
		p, err := strconv.ParseUint(hex, 16, 16)
		if err == nil && p > 0 {
			ports = append(ports, int(p))
		}
	}
	return ports, scanner.Err()
}

func discoverPorts(ctx context.Context, ports []int) ([]DiscoveredServer, error) {
	jobs := make(chan string)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var found []DiscoveredServer
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for address := range jobs {
				if ctx.Err() != nil {
					return
				}
				dialer := net.Dialer{Timeout: 80 * time.Millisecond}
				conn, err := dialer.DialContext(ctx, "tcp", address)
				if err != nil {
					continue
				}
				_ = conn.Close()
				probeCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
				result, ok := probeLocalAPI(probeCtx, address)
				cancel()
				if ok {
					mu.Lock()
					found = append(found, result)
					mu.Unlock()
				}
			}
		}()
	}
enqueue:
	for _, port := range ports {
		for _, host := range []string{"127.0.0.1", "::1"} {
			select {
			case jobs <- net.JoinHostPort(host, strconv.Itoa(port)):
			case <-ctx.Done():
				break enqueue
			}
		}
	}
	close(jobs)
	wg.Wait()
	sort.Slice(found, func(i, j int) bool {
		if found[i].Verified != found[j].Verified {
			return found[i].Verified
		}
		return found[i].Address < found[j].Address
	})
	return found, ctx.Err()
}

func probeLocalAPI(ctx context.Context, address string) (DiscoveredServer, bool) {
	result := DiscoveredServer{Address: address}
	// No environment proxy or redirects: discovery must stay on loopback.
	transport := &http.Transport{DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	httpClient := newHTTPClient(500 * time.Millisecond)
	httpClient.Transport = transport
	resp, err := clashGET(ctx, httpClient, "http://"+address, "/version", "")
	if err == nil {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		var v clashVersion
		if resp.StatusCode == http.StatusOK && json.Unmarshal(body, &v) == nil && v.Version != "" {
			root, e := clashGET(ctx, httpClient, "http://"+address, "/", "")
			if e == nil {
				var hello clashVersion
				e = json.NewDecoder(io.LimitReader(root.Body, 4096)).Decode(&hello)
				_ = root.Body.Close()
				if e == nil && root.StatusCode == http.StatusOK && hello.Hello == "clash" {
					result.API = apiClash
					result.Version = v.Version
					result.Verified = true
					return result, true
				}
			}
		}
		if resp.StatusCode == http.StatusUnauthorized {
			var auth struct {
				Message string `json:"message"`
			}
			if json.Unmarshal(body, &auth) == nil && strings.EqualFold(auth.Message, "Unauthorized") {
				result.API = apiClash
				result.NeedsSecret = true
				return result, true
			}
		}
	}
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithNoProxy())
	if err != nil {
		return result, false
	}
	defer func() { _ = conn.Close() }()
	version, err := daemon.NewStartedServiceClient(conn).GetVersion(ctx, &emptypb.Empty{})
	if err == nil && version.Version != "" {
		result.API = apiGRPC
		result.Version = version.Version
		result.Verified = true
		return result, true
	}
	if status.Code(err) == codes.Unauthenticated {
		result.API = apiGRPC
		result.NeedsSecret = true
		return result, true
	}
	return result, false
}
