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

package protoc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/fileutils"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/scoped"
	"k8s.io/klog/v2"
)

// KnownGoPlugins maps standard plugin names to their Go module paths.
var KnownGoPlugins = map[string]string{
	"protoc-gen-go":      "google.golang.org/protobuf/cmd/protoc-gen-go",
	"protoc-gen-go-grpc": "google.golang.org/grpc/cmd/protoc-gen-go-grpc",
}

// protocBuiltinGenerators lists output targets handled natively by protoc that do not require external plugins.
var protocBuiltinGenerators = map[string]bool{
	"cpp":    true,
	"csharp": true,
	"java":   true,
	"kotlin": true,
	"objc":   true,
	"php":    true,
	"python": true,
	"pyi":    true,
	"ruby":   true,
}

var (
	protocMu sync.Mutex
	pluginMu sync.Mutex
)

// CacheDir returns the root cache directory for ap tools.
func CacheDir() (string, error) {
	return fileutils.CacheDir()
}

// ResolveProtocAsset returns the expected protobuf release asset filename for the given version and target OS/arch.
func ResolveProtocAsset(version, goos, goarch string) (string, error) {
	normalizedVer := strings.TrimPrefix(version, "v")
	switch goos {
	case "linux":
		var archPart string
		switch goarch {
		case "amd64":
			archPart = "x86_64"
		case "arm64":
			archPart = "aarch_64"
		case "386":
			archPart = "x86_32"
		case "ppc64le":
			archPart = "ppcle_64"
		case "s390x":
			archPart = "s390_64"
		default:
			return "", fmt.Errorf("unsupported linux architecture: %s", goarch)
		}
		return fmt.Sprintf("protoc-%s-linux-%s.zip", normalizedVer, archPart), nil
	case "darwin":
		var archPart string
		switch goarch {
		case "amd64":
			archPart = "x86_64"
		case "arm64":
			archPart = "aarch_64"
		default:
			return "", fmt.Errorf("unsupported darwin architecture: %s", goarch)
		}
		return fmt.Sprintf("protoc-%s-osx-%s.zip", normalizedVer, archPart), nil
	case "windows":
		var archPart string
		switch goarch {
		case "amd64":
			archPart = "win64"
		case "386":
			archPart = "win32"
		default:
			return "", fmt.Errorf("unsupported windows architecture: %s", goarch)
		}
		return fmt.Sprintf("protoc-%s-%s.zip", normalizedVer, archPart), nil
	default:
		return "", fmt.Errorf("unsupported operating system: %s", goos)
	}
}

type githubRelease struct {
	TagName    string        `json:"tag_name"`
	Draft      bool          `json:"draft"`
	Prerelease bool          `json:"prerelease"`
	Assets     []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
}

// EnsureProtoc ensures that protoc is downloaded, verified against the release digest (if available),
// extracted into the cache directory, and executable.
// If version is "latest" or empty, it resolves from versions.yaml without network round-trips.
// If version is found in versions.yaml, it uses the embedded digest without network round-trips.
// If version is not in versions.yaml, it logs a warning and falls back to querying the GitHub API.
func EnsureProtoc(ctx context.Context, version string) (protocBin string, includeDir string, err error) {
	protocMu.Lock()
	defer protocMu.Unlock()

	cacheRoot, err := CacheDir()
	if err != nil {
		return "", "", err
	}

	binName := "protoc"
	if runtime.GOOS == "windows" {
		binName = "protoc.exe"
	}

	// Fast path for pinned versions already cached and executable
	if version != "" && version != "latest" {
		normalizedVer := strings.TrimPrefix(version, "v")
		installDir := filepath.Join(cacheRoot, "tools", "protoc", normalizedVer)
		binPath := filepath.Join(installDir, "bin", binName)
		if isExecutable(binPath) {
			klog.Infof("Using protoc %s", normalizedVer)
			return binPath, filepath.Join(installDir, "include"), nil
		}
	}

	// 1. Resolve release metadata and download URL
	resolvedVer, downloadURL, expectedDigest, err := resolveProtocRelease(ctx, version)
	if err != nil {
		return "", "", err
	}

	if envDigest := os.Getenv("PROTOC_DOWNLOAD_DIGEST"); envDigest != "" {
		expectedDigest = envDigest
	}
	if baseURL := os.Getenv("PROTOC_DOWNLOAD_BASE_URL"); baseURL != "" {
		assetName, assetErr := ResolveProtocAsset(resolvedVer, runtime.GOOS, runtime.GOARCH)
		if assetErr != nil {
			return "", "", assetErr
		}
		downloadURL = fmt.Sprintf("%s/%s", strings.TrimRight(baseURL, "/"), assetName)
	}

	installDir := filepath.Join(cacheRoot, "tools", "protoc", resolvedVer)
	binPath := filepath.Join(installDir, "bin", binName)
	if isExecutable(binPath) {
		klog.Infof("Using protoc %s", resolvedVer)
		return binPath, filepath.Join(installDir, "include"), nil
	}

	// 2. Download the release archive
	tmpZip, err := downloadProtocZip(ctx, downloadURL, expectedDigest, cacheRoot)
	if err != nil {
		return "", "", err
	}
	defer func() {
		if rmErr := os.Remove(tmpZip); rmErr != nil && !os.IsNotExist(rmErr) {
			klog.V(2).Infof("failed to remove temporary file %s: %v", tmpZip, rmErr)
		}
	}()

	// 3. Extract and install archive into installDir
	return installProtocZip(tmpZip, installDir, binName, resolvedVer)
}

