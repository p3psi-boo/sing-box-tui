package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/p3psi-boo/sing-box-tui/gen/daemon"
)

const clashTestURL = "https://www.gstatic.com/generate_204"

type clashProxy struct {
	Type    string           `json:"type"`
	Name    string           `json:"name"`
	Now     string           `json:"now"`
	All     []string         `json:"all"`
	History []clashDelayHist `json:"history"`
}

type clashDelayHist struct {
	Delay uint16 `json:"delay"`
}

type clashProxiesResponse struct {
	Proxies map[string]clashProxy `json:"proxies"`
}

type clashConfigs struct {
	Mode     string   `json:"mode"`
	ModeList []string `json:"mode-list"`
	LogLevel string   `json:"log-level"`
}

type clashTraffic struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

type clashLog struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

type clashConnSnapshot struct {
	DownloadTotal int64       `json:"downloadTotal"`
	UploadTotal   int64       `json:"uploadTotal"`
	Memory        uint64      `json:"memory"`
	Connections   []clashConn `json:"connections"`
}

type clashConn struct {
	ID       string    `json:"id"`
	Upload   int64     `json:"upload"`
	Download int64     `json:"download"`
	Start    time.Time `json:"start"`
	Chains   []string  `json:"chains"`
	Rule     string    `json:"rule"`
	Metadata struct {
		Network         string `json:"network"`
		Type            string `json:"type"`
		SourceIP        string `json:"sourceIP"`
		DestinationIP   string `json:"destinationIP"`
		SourcePort      string `json:"sourcePort"`
		DestinationPort string `json:"destinationPort"`
		Host            string `json:"host"`
		ProcessPath     string `json:"processPath"`
	} `json:"metadata"`
}

