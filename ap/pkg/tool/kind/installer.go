// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kind

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/fileutils"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/tool/fetch"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/tool/github"
	"k8s.io/klog/v2"
)

var kindMu sync.Mutex

// ResolveAsset returns the expected kind release asset filename for the given OS and architecture.
func ResolveAsset(goos, goarch string) (string, error) {
	switch goos {
	case "linux":
		switch goarch {
		case "amd64", "arm64":
			return "kind-linux-" + goarch, nil
		default:
			return "", fmt.Errorf("unsupported linux architecture: %s", goarch)
		}
	case "darwin":
		switch goarch {
		case "amd64", "arm64":
			return "kind-darwin-" + goarch, nil
		default:
			return "", fmt.Errorf("unsupported darwin architecture: %s", goarch)
		}
	case "windows":
		switch goarch {
		case "amd64":
			return "kind-windows-amd64", nil
		default:
			return "", fmt.Errorf("unsupported windows architecture: %s", goarch)
		}
	default:
		return "", fmt.Errorf("unsupported operating system: %s", goos)
	}
}

// EnsureKind ensures that the kind binary is downloaded, verified against checksum,
// and cached in AP_CACHE_DIR/tools/kind/<tag>/bin/kind. Returns the path to the executable binary.
func EnsureKind(ctx context.Context, version string) (string, error) {
	kindMu.Lock()
	defer kindMu.Unlock()

	cacheRoot, err := fileutils.CacheDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine cache dir: %w", err)
	}

	binName := "kind"
	if runtime.GOOS == "windows" {
		binName = "kind.exe"
	}

	// Fast path for pinned versions already cached and executable
	if version != "" && version != "latest" {
		tag := version
		if !strings.HasPrefix(tag, "v") {
			tag = "v" + tag
		}
		installDir := filepath.Join(cacheRoot, "tools", "kind", tag)
		binPath := filepath.Join(installDir, "bin", binName)
		if fileutils.IsExecutable(binPath) {
			klog.Infof("Using kind %s", tag)
			return binPath, nil
		}
	}

	resolvedVer, downloadURL, expectedDigest, err := resolveKindRelease(ctx, version)
	if err != nil {
		return "", err
	}

	if envDigest := os.Getenv("KIND_DOWNLOAD_DIGEST"); envDigest != "" {
		expectedDigest = envDigest
	}
	if baseURL := os.Getenv("KIND_DOWNLOAD_BASE_URL"); baseURL != "" {
		assetName, assetErr := ResolveAsset(runtime.GOOS, runtime.GOARCH)
		if assetErr != nil {
			return "", assetErr
		}
		downloadURL = fmt.Sprintf("%s/%s", strings.TrimRight(baseURL, "/"), assetName)
	}

	tag := resolvedVer
	if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}

	installDir := filepath.Join(cacheRoot, "tools", "kind", tag)
	binDir := filepath.Join(installDir, "bin")
	binPath := filepath.Join(binDir, binName)
	if fileutils.IsExecutable(binPath) {
		klog.Infof("Using kind %s", tag)
		return binPath, nil
	}

	tmpFile, err := fetch.Download(ctx, downloadURL, expectedDigest, cacheRoot)
	if err != nil {
		return "", fmt.Errorf("failed to download kind from %s: %w", downloadURL, err)
	}
	defer func() {
		if rmErr := os.Remove(tmpFile); rmErr != nil && !os.IsNotExist(rmErr) {
			klog.V(2).Infof("failed to remove temporary file %s: %v", tmpFile, rmErr)
		}
	}()

	if err := os.MkdirAll(binDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", binDir, err)
	}

	if err := fileutils.CopyFile(tmpFile, binPath, 0755); err != nil {
		return "", fmt.Errorf("failed to install kind binary to %s: %w", binPath, err)
	}

	if err := os.Chmod(binPath, 0755); err != nil {
		return "", fmt.Errorf("failed to chmod kind binary %s: %w", binPath, err)
	}

	klog.Infof("Installed kind %s to %s", tag, binPath)
	return binPath, nil
}