func resolveProtocRelease(ctx context.Context, version string) (resolvedVer, downloadURL, expectedDigest string, err error) {
	requestedVer := strings.TrimSpace(version)
	if requestedVer == "" || requestedVer == "latest" {
		rel := Latest()
		if rel == nil {
			return "", "", "", fmt.Errorf("no protoc releases found in versions.yaml")
		}
		resolvedVer = rel.Version
		assetName, err := ResolveProtocAsset(resolvedVer, runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return "", "", "", err
		}
		digest, ok := rel.Assets[assetName]
		if !ok || digest == "" {
			return "", "", "", fmt.Errorf("protoc %s has no asset %s in versions.yaml", resolvedVer, assetName)
		}
		baseURL := os.Getenv("PROTOC_DOWNLOAD_BASE_URL")
		if baseURL == "" {
			baseURL = fmt.Sprintf("https://github.com/protocolbuffers/protobuf/releases/download/v%s", resolvedVer)
		}
		return resolvedVer, fmt.Sprintf("%s/%s", strings.TrimRight(baseURL, "/"), assetName), digest, nil
	}

	if rel, ok := Lookup(requestedVer); ok {
		resolvedVer = rel.Version
		assetName, err := ResolveProtocAsset(resolvedVer, runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return "", "", "", err
		}
		digest, ok := rel.Assets[assetName]
		if !ok || digest == "" {
			return "", "", "", fmt.Errorf("protoc %s has no asset %s in versions.yaml", resolvedVer, assetName)
		}
		baseURL := os.Getenv("PROTOC_DOWNLOAD_BASE_URL")
		if baseURL == "" {
			baseURL = fmt.Sprintf("https://github.com/protocolbuffers/protobuf/releases/download/v%s", resolvedVer)
		}
		return resolvedVer, fmt.Sprintf("%s/%s", strings.TrimRight(baseURL, "/"), assetName), digest, nil
	}

	// On a miss: warn and resolve from GitHub API
	klog.Warningf("protoc %s is not in versions.yaml; resolving it from the GitHub API, which is rate limited; run 'ap tool update-protoc-versions' to add it", requestedVer)
	return resolvePinnedProtocRelease(ctx, requestedVer)
}

func resolvePinnedProtocRelease(ctx context.Context, version string) (resolvedVer, downloadURL, expectedDigest string, err error) {
	resolvedVer = strings.TrimPrefix(version, "v")
	assetName, err := ResolveProtocAsset(resolvedVer, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", "", "", err
	}

	baseURL := os.Getenv("PROTOC_DOWNLOAD_BASE_URL")
	if baseURL != "" {
		return resolvedVer, fmt.Sprintf("%s/%s", strings.TrimRight(baseURL, "/"), assetName), os.Getenv("PROTOC_DOWNLOAD_DIGEST"), nil
	}

	apiBase := os.Getenv("PROTOC_GITHUB_API_BASE_URL")
	if apiBase == "" {
		apiBase = "https://api.github.com"
	}

	tagURL := fmt.Sprintf("%s/repos/protocolbuffers/protobuf/releases/tags/v%s", strings.TrimRight(apiBase, "/"), resolvedVer)
	rel, apiErr := fetchGitHubRelease(ctx, tagURL)
	if apiErr != nil {
		tagURL = fmt.Sprintf("%s/repos/protocolbuffers/protobuf/releases/tags/%s", strings.TrimRight(apiBase, "/"), resolvedVer)
		var retryErr error
		rel, retryErr = fetchGitHubRelease(ctx, tagURL)
		if retryErr != nil {
			return "", "", "", fmt.Errorf("failed to fetch release for protoc %s: %w", resolvedVer, apiErr)
		}
	}

	asset, found := findAsset(rel.Assets, assetName)
	if !found {
		return "", "", "", fmt.Errorf("release %s has no asset %s", resolvedVer, assetName)
	}

	return resolvedVer, asset.BrowserDownloadURL, asset.Digest, nil
}