func groupsFromProxies(proxies map[string]clashProxy) []*daemon.Group {
	if len(proxies) == 0 {
		return nil
	}
	names := make([]string, 0, len(proxies))
	for name := range proxies {
		if name != "GLOBAL" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if _, ok := proxies["GLOBAL"]; ok {
		names = append(names, "GLOBAL")
	}
	groups := make([]*daemon.Group, 0)
	for _, name := range names {
		p := proxies[name]
		if len(p.All) == 0 {
			continue
		}
		g := &daemon.Group{
			Tag:        name,
			Type:       clashTypeKey(p.Type),
			Selectable: p.Type == "Selector",
			Selected:   p.Now,
		}
		for _, tag := range p.All {
			item := &daemon.GroupItem{Tag: tag}
			if member, ok := proxies[tag]; ok {
				item.Type = clashTypeKey(member.Type)
				item.UrlTestDelay = lastDelay(member.History)
			}
			g.Items = append(g.Items, item)
		}
		groups = append(groups, g)
	}
	return groups
}

func clashTypeKey(t string) string {
	switch t {
	case "Selector":
		return "selector"
	case "URLTest":
		return "urltest"
	case "Fallback":
		return "fallback"
	case "LoadBalance":
		return "loadbalance"
	case "Reject":
		return "block"
	default:
		return t
	}
}

func lastDelay(hist []clashDelayHist) int32 {
	if len(hist) == 0 {
		return 0
	}
	return int32(hist[len(hist)-1].Delay)
}

func clashLogLevel(s string) daemon.LogLevel {
	switch strings.ToLower(s) {
	case "panic":
		return daemon.LogLevel_PANIC
	case "fatal":
		return daemon.LogLevel_FATAL
	case "error":
		return daemon.LogLevel_ERROR
	case "warning", "warn":
		return daemon.LogLevel_WARN
	case "info":
		return daemon.LogLevel_INFO
	case "debug":
		return daemon.LogLevel_DEBUG
	case "trace":
		return daemon.LogLevel_TRACE
	default:
		return daemon.LogLevel_INFO
	}
}

func joinHostPort(host, port string) string {
	if host == "" {
		return ""
	}
	if port == "" {
		return host
	}
	return net.JoinHostPort(host, port)
}

func connectionFromClash(c clashConn) *daemon.Connection {
	outbound := ""
	if len(c.Chains) > 0 {
		outbound = c.Chains[0]
	}
	conn := &daemon.Connection{
		Id:            c.ID,
		Network:       c.Metadata.Network,
		Source:        joinHostPort(c.Metadata.SourceIP, c.Metadata.SourcePort),
		Destination:   joinHostPort(c.Metadata.DestinationIP, c.Metadata.DestinationPort),
		Domain:        c.Metadata.Host,
		Outbound:      outbound,
		Rule:          c.Rule,
		ChainList:     c.Chains,
		UplinkTotal:   c.Upload,
		DownlinkTotal: c.Download,
		CreatedAt:     c.Start.UnixMilli(),
	}
	if t := c.Metadata.Type; t != "" {
		if j := strings.IndexByte(t, '/'); j >= 0 {
			conn.InboundType = t[:j]
			conn.Inbound = t[j+1:]
		} else {
			conn.InboundType = t
		}
	}
	if c.Metadata.ProcessPath != "" {
		conn.ProcessInfo = &daemon.ProcessInfo{ProcessPath: c.Metadata.ProcessPath}
	}
	return conn
}

func mergeClashConnections(prev map[string]*ConnectionRow, snap clashConnSnapshot, now int64) map[string]*ConnectionRow {
	next := dupConnMap(prev)
	seen := make(map[string]bool, len(snap.Connections))
	for _, c := range snap.Connections {
		if c.ID == "" {
			continue
		}
		seen[c.ID] = true
		row, ok := next[c.ID]
		if !ok {
			next[c.ID] = &ConnectionRow{Connection: connectionFromClash(c)}
			continue
		}
		row = cloneConnectionRow(row)
		if row.Connection != nil {
			row.UplinkRate = c.Upload - row.Connection.UplinkTotal
			row.DownlinkRate = c.Download - row.Connection.DownlinkTotal
			if row.UplinkRate < 0 {
				row.UplinkRate = 0
			}
			if row.DownlinkRate < 0 {
				row.DownlinkRate = 0
			}
		}
		row.Connection = connectionFromClash(c)
		row.ClosedAt = 0
		next[c.ID] = row
	}
	for id, row := range next {
		if !seen[id] && row != nil && row.ClosedAt == 0 {
			row = cloneConnectionRow(row)
			row.ClosedAt = now
			row.UplinkRate = 0
			row.DownlinkRate = 0
			next[id] = row
		}
	}
	pruneClosedLocked(next)
	return next
}

func (s *Session) connectClash(ctx context.Context) error {
	s.mu.Lock()
	s.baseURL = httpBaseURL(s.target, s.server.TLS)
	s.http = newHTTPClient(0)
	s.kind = apiClash
	s.mu.Unlock()

	var ver clashVersion
	if err := s.clashDo(ctx, http.MethodGet, "/version", nil, &ver); err != nil {
		return clashDialError(err)
	}
	var cfg clashConfigs
	_ = s.clashDo(ctx, http.MethodGet, "/configs", nil, &cfg)

	s.mu.Lock()
	s.snapshot = newSnapshot()
	s.snapshot.Connected = true
	s.snapshot.API = apiClash
	s.snapshot.Version = strings.TrimPrefix(ver.Version, "sing-box ")
	s.snapshot.ServiceStatus = daemon.ServiceStatus_STARTED
	s.snapshot.ClashMode = cfg.Mode
	s.snapshot.ClashModeList = slices.Clone(cfg.ModeList)
	if len(s.snapshot.ClashModeList) == 0 && cfg.Mode != "" {
		s.snapshot.ClashModeList = []string{"Rule", "Global", "Direct"}
	}
	if cfg.LogLevel != "" {
		s.snapshot.DefaultLogLevel = clashLogLevel(cfg.LogLevel)
		s.snapshot.DefaultLogLevelSet = true
	}
	s.snapshot.Status = &daemon.Status{}
	s.mu.Unlock()
	s.publish()
	s.spawn(s.runClash)
	return nil
}

func (s *Session) runClash(ctx context.Context) {
	var wg sync.WaitGroup
	start := func(name string, fn func(context.Context) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.runWithRetry(ctx, name, fn)
		}()
	}
	start("clashTraffic", s.clashStreamTraffic)
	start("clashLogs", s.clashStreamLogs)
	start("clashConnections", s.clashPollConnections)
	start("clashProxies", s.clashPollProxies)
	wg.Wait()
	s.mu.Lock()
	s.snapshot.Connected = false
	s.mu.Unlock()
	s.publish()
}

func (s *Session) clashClient() (*http.Client, string, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	secret := ""
	if s.server != nil {
		secret = s.server.Secret
	}
	return s.http, s.baseURL, secret
}

func (s *Session) clashDo(ctx context.Context, method, path string, body any, out any) error {
	httpClient, baseURL, secret := s.clashClient()
	if httpClient == nil {
		return fmt.Errorf("not connected")
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrInvalidSecret
	}
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if resp.StatusCode >= 400 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if len(bytes.TrimSpace(msg)) == 0 {
			return fmt.Errorf("clash api %s", resp.Status)
		}
		return fmt.Errorf("clash api %s: %s", resp.Status, string(bytes.TrimSpace(msg)))
	}
	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (s *Session) clashStream(ctx context.Context, path string, handle func([]byte) error) error {
	httpClient, baseURL, secret := s.clashClient()
	if httpClient == nil {
		return fmt.Errorf("not connected")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrInvalidSecret
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("clash api %s", resp.Status)
	}
	dec := json.NewDecoder(resp.Body)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return err
		}
		if err := handle(raw); err != nil {
			return err
		}
	}
}

