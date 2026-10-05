// Package syncer coordinates cache readiness between peer gateway pods and the elected leader.
package syncer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// SyncRequest is sent by peer pods to notify the leader of cached revisions.
type SyncRequest struct {
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	Website   string `json:"website"`
	Revision  string `json:"revision"`
}

// Tracker maintains in-memory state of which pods have reported readiness for revisions.
type Tracker struct {
	mu sync.RWMutex
	// key: "namespace/revision" -> set of pod names
	readyPods map[string]map[string]struct{}
}

// NewTracker creates a new readiness tracker for the elected leader.
func NewTracker() *Tracker {
	return &Tracker{
		readyPods: make(map[string]map[string]struct{}),
	}
}

// RecordReady marks a pod as ready for a specific revision.
func (t *Tracker) RecordReady(namespace, revision, podName string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := fmt.Sprintf("%s/%s", namespace, revision)
	if _, ok := t.readyPods[key]; !ok {
		t.readyPods[key] = make(map[string]struct{})
	}
	t.readyPods[key][podName] = struct{}{}
}

// GetReadyPods returns a slice of pod names that have cached the revision.
func (t *Tracker) GetReadyPods(namespace, revision string) []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	key := fmt.Sprintf("%s/%s", namespace, revision)
	podsSet, ok := t.readyPods[key]
	if !ok {
		return nil
	}

	pods := make([]string, 0, len(podsSet))
	for p := range podsSet {
		pods = append(pods, p)
	}
	return pods
}

// ServeHTTP handles incoming POST /internal/sync notifications.
func (t *Tracker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	t.RecordReady(req.Namespace, req.Revision, req.PodName)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// Client sends sync notifications to the elected leader.
type Client struct {
	httpClient *http.Client
}

// NewClient returns a syncer client.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// NotifyLeader sends a sync event to the leader pod.
func (c *Client) NotifyLeader(ctx context.Context, leaderAddr string, req SyncRequest) error {
	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("http://%s/internal/sync", leaderAddr)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to notify leader at %s: %w", leaderAddr, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("leader returned unexpected status: %d", resp.StatusCode)
	}
	return nil
}