func downloadProtocZip(ctx context.Context, downloadURL, expectedDigest, cacheRoot string) (string, error) {
	tmpZip, err := os.CreateTemp(cacheRoot, "protoc-*.zip")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary zip file: %w", err)
	}
	tmpZipName := tmpZip.Name()
	defer func() {
		if closeErr := tmpZip.Close(); closeErr != nil {
			klog.V(2).Infof("failed to close temporary file: %v", closeErr)
		}
	}()

	req, err := newGitHubRequest(ctx, downloadURL)
	if err != nil {
		return "", fmt.Errorf("failed to create request for %s: %w", downloadURL, err)
	}

	resp, err := scoped.HTTPClient(ctx).Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download protoc from %s: %w", downloadURL, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			klog.V(2).Infof("failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download protoc from %s: status %s", downloadURL, resp.Status)
	}

	hasher := sha256.New()
	writer := io.MultiWriter(tmpZip, hasher)

	if _, err := io.Copy(writer, resp.Body); err != nil {
		return "", fmt.Errorf("failed writing downloaded protoc zip: %w", err)
	}
	if err := tmpZip.Close(); err != nil {
		return "", fmt.Errorf("failed closing downloaded protoc zip: %w", err)
	}

	// Verify digest if provided
	if expectedDigest != "" {
		expectedHash := strings.TrimPrefix(expectedDigest, "sha256:")
		actualHash := hex.EncodeToString(hasher.Sum(nil))
		if !strings.EqualFold(actualHash, expectedHash) {
			return "", fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHash, actualHash)
		}
	}

	return tmpZipName, nil
}

func installProtocZip(zipPath, installDir, binName, resolvedVer string) (protocBin, includeDir string, err error) {
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create protoc directory %s: %w", installDir, err)
	}

	if err := extractZip(zipPath, installDir); err != nil {
		return "", "", fmt.Errorf("failed to extract protoc zip: %w", err)
	}

	protocBin = filepath.Join(installDir, "bin", binName)
	if err := os.Chmod(protocBin, 0755); err != nil {
		return "", "", fmt.Errorf("failed to chmod protoc binary %s: %w", protocBin, err)
	}

	klog.Infof("Using protoc %s", resolvedVer)
	return protocBin, filepath.Join(installDir, "include"), nil
}