func (s *Session) clashStreamTraffic(ctx context.Context) error {
	return s.clashStream(ctx, "/traffic", func(raw []byte) error {
		var t clashTraffic
		if err := json.Unmarshal(raw, &t); err != nil {
			return err
		}
		s.mu.Lock()
		s.snapshot.Status = patchStatus(s.snapshot.Status, func(st *daemon.Status) {
			st.TrafficAvailable = true
			st.Uplink = t.Up
			st.Downlink = t.Down
		})
		s.snapshot.UplinkHist = appendHist(s.snapshot.UplinkHist, t.Up)
		s.snapshot.DownlinkHist = appendHist(s.snapshot.DownlinkHist, t.Down)
		s.mu.Unlock()
		s.publish()
		return nil
	})
}

func (s *Session) clashStreamLogs(ctx context.Context) error {
	return s.clashStream(ctx, "/logs?level=debug", func(raw []byte) error {
		var line clashLog
		if err := json.Unmarshal(raw, &line); err != nil {
			return err
		}
		s.mu.Lock()
		s.logID++
		s.snapshot.Logs = appendLogEntries(s.snapshot.Logs, LogEntry{
			ID:      s.logID,
			Level:   clashLogLevel(line.Type),
			Message: line.Payload,
		})
		s.mu.Unlock()
		s.publish()
		return nil
	})
}

func (s *Session) clashPollConnections(ctx context.Context) error {
	if err := s.clashRefreshConnections(ctx); err != nil {
		return err
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
			if err := s.clashRefreshConnections(ctx); err != nil {
				return err
			}
		}
	}
}

func (s *Session) clashRefreshConnections(ctx context.Context) error {
	var snap clashConnSnapshot
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.clashDo(cctx, http.MethodGet, "/connections", nil, &snap); err != nil {
		return err
	}
	s.mu.Lock()
	s.snapshot.Connections = mergeClashConnections(s.snapshot.Connections, snap, time.Now().UnixMilli())
	s.snapshot.ConnectionsLoaded = true
	n := 0
	for _, row := range s.snapshot.Connections {
		if row != nil && row.ClosedAt == 0 {
			n++
		}
	}
	s.snapshot.Status = patchStatus(s.snapshot.Status, func(st *daemon.Status) {
		st.Memory = snap.Memory
		st.UplinkTotal = snap.UploadTotal
		st.DownlinkTotal = snap.DownloadTotal
		st.ConnectionsOut = int32(n)
	})
	s.mu.Unlock()
	s.publish()
	return nil
}

func (s *Session) clashPollProxies(ctx context.Context) error {
	if err := s.clashRefreshProxies(ctx); err != nil {
		return err
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
			if err := s.clashRefreshProxies(ctx); err != nil {
				return err
			}
		}
	}
}

func (s *Session) clashRefreshProxies(ctx context.Context) error {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var resp clashProxiesResponse
	if err := s.clashDo(cctx, http.MethodGet, "/proxies", nil, &resp); err != nil {
		return err
	}
	var cfg clashConfigs
	_ = s.clashDo(cctx, http.MethodGet, "/configs", nil, &cfg)
	groups := groupsFromProxies(resp.Proxies)
	s.mu.Lock()
	s.snapshot.Groups = groups
	s.snapshot.GroupsLoaded = true
	if cfg.Mode != "" {
		s.snapshot.ClashMode = cfg.Mode
	}
	if len(cfg.ModeList) > 0 {
		s.snapshot.ClashModeList = slices.Clone(cfg.ModeList)
	}
	s.mu.Unlock()
	s.publish()
	return nil
}

func (s *Session) clashSelect(groupTag, outboundTag string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	path := "/proxies/" + url.PathEscape(groupTag)
	return s.clashDo(ctx, http.MethodPut, path, map[string]string{"name": outboundTag}, nil)
}

func (s *Session) clashURLTest(groupTag string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s.mu.RLock()
	var members []string
	for _, g := range s.snapshot.Groups {
		if g.Tag == groupTag {
			for _, item := range g.Items {
				members = append(members, item.Tag)
			}
			break
		}
	}
	s.mu.RUnlock()
	if len(members) == 0 {
		members = []string{groupTag}
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for _, tag := range members {
		tag := tag
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			q := url.Values{}
			q.Set("url", clashTestURL)
			q.Set("timeout", "5000")
			path := "/proxies/" + url.PathEscape(tag) + "/delay?" + q.Encode()
			tctx, tcancel := context.WithTimeout(ctx, 8*time.Second)
			defer tcancel()
			_ = s.clashDo(tctx, http.MethodGet, path, nil, nil)
		}()
	}
	wg.Wait()
	return s.clashRefreshProxies(ctx)
}

func (s *Session) clashSetMode(mode string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.clashDo(ctx, http.MethodPatch, "/configs", map[string]string{"mode": mode}, nil)
}

func (s *Session) clashCloseConnection(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.clashDo(ctx, http.MethodDelete, "/connections/"+url.PathEscape(id), nil, nil)
}

func (s *Session) clashCloseAll() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.clashDo(ctx, http.MethodDelete, "/connections", nil, nil)
}
