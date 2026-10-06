// Package server implements the HTTP static file server, host routing, config injection, and SPA fallback.
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/eliotstocker/webapp-operator/internal/cache"
	"github.com/eliotstocker/webapp-operator/internal/injection"
)

// RouteTarget holds the active serving parameters for a website.
type RouteTarget struct {
	Namespace           string
	WebsiteName         string
	RevisionName        string
	Image               string
	Env                 map[string]string
	InjectionMode       string
	ConfigPath          string
	VersionPath         string
	DisablePolling      bool
	PollIntervalSeconds int32
}

// EffectivePollInterval returns the effective polling interval in seconds.
// It defaults to 30 seconds unless explicitly disabled.
func (t *RouteTarget) EffectivePollInterval() int32 {
	if t.DisablePolling {
		return 0
	}
	if t.PollIntervalSeconds > 0 {
		return t.PollIntervalSeconds
	}
	return 30
}

// EffectiveVersionPath returns the configured version endpoint path, defaulting to "/_version".
func (t *RouteTarget) EffectiveVersionPath() string {
	return injection.CleanVersionPath(t.VersionPath)
}

// Router maintains the mapping from Host header to RouteTarget.
type Router struct {
	mu           sync.RWMutex
	hosts        map[string]RouteTarget
	cacheManager cache.Manager
}

// NewRouter creates a new HTTP router backed by the local cache.
func NewRouter(cm cache.Manager) *Router {
	return &Router{
		hosts:        make(map[string]RouteTarget),
		cacheManager: cm,
	}
}


// SetWebsiteRoutes registers or updates all hostnames for a specific website.
func (rt *Router) SetWebsiteRoutes(hostnames []string, target RouteTarget) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	for _, host := range hostnames {
		rt.hosts[host] = target
	}
}

// RemoveWebsiteRoutes unregisters all hostnames for a specific website.
func (rt *Router) RemoveWebsiteRoutes(hostnames []string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	for _, host := range hostnames {
		delete(rt.hosts, host)
	}
}

// ServeHTTP handles static file serving, config injection, and SPA fallback.
func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if colonIdx := strings.IndexByte(host, ':'); colonIdx != -1 {
		host = host[:colonIdx]
	}

	rt.mu.RLock()
	target, exists := rt.hosts[host]
	rt.mu.RUnlock()

	if !exists {
		http.Error(w, fmt.Sprintf("no website configured for host %q", host), http.StatusNotFound)
		return
	}

	configPath := injection.CleanPath(target.ConfigPath)
	versionPath := target.EffectiveVersionPath()
	pollInterval := target.EffectivePollInterval()

	// 1. Dynamic Config Endpoint (/_config.js)
	if (target.InjectionMode == "endpoint" || target.InjectionMode == "both" || target.InjectionMode == "") && r.URL.Path == configPath {
		js, err := injection.GenerateConfigJS(target.Env, target.RevisionName, pollInterval, versionPath)
		if err != nil {
			http.Error(w, "failed to generate runtime config", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(js)
		return
	}

	// 2. Active Version Endpoint (/_version or custom path)
	if r.URL.Path == versionPath {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"version": target.RevisionName,
			"image":   target.Image,
		})
		return
	}

	// 3. Resolve on-disk directory
	dir := rt.cacheManager.RevisionDir(target.Namespace, target.WebsiteName, target.RevisionName)
	if !rt.cacheManager.IsCached(target.Namespace, target.WebsiteName, target.RevisionName) {
		http.Error(w, "revision assets not yet warmed on this replica", http.StatusServiceUnavailable)
		return
	}

	relPath := strings.TrimPrefix(filepath.Clean(r.URL.Path), "/")
	filePath := filepath.Join(dir, relPath)

	// Check if requested path points to an actual file on disk
	stat, err := os.Stat(filePath)
	if err == nil && !stat.IsDir() {
		// Immutable assets caching
		if strings.HasPrefix(relPath, "assets/") || isImmutableAsset(relPath) {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		}
		http.ServeFile(w, r, filePath)
		return
	}

	// 4. SPA Fallback: Serve index.html
	indexPath := filepath.Join(dir, "index.html")
	indexBytes, err := os.ReadFile(indexPath)
	if err != nil {
		http.Error(w, "index.html not found", http.StatusNotFound)
		return
	}

	// Inject inline script if enabled
	if target.InjectionMode == "inline" || target.InjectionMode == "both" {
		injected, err := injection.InjectInlineScript(indexBytes, target.Env, target.RevisionName, pollInterval, versionPath)
		if err == nil {
			indexBytes = injected
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(indexBytes)
}

func isImmutableAsset(path string) bool {
	ext := filepath.Ext(path)
	switch ext {
	case ".js", ".css", ".png", ".jpg", ".jpeg", ".svg", ".webp", ".woff", ".woff2", ".ttf":
		return true
	default:
		return false
	}
}
