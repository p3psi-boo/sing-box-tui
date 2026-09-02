package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/p3psi-boo/sing-box-tui/gen/daemon"
	"github.com/p3psi-boo/sing-box-tui/internal/config"
	"github.com/p3psi-boo/sing-box-tui/internal/tunnel"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

var ErrInvalidSecret = errors.New("invalid secret")

const (
	maxLogEntries        = 3000
	maxClosedConnections = 1000
	statusInterval       = 1000
	trafficHistory       = 32
)

type LogEntry struct {
	ID      int
	Level   daemon.LogLevel
	Message string
}

type ConnectionRow struct {
	Connection   *daemon.Connection
	UplinkRate   int64
	DownlinkRate int64
	ClosedAt     int64
}

type Snapshot struct {
	Connected          bool
	ConnectError       string
	Version            string
	API                string
	APIVersion         int32
	ServiceStatus      daemon.ServiceStatus_Type
	ServiceError       string
	StartedAt          time.Time
	Status             *daemon.Status
	ClashModeList      []string
	ClashMode          string
	Groups             []*daemon.Group
	GroupsLoaded       bool
	Connections        map[string]*ConnectionRow
	ConnectionsLoaded  bool
	ConnectionsError   string
	ConnectionsBlocked bool
	Logs               []LogEntry
	DefaultLogLevel    daemon.LogLevel
	DefaultLogLevelSet bool
	UplinkHist         []int64
	DownlinkHist       []int64
}

func newSnapshot() Snapshot {
	return Snapshot{
		Connections: make(map[string]*ConnectionRow),
	}
}

type Update struct {
	Snapshot Snapshot
}

type Session struct {
	server   *config.Server
	target   string
	tunnel   *tunnel.Tunnel
	kind     string
	http     *http.Client
	baseURL  string
	conn     *grpc.ClientConn
	grpc     daemon.StartedServiceClient
	cancel   context.CancelFunc
	updates  chan Update
	mu       sync.RWMutex
	snapshot Snapshot
	logID    int
}

func NewSession(server *config.Server) *Session {
	return &Session{
		server:   server,
		updates:  make(chan Update, 64),
		snapshot: newSnapshot(),
	}
}

func (s *Session) Updates() <-chan Update {
	return s.updates
}

func (s *Session) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshot
}

func (s *Session) Connect(ctx context.Context) error {
	if err := s.Close(); err != nil {
		return err
	}
	if err := s.server.Validate(); err != nil {
		return err
	}

	target, tun, err := s.resolveTarget()
	if err != nil {
		return err
	}
	s.target = target
	s.tunnel = tun

	kind, version, err := resolveAPIKind(ctx, s.server, target)
	if err != nil {
		s.cleanupTransport()
		return err
	}
	s.kind = kind
	if kind == apiClash {
		if err := s.connectClash(ctx); err != nil {
			s.cleanupTransport()
			return err
		}
		if version != "" && s.snapshot.Version == "" {
			s.mu.Lock()
			s.snapshot.Version = version
			s.mu.Unlock()
			s.publish()
		}
		return nil
	}
	return s.connectGRPC(ctx)
}

func (s *Session) connectGRPC(ctx context.Context) error {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(s.unaryAuth),
		grpc.WithStreamInterceptor(s.streamAuth),
	}

	conn, err := grpc.NewClient(s.target, opts...)
	if err != nil {
		s.cleanupTransport()
		return fmt.Errorf("grpc dial: %w", err)
	}
	s.conn = conn
	s.grpc = daemon.NewStartedServiceClient(conn)
	s.kind = apiGRPC

	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	version, err := s.grpc.GetVersion(probeCtx, &emptypb.Empty{})
	if err != nil {
		_ = conn.Close()
		s.conn = nil
		s.grpc = nil
		s.cleanupTransport()
		return classifyError(err)
	}

	s.mu.Lock()
	s.snapshot = newSnapshot()
	s.snapshot.Connected = true
	s.snapshot.API = apiGRPC
	s.snapshot.Version = version.Version
	s.snapshot.APIVersion = version.ApiVersion
	s.mu.Unlock()
	s.publish()

	streamCtx, streamCancel := context.WithCancel(context.Background())
	s.cancel = streamCancel
	go s.runStreams(streamCtx)
	return nil
}

