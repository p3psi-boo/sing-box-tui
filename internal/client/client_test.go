package client

import (
	"fmt"
	"sync"
	"testing"

	"github.com/p3psi-boo/sing-box-tui/gen/daemon"
)

func TestSnapshotCopiesConnections(t *testing.T) {
	s := &Session{snapshot: newSnapshot()}
	s.snapshot.Connections["a"] = &ConnectionRow{
		Connection: &daemon.Connection{Id: "a", Domain: "example.com"},
		UplinkRate: 1,
	}
	s.snapshot.Logs = []LogEntry{{ID: 1, Message: "hello"}}
	s.snapshot.Status = &daemon.Status{Uplink: 10}
	s.snapshot.ClashModeList = []string{"Rule"}
	s.snapshot.Groups = []*daemon.Group{{
		Tag:      "proxy",
		Selected: "jp-1",
		Items:    []*daemon.GroupItem{{Tag: "jp-1", UrlTestDelay: 42}},
	}}

	snap := s.Snapshot()
	s.snapshot.Connections["b"] = &ConnectionRow{Connection: &daemon.Connection{Id: "b"}}
	s.snapshot.Connections["a"].UplinkRate = 99
	s.snapshot.Connections["a"].Connection.Domain = "mutated.example"
	s.snapshot.Logs[0].Message = "mutated"
	s.snapshot.Status.Uplink = 99
	s.snapshot.ClashModeList[0] = "Global"
	s.snapshot.Groups[0].Selected = "us-1"
	s.snapshot.Groups[0].Items[0].UrlTestDelay = 1

	if _, ok := snap.Connections["b"]; ok {
		t.Fatal("published connections map still shared")
	}
	if snap.Connections["a"].UplinkRate != 1 {
		t.Fatalf("published row mutated: %d", snap.Connections["a"].UplinkRate)
	}
	if snap.Connections["a"].Connection.Domain != "example.com" {
		t.Fatalf("published connection proto mutated: %q", snap.Connections["a"].Connection.Domain)
	}
	if snap.Logs[0].Message != "hello" {
		t.Fatalf("published logs mutated: %q", snap.Logs[0].Message)
	}
	if snap.Status.Uplink != 10 {
		t.Fatalf("published status mutated: %d", snap.Status.Uplink)
	}
	if snap.ClashModeList[0] != "Rule" {
		t.Fatalf("published mode list mutated: %q", snap.ClashModeList[0])
	}
	if snap.Groups[0].Selected != "jp-1" || snap.Groups[0].Items[0].UrlTestDelay != 42 {
		t.Fatalf("published group mutated: %+v", snap.Groups[0])
	}
}

func TestSnapshotConnectionsIsolatedFromLiveWrites(t *testing.T) {
	s := &Session{snapshot: newSnapshot()}
	s.snapshot.Connections["a"] = &ConnectionRow{
		Connection: &daemon.Connection{Id: "a"},
		UplinkRate: 1,
	}
	snap := s.Snapshot()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 8000; i++ {
			s.mu.Lock()
			s.snapshot.Connections[fmt.Sprintf("x%d", i)] = &ConnectionRow{
				Connection: &daemon.Connection{Id: fmt.Sprintf("x%d", i)},
			}
			if i%80 == 0 {
				delete(s.snapshot.Connections, "a")
				s.snapshot.Connections["a"] = &ConnectionRow{
					Connection: &daemon.Connection{Id: "a"},
					UplinkRate: int64(i),
				}
			}
			pruneClosedLocked(s.snapshot.Connections)
			s.mu.Unlock()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 8000; i++ {
			n := 0
			for _, row := range snap.Connections {
				if row != nil && row.Connection != nil {
					n++
					_ = row.UplinkRate
				}
			}
			if n == 0 {
				t.Error("published snapshot became empty")
				return
			}
		}
	}()
	wg.Wait()

	if row := snap.Connections["a"]; row == nil || row.UplinkRate != 1 {
		t.Fatalf("published row = %+v", row)
	}
}

func TestSnapshotLogsIsolatedFromLiveWrites(t *testing.T) {
	s := &Session{snapshot: newSnapshot()}
	s.snapshot.Logs = []LogEntry{{ID: 1, Message: "hello"}}
	snap := s.Snapshot()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 4000; i++ {
			s.mu.Lock()
			s.snapshot.Logs = appendLogEntries(s.snapshot.Logs, LogEntry{ID: i + 2, Message: fmt.Sprintf("l%d", i)})
			s.mu.Unlock()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 4000; i++ {
			if len(snap.Logs) != 1 || snap.Logs[0].Message != "hello" {
				t.Errorf("published logs mutated: %+v", snap.Logs)
				return
			}
		}
	}()
	wg.Wait()
}
