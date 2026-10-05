// Package oci pulls OCI artifacts and unpacks layers into destination directories.
package oci

import (
	"archive/tar"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	corev1 "k8s.io/api/core/v1"
)

// PullResult contains metadata about the fetched OCI artifact.
type PullResult struct {
	Digest       string
	UnpackedSize int64
}

// Puller handles pulling and extracting static assets from OCI artifacts.
type Puller interface {
	PullAndExtract(ctx context.Context, imageRef string, destDir string, secrets []corev1.Secret) (*PullResult, error)
}

type defaultPuller struct{}

// NewPuller creates a default OCI puller.
func NewPuller() Puller {
	return &defaultPuller{}
}

// PullAndExtract pulls the specified OCI artifact and unpacks its layers into destDir.
func (p *defaultPuller) PullAndExtract(ctx context.Context, imageRef string, destDir string, secrets []corev1.Secret) (*PullResult, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return nil, fmt.Errorf("invalid image reference %q: %w", imageRef, err)
	}

	keychain := BuildKeychain(secrets)
	remoteOpts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(keychain),
		remote.WithTransport(http.DefaultTransport),
	}

	img, err := remote.Image(ref, remoteOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch remote image %q: %w", imageRef, err)
	}

	digest, err := img.Digest()
	if err != nil {
		return nil, fmt.Errorf("failed to get image digest: %w", err)
	}

	layers, err := img.Layers()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve image layers: %w", err)
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create destination dir: %w", err)
	}

	var totalSize int64
	for i, layer := range layers {
		layerReader, err := layer.Uncompressed()
		if err != nil {
			return nil, fmt.Errorf("failed to read uncompressed layer %d: %w", i, err)
		}

		size, err := ExtractTar(layerReader, destDir)
		_ = layerReader.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to extract layer %d: %w", i, err)
		}
		totalSize += size
	}

	return &PullResult{
		Digest:       digest.String(),
		UnpackedSize: totalSize,
	}, nil
}

// ExtractTar extracts an uncompressed tar stream into destDir, returning total bytes written.
func ExtractTar(r io.Reader, destDir string) (int64, error) {
	tr := tar.NewReader(r)
	var totalBytes int64

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return totalBytes, fmt.Errorf("error reading tar archive: %w", err)
		}

		// Prevent zip-slip vulnerability
		cleanName := filepath.Clean(header.Name)
		if strings.HasPrefix(cleanName, "..") || filepath.IsAbs(cleanName) {
			continue
		}

		target := filepath.Join(destDir, cleanName)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return totalBytes, err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return totalBytes, err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, header.FileInfo().Mode())
			if err != nil {
				return totalBytes, err
			}
			n, err := io.Copy(f, tr)
			f.Close()
			if err != nil {
				return totalBytes, err
			}
			totalBytes += n
		}
	}

	return totalBytes, nil
}

// secretKeychain implements authn.Keychain backed by Kubernetes imagePullSecrets
type secretKeychain struct {
	authConfigs map[string]authn.AuthConfig
}

// BuildKeychain creates an authn.Keychain from Kubernetes Secrets containing Docker configs.
func BuildKeychain(secrets []corev1.Secret) authn.Keychain {
	auths := make(map[string]authn.AuthConfig)

	for _, secret := range secrets {
		if data, ok := secret.Data[corev1.DockerConfigJsonKey]; ok {
			parseDockerConfigJSON(data, auths)
		}
	}

	return &secretKeychain{authConfigs: auths}
}

func (k *secretKeychain) Resolve(target authn.Resource) (authn.Authenticator, error) {
	server := target.RegistryStr()
	if cfg, ok := k.authConfigs[server]; ok {
		return authn.FromConfig(cfg), nil
	}
	return authn.DefaultKeychain.Resolve(target)
}

type dockerConfigJSON struct {
	Auths map[string]dockerAuthEntry `json:"auths"`
}

type dockerAuthEntry struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Auth     string `json:"auth,omitempty"`
}

func parseDockerConfigJSON(data []byte, out map[string]authn.AuthConfig) {
	var cfg dockerConfigJSON
	if err := json.Unmarshal(data, &cfg); err != nil {
		return
	}
	for reg, entry := range cfg.Auths {
		authCfg := authn.AuthConfig{
			Username: entry.Username,
			Password: entry.Password,
			Auth:     entry.Auth,
		}
		if authCfg.Username == "" && entry.Auth != "" {
			decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
			if err == nil {
				parts := strings.SplitN(string(decoded), ":", 2)
				if len(parts) == 2 {
					authCfg.Username = parts[0]
					authCfg.Password = parts[1]
				}
			}
		}
		out[reg] = authCfg
	}
}
