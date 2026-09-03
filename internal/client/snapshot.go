package client

import (
	"slices"

	"github.com/p3psi-boo/sing-box-tui/gen/daemon"
	"google.golang.org/protobuf/proto"
)

func newSnapshot() Snapshot {
	return Snapshot{
		Connections: make(map[string]*ConnectionRow),
	}
}

func cloneSnapshot(s Snapshot) Snapshot {
	out := s
	out.Status = cloneStatus(s.Status)
	out.ClashModeList = slices.Clone(s.ClashModeList)
	out.Groups = cloneGroups(s.Groups)
	out.Connections = cloneConnections(s.Connections)
	out.Logs = slices.Clone(s.Logs)
	out.UplinkHist = slices.Clone(s.UplinkHist)
	out.DownlinkHist = slices.Clone(s.DownlinkHist)
	return out
}

func cloneStatus(st *daemon.Status) *daemon.Status {
	if st == nil {
		return nil
	}
	return proto.Clone(st).(*daemon.Status)
}

func cloneConnection(c *daemon.Connection) *daemon.Connection {
	if c == nil {
		return nil
	}
	return proto.Clone(c).(*daemon.Connection)
}

func cloneGroups(groups []*daemon.Group) []*daemon.Group {
	if groups == nil {
		return nil
	}
	out := make([]*daemon.Group, len(groups))
	for i, g := range groups {
		if g != nil {
			out[i] = proto.Clone(g).(*daemon.Group)
		}
	}
	return out
}

func cloneConnectionRow(row *ConnectionRow) *ConnectionRow {
	if row == nil {
		return nil
	}
	cp := *row
	cp.Connection = cloneConnection(row.Connection)
	return &cp
}

func cloneConnections(src map[string]*ConnectionRow) map[string]*ConnectionRow {
	if src == nil {
		return nil
	}
	dst := make(map[string]*ConnectionRow, len(src))
	for id, row := range src {
		dst[id] = cloneConnectionRow(row)
	}
	return dst
}

func dupConnMap(src map[string]*ConnectionRow) map[string]*ConnectionRow {
	if src == nil {
		return make(map[string]*ConnectionRow)
	}
	dst := make(map[string]*ConnectionRow, len(src))
	for id, row := range src {
		dst[id] = row
	}
	return dst
}

func patchStatus(st *daemon.Status, patch func(*daemon.Status)) *daemon.Status {
	out := cloneStatus(st)
	if out == nil {
		out = &daemon.Status{}
	}
	patch(out)
	return out
}

func appendLogEntries(logs []LogEntry, extra ...LogEntry) []LogEntry {
	if len(extra) == 0 {
		return slices.Clone(logs)
	}
	out := make([]LogEntry, len(logs)+len(extra))
	copy(out, logs)
	copy(out[len(logs):], extra)
	if len(out) > maxLogEntries {
		out = slices.Clone(out[len(out)-maxLogEntries:])
	}
	return out
}