// EnsurePlugin ensures that the specified Go plugin is installed and cached under its exact module version.
// If requestedVer is "latest" or empty, it checks versions.yaml.
// If not found in versions.yaml or config, it warns and runs `go install <module>@latest`.
func EnsurePlugin(ctx context.Context, pluginName, requestedVer string, cfgPlugins map[string]string) (string, error) {
	pluginMu.Lock()
	defer pluginMu.Unlock()

	normalizedName := pluginName
	if !strings.HasPrefix(normalizedName, "protoc-gen-") {
		normalizedName = "protoc-gen-" + normalizedName
	}

	shortName := strings.TrimPrefix(normalizedName, "protoc-gen-")

	var module string
	if m, ok := KnownGoPlugins[normalizedName]; ok {
		module = m
	} else if m, ok := cfgPlugins[normalizedName]; ok {
		module = m
	} else if m, ok := cfgPlugins[shortName]; ok {
		module = m
	}

	if module == "" {
		return "", fmt.Errorf("unknown protoc plugin %q: to use custom plugins, declare them in .ap/proto.yaml under plugins with their Go module path (e.g. %s: <module>@<version>)", pluginName, pluginName)
	}

	// If module declaration in config had @version in it (e.g. example.com/cmd/protoc-gen-foo@v1.0.0)
	if atIdx := strings.LastIndex(module, "@"); atIdx != -1 {
		if requestedVer == "" || requestedVer == "latest" {
			requestedVer = module[atIdx+1:]
		}
		module = module[:atIdx]
	}

	cacheRoot, err := CacheDir()
	if err != nil {
		return "", err
	}

	binName := normalizedName
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}

	var targetVer string
	if requestedVer != "" && requestedVer != "latest" {
		targetVer = requestedVer
	} else if yamlVer, ok := PluginVersion(normalizedName); ok {
		targetVer = yamlVer
	} else {
		klog.Warningf("plugin %s is not in versions.yaml; resolving @latest via go install; run 'ap tool update-protoc-versions' to pin it", normalizedName)
		targetVer = "latest"
	}

	// If a specific version is requested, check if it's already cached
	if targetVer != "latest" {
		ver := targetVer
		if !strings.HasPrefix(ver, "v") && !strings.Contains(ver, "/") {
			ver = "v" + ver
		}
		targetBinDir := filepath.Join(cacheRoot, "tools", "plugins", normalizedName, ver, "bin")
		targetBin := filepath.Join(targetBinDir, binName)
		if isExecutable(targetBin) {
			klog.Infof("Using %s %s", normalizedName, ver)
			return targetBinDir, nil
		}

		if err := os.MkdirAll(targetBinDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create plugin directory %s: %w", targetBinDir, err)
		}

		cmd := exec.CommandContext(ctx, "go", "install", fmt.Sprintf("%s@%s", module, ver))
		cmd.Env = append(os.Environ(), "GOBIN="+targetBinDir, "GOTOOLCHAIN=auto")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("failed to install plugin %s@%s: %w\n%s", normalizedName, ver, err, strings.TrimSpace(string(output)))
		}

		if !isExecutable(targetBin) {
			return "", fmt.Errorf("expected plugin binary was not produced at %s", targetBin)
		}
		if err := os.Chmod(targetBin, 0755); err != nil {
			return "", fmt.Errorf("failed to chmod plugin binary %s: %w", targetBin, err)
		}

		klog.Infof("Using %s %s", normalizedName, ver)
		return targetBinDir, nil
	}

	// For @latest: install to a staging dir, inspect version with `go version -m`, then cache under it
	stagingBinDir := filepath.Join(cacheRoot, "tools", "plugins", normalizedName, ".staging", "bin")
	stagingBin := filepath.Join(stagingBinDir, binName)
	stagingDir := filepath.Dir(stagingBinDir)
	if err := os.RemoveAll(stagingDir); err != nil {
		klog.V(2).Infof("failed to remove existing staging directory %s: %v", stagingDir, err)
	}
	if err := os.MkdirAll(stagingBinDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create staging directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(stagingDir); err != nil {
			klog.V(2).Infof("failed to cleanup staging directory %s: %v", stagingDir, err)
		}
	}()

	cmd := exec.CommandContext(ctx, "go", "install", module+"@latest")
	cmd.Env = append(os.Environ(), "GOBIN="+stagingBinDir, "GOTOOLCHAIN=auto")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to install %s@latest: %w\n%s", normalizedName, err, strings.TrimSpace(string(output)))
	}

	resolvedVer, err := readPluginVersion(ctx, stagingBin)
	if err != nil || resolvedVer == "" {
		resolvedVer = "latest"
	}

	targetBinDir := filepath.Join(cacheRoot, "tools", "plugins", normalizedName, resolvedVer, "bin")
	targetBin := filepath.Join(targetBinDir, binName)
	if err := os.MkdirAll(targetBinDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create target plugin directory: %w", err)
	}

	// Move staging binary to target
	if err := os.Rename(stagingBin, targetBin); err != nil {
		// If rename fails (e.g. cross-device), copy
		if copyErr := fileutils.CopyFile(stagingBin, targetBin, 0755); copyErr != nil {
			return "", fmt.Errorf("failed to copy binary to cache: %w", copyErr)
		}
	}
	if err := os.Chmod(targetBin, 0755); err != nil {
		return "", fmt.Errorf("failed to chmod plugin binary %s: %w", targetBin, err)
	}

	klog.Infof("Using %s %s", normalizedName, resolvedVer)
	return targetBinDir, nil
}

func readPluginVersion(ctx context.Context, binPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "go", "version", "-m", binPath)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "mod" {
			return fields[2], nil
		}
	}
	return "", fmt.Errorf("module version not found in build info")
}

func newGitHubRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req, nil
}

func fetchGitHubRelease(ctx context.Context, url string) (*githubRelease, error) {
	req, err := newGitHubRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	resp, err := scoped.HTTPClient(ctx).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			klog.V(2).Infof("failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s returned status %s", url, resp.Status)
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("failed to decode GitHub release response: %w", err)
	}
	return &rel, nil
}

func findAsset(assets []githubAsset, name string) (githubAsset, bool) {
	for _, a := range assets {
		if a.Name == name {
			return a, true
		}
	}
	return githubAsset{}, false
}

func isExecutable(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return fi.Mode()&0111 != 0
}