func (s *Session) resolveTarget() (string, *tunnel.Tunnel, error) {
	switch s.server.Type {
	case "direct", "":
		return s.server.Address, nil, nil
	case "ssh":
		tun, err := tunnel.Start(s.server)
		if err != nil {
			return "", nil, err
		}
		return tun.LocalAddr(), tun, nil
	default:
		return "", nil, fmt.Errorf("unknown server type %q", s.server.Type)
	}
}

func (s *Session) Close() error {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.conn != nil {
		_ = s.conn.Close()
		s.conn = nil
	}
	s.grpc = nil
	s.http = nil
	s.baseURL = ""
	s.kind = ""
	s.cleanupTransport()
	s.mu.Lock()
	s.snapshot.Connected = false
	s.mu.Unlock()
	s.publish()
	return nil
}

func (s *Session) cleanupTransport() {
	if s.tunnel != nil {
		_ = s.tunnel.Close()
		s.tunnel = nil
	}
}

func (s *Session) Reconnect(ctx context.Context) error {
	server := s.server
	if err := s.Close(); err != nil {
		return err
	}
	s.server = server
	return s.Connect(ctx)
}

func appendHist(hist []int64, v int64) []int64 {
	next := make([]int64, 0, min(len(hist)+1, trafficHistory))
	if len(hist) >= trafficHistory {
		next = append(next, hist[len(hist)-trafficHistory+1:]...)
	} else {
		next = append(next, hist...)
	}
	return append(next, v)
}

func pruneClosedLocked(conns map[string]*ConnectionRow) {
	type item struct {
		id string
		at int64
	}
	closed := make([]item, 0, len(conns))
	for id, row := range conns {
		if row != nil && row.ClosedAt > 0 {
			closed = append(closed, item{id, row.ClosedAt})
		}
	}
	extra := len(closed) - maxClosedConnections
	if extra <= 0 {
		return
	}
	sort.Slice(closed, func(i, j int) bool { return closed[i].at < closed[j].at })
	for i := 0; i < extra; i++ {
		delete(conns, closed[i].id)
	}
}

func (s *Session) publish() {
	s.mu.RLock()
	snap := s.snapshot
	s.mu.RUnlock()
	select {
	case s.updates <- Update{Snapshot: snap}:
	default:
	}
}

func (s *Session) unaryAuth(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	return invoker(s.authContext(ctx), method, req, reply, cc, opts...)
}

func (s *Session) streamAuth(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return streamer(s.authContext(ctx), desc, cc, method, opts...)
}

func (s *Session) authContext(ctx context.Context) context.Context {
	if s.server.Secret != "" {
		return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+s.server.Secret)
	}
	return ctx
}

func ClassifyError(err error) error {
	return classifyError(err)
}

func classifyError(err error) error {
	if err == nil {
		return nil
	}
	if st, ok := status.FromError(err); ok {
		msg := st.Message()
		switch st.Code() {
		case codes.Unauthenticated:
			return ErrInvalidSecret
		case codes.PermissionDenied:
			return fmt.Errorf("permission denied")
		case codes.Unimplemented:
			return fmt.Errorf("not a sing-box API")
		case codes.DeadlineExceeded:
			return fmt.Errorf("timed out")
		case codes.Unavailable:
			return fmt.Errorf("%s", unavailableReason(msg))
		}
		if reason := unavailableReason(msg); reason != msg && reason != "unavailable" {
			return fmt.Errorf("%s", reason)
		}
	}
	if reason := unavailableReason(err.Error()); reason != err.Error() {
		return fmt.Errorf("%s", reason)
	}
	return err
}

func unavailableReason(msg string) string {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "frame too large"):
		return "not a sing-box API (wrong protocol or port)"
	case strings.Contains(lower, "connection refused"):
		return "connection refused"
	case strings.Contains(lower, "deadline exceeded"), strings.Contains(lower, "timeout"):
		return "timed out"
	case strings.Contains(lower, "unavailable") && !strings.Contains(lower, "rpc error"):
		return "unavailable"
	}
	if strings.Contains(lower, "rpc error") {
		return "unavailable"
	}
	return msg
}

func isTerminalError(err error) bool {
	if errors.Is(err, ErrInvalidSecret) {
		return true
	}
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.Unauthenticated, codes.PermissionDenied, codes.Unimplemented, codes.NotFound:
			return true
		}
	}
	return false
}

