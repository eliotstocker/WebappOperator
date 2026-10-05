package oci

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestExtractTar(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "oci-test-extract-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	files := []struct {
		Name string
		Body string
		Type byte
	}{
		{"index.html", "<h1>Hello OCI</h1>", tar.TypeReg},
		{"assets/", "", tar.TypeDir},
		{"assets/app.js", "console.log('hi');", tar.TypeReg},
		{"../evil.txt", "exploit", tar.TypeReg}, // Should be skipped by zip-slip check
	}

	for _, file := range files {
		hdr := &tar.Header{
			Name:     file.Name,
			Mode:     0644,
			Size:     int64(len(file.Body)),
			Typeflag: file.Type,
		}
		if file.Type == tar.TypeDir {
			hdr.Mode = 0755
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("failed to write tar header: %v", err)
		}
		if file.Type == tar.TypeReg {
			if _, err := tw.Write([]byte(file.Body)); err != nil {
				t.Fatalf("failed to write tar body: %v", err)
			}
		}
	}
	_ = tw.Close()

	if _, err := ExtractTar(&buf, tempDir); err != nil {
		t.Fatalf("unexpected extract error: %v", err)
	}

	indexContent, err := os.ReadFile(filepath.Join(tempDir, "index.html"))
	if err != nil || string(indexContent) != "<h1>Hello OCI</h1>" {
		t.Fatalf("index.html content mismatch: %v, %s", err, string(indexContent))
	}

	jsContent, err := os.ReadFile(filepath.Join(tempDir, "assets", "app.js"))
	if err != nil || string(jsContent) != "console.log('hi');" {
		t.Fatalf("assets/app.js content mismatch: %v, %s", err, string(jsContent))
	}

	if _, err := os.Stat(filepath.Join(tempDir, "..", "evil.txt")); err == nil {
		t.Fatalf("evil.txt should not have been extracted outside destDir!")
	}
}

func TestBuildKeychain(t *testing.T) {
	dockerCfgJSON := []byte(`{
		"auths": {
			"ghcr.io": {
				"auth": "dXNlcjpwYXNz"
			},
			"myregistry.azurecr.io": {
				"username": "azureuser",
				"password": "azurepassword"
			}
		}
	}`)

	secretJSON := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "pull-secret", Namespace: "default"},
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: dockerCfgJSON,
		},
	}

	kc := BuildKeychain([]corev1.Secret{secretJSON})

	// Resolve ghcr.io
	auth, err := kc.Resolve(&mockResource{registry: "ghcr.io"})
	if err != nil {
		t.Fatalf("failed to resolve auth: %v", err)
	}
	cfg, err := auth.Authorization()
	if err != nil {
		t.Fatalf("failed to get authorization: %v", err)
	}
	if cfg.Username != "user" || cfg.Password != "pass" {
		t.Errorf("expected user:pass, got %s:%s", cfg.Username, cfg.Password)
	}

	// Resolve azure registry
	auth, err = kc.Resolve(&mockResource{registry: "myregistry.azurecr.io"})
	if err != nil {
		t.Fatalf("failed to resolve azure auth: %v", err)
	}
	cfg, err = auth.Authorization()
	if err != nil {
		t.Fatalf("failed to get authorization: %v", err)
	}
	if cfg.Username != "azureuser" || cfg.Password != "azurepassword" {
		t.Errorf("expected azureuser:azurepassword, got %s:%s", cfg.Username, cfg.Password)
	}
}

func TestPullAndExtractInvalidRef(t *testing.T) {
	puller := NewPuller()
	_, err := puller.PullAndExtract(context.Background(), "INVALID:::REFERENCE", t.TempDir(), nil)
	if err == nil {
		t.Errorf("expected error on invalid image ref")
	}
}

type mockResource struct {
	registry string
}

func (m *mockResource) String() string      { return m.registry }
func (m *mockResource) RegistryStr() string { return m.registry }
