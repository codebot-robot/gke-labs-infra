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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/fileutils"
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

	httpClient = http.DefaultClient
	timeNow    = time.Now
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
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
}

type latestMemo struct {
	ResolvedAt time.Time      `json:"resolved_at"`
	Release    *githubRelease `json:"release"`
}

// EnsureProtoc ensures that protoc is downloaded, verified against the release digest (if available),
// extracted into the cache directory, and executable.
// If version is "latest" or empty, it queries the GitHub releases API (memoized for 24 hours in tools/protoc/latest.json).
// If the API is unreachable, it falls back to the newest version already in the cache with a warning.
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
	resolvedVer, downloadURL, expectedDigest, err := resolveProtocRelease(ctx, version, cacheRoot, binName)
	if err != nil {
		return "", "", err
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

func resolveProtocRelease(ctx context.Context, version, cacheRoot, binName string) (resolvedVer, downloadURL, expectedDigest string, err error) {
	if version == "" || version == "latest" {
		return resolveLatestProtocRelease(ctx, cacheRoot, binName)
	}
	return resolvePinnedProtocRelease(ctx, version)
}

func resolveLatestProtocRelease(ctx context.Context, cacheRoot, binName string) (resolvedVer, downloadURL, expectedDigest string, err error) {
	var rel *githubRelease
	memoPath := filepath.Join(cacheRoot, "tools", "protoc", "latest.json")

	// Check if we have a valid memo within 24 hours
	memo, readErr := readMemo(memoPath)
	if readErr != nil {
		if !os.IsNotExist(readErr) {
			klog.V(2).Infof("failed to read latest.json: %v", readErr)
		}
	} else {
		age := timeNow().Sub(memo.ResolvedAt)
		if age >= 0 && age < 24*time.Hour {
			rel = memo.Release
		}
	}

	if rel == nil {
		baseURL := os.Getenv("PROTOC_DOWNLOAD_BASE_URL")
		apiURL := "https://api.github.com/repos/protocolbuffers/protobuf/releases/latest"
		if baseURL != "" {
			apiURL = fmt.Sprintf("%s/releases/latest", strings.TrimRight(baseURL, "/"))
		}

		var apiErr error
		rel, apiErr = fetchGitHubRelease(ctx, apiURL)
		if apiErr != nil {
			// Fallback to expired memo if available
			if memo != nil {
				klog.Warningf("failed to resolve latest protoc from GitHub API: %v; falling back to memoized release from %s", apiErr, memo.ResolvedAt.Format(time.RFC3339))
				rel = memo.Release
			} else {
				// Fall back to newest version in cache
				cachedVer := findNewestCachedProtoc(cacheRoot, binName)
				if cachedVer != "" {
					klog.Warningf("failed to resolve latest protoc from GitHub API: %v; falling back to cached %s", apiErr, cachedVer)
					return cachedVer, "", "", nil
				}
				return "", "", "", fmt.Errorf("failed to resolve latest protoc from GitHub and no cached version found: %w", apiErr)
			}
		} else {
			// Memoize successful latest resolution
			newMemo := latestMemo{
				ResolvedAt: timeNow(),
				Release:    rel,
			}
			if writeErr := writeMemo(memoPath, newMemo); writeErr != nil {
				klog.V(2).Infof("failed to write latest.json: %v", writeErr)
			}
		}
	}

	resolvedVer = strings.TrimPrefix(rel.TagName, "v")
	assetName, err := ResolveProtocAsset(resolvedVer, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", "", "", err
	}
	asset, found := findAsset(rel.Assets, assetName)
	if !found {
		return "", "", "", fmt.Errorf("release %s has no asset %s", resolvedVer, assetName)
	}

	return resolvedVer, asset.BrowserDownloadURL, asset.Digest, nil
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

	tagURL := fmt.Sprintf("https://api.github.com/repos/protocolbuffers/protobuf/releases/tags/v%s", resolvedVer)
	rel, apiErr := fetchGitHubRelease(ctx, tagURL)
	if apiErr != nil {
		tagURL = fmt.Sprintf("https://api.github.com/repos/protocolbuffers/protobuf/releases/tags/%s", resolvedVer)
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

	resp, err := httpClient.Do(req)
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
// If requestedVer is "latest" or empty, it runs `go install <module>@latest`, reads the version with `go version -m`,
// and caches it under that version. If the install fails, it falls back to the newest cached version with a warning.
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

	isLatest := requestedVer == "" || requestedVer == "latest"

	// If a specific version is requested, check if it's already cached
	if !isLatest {
		ver := requestedVer
		if !strings.HasPrefix(ver, "v") {
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
		// Fallback to newest cached version if install fails (e.g. offline)
		cachedVer := findNewestCachedPlugin(cacheRoot, normalizedName, binName)
		if cachedVer != "" {
			klog.Warningf("failed to install %s@latest: %v; falling back to cached %s", normalizedName, err, cachedVer)
			klog.Infof("Using %s %s", normalizedName, cachedVer)
			return filepath.Join(cacheRoot, "tools", "plugins", normalizedName, cachedVer, "bin"), nil
		}
		return "", fmt.Errorf("failed to install %s@latest and no cached version found: %w\n%s", normalizedName, err, strings.TrimSpace(string(output)))
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

	resp, err := httpClient.Do(req)
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

func findNewestCachedProtoc(cacheRoot, binName string) string {
	protocDir := filepath.Join(cacheRoot, "tools", "protoc")
	entries, err := os.ReadDir(protocDir)
	if err != nil {
		return ""
	}

	var versions []string
	for _, e := range entries {
		if e.IsDir() && isExecutable(filepath.Join(protocDir, e.Name(), "bin", binName)) {
			versions = append(versions, e.Name())
		}
	}
	if len(versions) == 0 {
		return ""
	}
	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i], versions[j]) > 0
	})
	return versions[0]
}

func findNewestCachedPlugin(cacheRoot, pluginName, binName string) string {
	pluginDir := filepath.Join(cacheRoot, "tools", "plugins", pluginName)
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return ""
	}

	var versions []string
	for _, e := range entries {
		if e.IsDir() && e.Name() != ".staging" && isExecutable(filepath.Join(pluginDir, e.Name(), "bin", binName)) {
			versions = append(versions, e.Name())
		}
	}
	if len(versions) == 0 {
		return ""
	}
	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i], versions[j]) > 0
	})
	return versions[0]
}

func compareVersions(v1, v2 string) int {
	parts1 := strings.Split(strings.TrimPrefix(v1, "v"), ".")
	parts2 := strings.Split(strings.TrimPrefix(v2, "v"), ".")
	for i := 0; i < len(parts1) && i < len(parts2); i++ {
		var n1, n2 int
		if val, err := strconv.Atoi(parts1[i]); err != nil {
			n1 = 0
		} else {
			n1 = val
		}
		if val, err := strconv.Atoi(parts2[i]); err != nil {
			n2 = 0
		} else {
			n2 = val
		}
		if n1 != n2 {
			return n1 - n2
		}
	}
	return len(parts1) - len(parts2)
}

func readMemo(memoPath string) (*latestMemo, error) {
	data, err := os.ReadFile(memoPath)
	if err != nil {
		return nil, err
	}
	var memo latestMemo
	if err := json.Unmarshal(data, &memo); err != nil {
		return nil, err
	}
	if memo.Release == nil {
		return nil, fmt.Errorf("memo has nil release")
	}
	return &memo, nil
}

func writeMemo(memoPath string, memo latestMemo) error {
	memoBytes, err := json.MarshalIndent(memo, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(memoPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(memoPath, memoBytes, 0644)
}