func (s *Session) runStreams(ctx context.Context) {
	var wg sync.WaitGroup
	start := func(name string, fn func(context.Context) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.runWithRetry(ctx, name, fn)
		}()
	}

	start("serviceStatus", s.streamServiceStatus)
	start("status", s.streamStatus)
	start("groups", s.streamGroups)
	start("clashMode", s.streamClashMode)
	start("connections", s.streamConnections)
	start("logs", s.streamLogs)
	start("startedAt", s.fetchStartedAt)

	wg.Wait()
	s.mu.Lock()
	s.snapshot.Connected = false
	s.mu.Unlock()
	s.publish()
}

func (s *Session) runWithRetry(ctx context.Context, name string, fn func(context.Context) error) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		err := fn(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil && isTerminalError(err) {
			s.mu.Lock()
			s.snapshot.ConnectError = classifyError(err).Error()
			s.mu.Unlock()
			s.publish()
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 5*time.Second {
			backoff += time.Second
		}
		_ = name
	}
}

func (s *Session) streamServiceStatus(ctx context.Context) error {
	stream, err := s.grpc.SubscribeServiceStatus(ctx, &emptypb.Empty{})
	if err != nil {
		return err
	}
	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.snapshot.ServiceStatus = msg.Status
		s.snapshot.ServiceError = msg.ErrorMessage
		s.mu.Unlock()
		s.publish()
	}
}

func (s *Session) streamStatus(ctx context.Context) error {
	stream, err := s.grpc.SubscribeStatus(ctx, &daemon.SubscribeStatusRequest{Interval: statusInterval})
	if err != nil {
		return err
	}
	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.snapshot.Status = msg
		s.snapshot.UplinkHist = appendHist(s.snapshot.UplinkHist, msg.Uplink)
		s.snapshot.DownlinkHist = appendHist(s.snapshot.DownlinkHist, msg.Downlink)
		s.mu.Unlock()
		s.publish()
	}
}

func (s *Session) streamGroups(ctx context.Context) error {
	stream, err := s.grpc.SubscribeGroups(ctx, &emptypb.Empty{})
	if err != nil {
		return err
	}
	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.snapshot.Groups = msg.Group
		s.snapshot.GroupsLoaded = true
		s.mu.Unlock()
		s.publish()
	}
}

func (s *Session) streamClashMode(ctx context.Context) error {
	status, err := s.grpc.GetClashModeStatus(ctx, &emptypb.Empty{})
	if err == nil {
		s.mu.Lock()
		s.snapshot.ClashModeList = status.ModeList
		s.snapshot.ClashMode = status.CurrentMode
		s.mu.Unlock()
		s.publish()
	}
	stream, err := s.grpc.SubscribeClashMode(ctx, &emptypb.Empty{})
	if err != nil {
		return err
	}
	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.snapshot.ClashMode = msg.Mode
		s.mu.Unlock()
		s.publish()
	}
}

func (s *Session) streamConnections(ctx context.Context) error {
	stream, err := s.grpc.SubscribeConnections(ctx, &daemon.SubscribeConnectionsRequest{Interval: statusInterval})
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.Unimplemented {
			s.mu.Lock()
			s.snapshot.ConnectionsBlocked = true
			s.snapshot.ConnectionsError = "Clash API is not enabled on this instance"
			s.snapshot.ConnectionsLoaded = true
			s.mu.Unlock()
			s.publish()
			return nil
		}
		return err
	}
	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}
		s.mu.Lock()
		if msg.GetReset_() {
			s.snapshot.Connections = make(map[string]*ConnectionRow)
		}
		for _, event := range msg.Events {
			switch event.Type {
			case daemon.ConnectionEventType_CONNECTION_EVENT_NEW:
				s.snapshot.Connections[event.Id] = &ConnectionRow{Connection: event.Connection}
			case daemon.ConnectionEventType_CONNECTION_EVENT_UPDATE:
				if row, ok := s.snapshot.Connections[event.Id]; ok {
					row.UplinkRate = event.UplinkDelta
					row.DownlinkRate = event.DownlinkDelta
					if event.Connection != nil {
						row.Connection = event.Connection
					}
				}
			case daemon.ConnectionEventType_CONNECTION_EVENT_CLOSED:
				if row, ok := s.snapshot.Connections[event.Id]; ok {
					row.ClosedAt = event.ClosedAt
					row.UplinkRate = 0
					row.DownlinkRate = 0
				}
			}
		}
		pruneClosedLocked(s.snapshot.Connections)
		s.snapshot.ConnectionsLoaded = true
		s.mu.Unlock()
		s.publish()
	}
}

