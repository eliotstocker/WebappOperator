package syncer

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestTrackerAndClient(t *testing.T) {
	tracker := NewTracker()
	server := httptest.NewServer(tracker)
	defer server.Close()

	ns := "default"
	rev := "rev-123"
	pod1 := "pod-1"
	pod2 := "pod-2"

	// Initially empty
	if pods := tracker.GetReadyPods(ns, rev); len(pods) != 0 {
		t.Fatalf("expected 0 pods, got %d", len(pods))
	}

	// 1. Record pod1 directly on tracker
	tracker.RecordReady(ns, rev, pod1)
	pods := tracker.GetReadyPods(ns, rev)
	if len(pods) != 1 || pods[0] != pod1 {
		t.Fatalf("expected [pod-1], got %v", pods)
	}

	// 2. Pod2 reports via HTTP client to tracker HTTP server
	client := NewClient()
	leaderAddr := server.Listener.Addr().String()

	err := client.NotifyLeader(context.Background(), leaderAddr, SyncRequest{
		PodName:   pod2,
		Namespace: ns,
		Website:   "my-web",
		Revision:  rev,
	})
	if err != nil {
		t.Fatalf("NotifyLeader failed: %v", err)
	}

	// 3. Verify tracker now has both pods
	pods = tracker.GetReadyPods(ns, rev)
	if len(pods) != 2 {
		t.Fatalf("expected 2 pods, got %v", pods)
	}
}