func resolveKindRelease(ctx context.Context, version string) (resolvedVer, downloadURL, expectedDigest string, err error) {
	requestedVer := strings.TrimSpace(version)
	if requestedVer == "" || requestedVer == "latest" {
		rel := Latest()
		if rel == nil {
			return "", "", "", fmt.Errorf("no kind releases found in versions.yaml")
		}
		resolvedVer = rel.Version
		assetName, err := ResolveAsset(runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return "", "", "", err
		}
		digest, ok := rel.Assets[assetName]
		if !ok || digest == "" {
			return "", "", "", fmt.Errorf("kind %s has no asset %s in versions.yaml", resolvedVer, assetName)
		}
		baseURL := os.Getenv("KIND_DOWNLOAD_BASE_URL")
		if baseURL == "" {
			baseURL = fmt.Sprintf("https://github.com/kubernetes-sigs/kind/releases/download/%s", resolvedVer)
		}
		return resolvedVer, fmt.Sprintf("%s/%s", strings.TrimRight(baseURL, "/"), assetName), digest, nil
	}

	if rel, ok := Lookup(requestedVer); ok {
		resolvedVer = rel.Version
		assetName, err := ResolveAsset(runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return "", "", "", err
		}
		digest, ok := rel.Assets[assetName]
		if !ok || digest == "" {
			return "", "", "", fmt.Errorf("kind %s has no asset %s in versions.yaml", resolvedVer, assetName)
		}
		baseURL := os.Getenv("KIND_DOWNLOAD_BASE_URL")
		if baseURL == "" {
			baseURL = fmt.Sprintf("https://github.com/kubernetes-sigs/kind/releases/download/%s", resolvedVer)
		}
		return resolvedVer, fmt.Sprintf("%s/%s", strings.TrimRight(baseURL, "/"), assetName), digest, nil
	}

	// On a miss: warn and resolve from GitHub API
	klog.Warningf("kind %s is not in versions.yaml; resolving it from the GitHub API, which is rate limited; run 'ap tool update-versions' to add it", requestedVer)
	return resolvePinnedKindRelease(ctx, requestedVer)
}

func resolvePinnedKindRelease(ctx context.Context, version string) (resolvedVer, downloadURL, expectedDigest string, err error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
	}

	resolvedVer = version
	if !strings.HasPrefix(resolvedVer, "v") {
		resolvedVer = "v" + resolvedVer
	}

	assetName, err := ResolveAsset(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", "", "", err
	}

	baseURL := os.Getenv("KIND_DOWNLOAD_BASE_URL")
	if baseURL != "" {
		return resolvedVer, fmt.Sprintf("%s/%s", strings.TrimRight(baseURL, "/"), assetName), os.Getenv("KIND_DOWNLOAD_DIGEST"), nil
	}

	apiBase := os.Getenv("KIND_GITHUB_API_BASE_URL")
	if apiBase == "" {
		apiBase = os.Getenv("GITHUB_API_BASE_URL")
	}
	if apiBase == "" {
		apiBase = "https://api.github.com"
	}

	tagURL := fmt.Sprintf("%s/repos/kubernetes-sigs/kind/releases/tags/%s", strings.TrimRight(apiBase, "/"), resolvedVer)
	rel, err := github.FetchRelease(ctx, tagURL)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to fetch release for kind %s: %w", resolvedVer, err)
	}

	asset, found := github.FindAsset(rel.Assets, assetName)
	if !found {
		return "", "", "", fmt.Errorf("release %s has no asset %s", resolvedVer, assetName)
	}

	digest := asset.Digest
	if digest == "" {
		shaAsset, shaFound := github.FindAsset(rel.Assets, assetName+".sha256sum")
		if shaFound && shaAsset.BrowserDownloadURL != "" {
			content, err := github.GetText(ctx, shaAsset.BrowserDownloadURL)
			if err != nil {
				klog.V(2).Infof("failed to fetch kind checksum asset %s: %v", shaAsset.BrowserDownloadURL, err)
			} else {
				fields := strings.Fields(content)
				if len(fields) >= 1 && len(fields[0]) == 64 {
					digest = "sha256:" + fields[0]
				}
			}
		}
	}

	return resolvedVer, asset.BrowserDownloadURL, digest, nil
}