func (s *Session) streamLogs(ctx context.Context) error {
	level, err := s.grpc.GetDefaultLogLevel(ctx, &emptypb.Empty{})
	if err == nil {
		s.mu.Lock()
		s.snapshot.DefaultLogLevel = level.Level
		s.snapshot.DefaultLogLevelSet = true
		s.mu.Unlock()
		s.publish()
	}
	stream, err := s.grpc.SubscribeLog(ctx, &emptypb.Empty{})
	if err != nil {
		return err
	}
	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}
		s.mu.Lock()
		if msg.GetReset_() {
			s.snapshot.Logs = nil
			s.logID = 0
		}
		for _, entry := range msg.Messages {
			s.logID++
			s.snapshot.Logs = append(s.snapshot.Logs, LogEntry{
				ID:      s.logID,
				Level:   entry.Level,
				Message: entry.Message,
			})
		}
		if len(s.snapshot.Logs) > maxLogEntries {
			s.snapshot.Logs = s.snapshot.Logs[len(s.snapshot.Logs)-maxLogEntries:]
		}
		s.mu.Unlock()
		s.publish()
	}
}

func (s *Session) fetchStartedAt(ctx context.Context) error {
	resp, err := s.grpc.GetStartedAt(ctx, &emptypb.Empty{})
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.snapshot.StartedAt = time.UnixMilli(resp.StartedAt)
	s.mu.Unlock()
	s.publish()
	<-ctx.Done()
	return ctx.Err()
}

// Actions

func (s *Session) SelectOutbound(groupTag, outboundTag string) error {
	if s.kind == apiClash {
		return s.clashSelect(groupTag, outboundTag)
	}
	if s.grpc == nil {
		return fmt.Errorf("not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.grpc.SelectOutbound(ctx, &daemon.SelectOutboundRequest{
		GroupTag:    groupTag,
		OutboundTag: outboundTag,
	})
	return err
}

func (s *Session) URLTest(groupTag string) error {
	if s.kind == apiClash {
		return s.clashURLTest(groupTag)
	}
	if s.grpc == nil {
		return fmt.Errorf("not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := s.grpc.URLTest(ctx, &daemon.URLTestRequest{OutboundTag: groupTag})
	return err
}

func (s *Session) SetGroupExpand(groupTag string, expand bool) error {
	if s.kind == apiClash {
		return nil
	}
	if s.grpc == nil {
		return fmt.Errorf("not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.grpc.SetGroupExpand(ctx, &daemon.SetGroupExpandRequest{
		GroupTag: groupTag,
		IsExpand: expand,
	})
	return err
}

func (s *Session) SetClashMode(mode string) error {
	if s.kind == apiClash {
		return s.clashSetMode(mode)
	}
	if s.grpc == nil {
		return fmt.Errorf("not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.grpc.SetClashMode(ctx, &daemon.ClashMode{Mode: mode})
	return err
}

func (s *Session) CloseConnection(id string) error {
	if s.kind == apiClash {
		return s.clashCloseConnection(id)
	}
	if s.grpc == nil {
		return fmt.Errorf("not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.grpc.CloseConnection(ctx, &daemon.CloseConnectionRequest{Id: id})
	return err
}

func (s *Session) CloseAllConnections() error {
	if s.kind == apiClash {
		return s.clashCloseAll()
	}
	if s.grpc == nil {
		return fmt.Errorf("not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.grpc.CloseAllConnections(ctx, &emptypb.Empty{})
	return err
}

func (s *Session) ClearLogs() error {
	if s.kind == apiClash {
		s.mu.Lock()
		s.snapshot.Logs = nil
		s.logID = 0
		s.mu.Unlock()
		s.publish()
		return nil
	}
	if s.grpc == nil {
		return fmt.Errorf("not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.grpc.ClearLogs(ctx, &emptypb.Empty{})
	return err
}

type Manager struct {
	session *Session
}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) Session() *Session {
	return m.session
}

func (m *Manager) Connect(ctx context.Context, server *config.Server) error {
	if m.session != nil {
		_ = m.session.Close()
	}
	m.session = NewSession(server)
	return m.session.Connect(ctx)
}

func (m *Manager) Close() error {
	if m.session == nil {
		return nil
	}
	err := m.session.Close()
	m.session = nil
	return err
}

func (m *Manager) Reconnect(ctx context.Context) error {
	if m.session == nil {
		return fmt.Errorf("no active session")
	}
	return m.session.Reconnect(ctx)
}
