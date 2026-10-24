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

package docker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/scoped"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/tool/github"
	"gopkg.in/yaml.v3"
)

func TestUpdateVersionsFile(t *testing.T) {
	tests := []struct {
		name               string
		dockerListings     map[string][]string // platform -> slice of filenames in HTML
		buildxReleasesFunc func(serverURL string) []github.Release
		wantDockerVersions []string
		wantBuildxVersions []string
	}{
		{
			name: "filters incomplete platforms and prereleases, sorts newest first",
			dockerListings: map[string][]string{
				"linux-amd64": {
					"docker-29.9.2.tgz",
					"docker-29.8.2.tgz",
					"docker-27.0.0.tgz",                 // not on windows
					"docker-30.0.0-rc.1.tgz",            // prerelease
					"docker-rootless-extras-29.9.2.tgz", // extras, ignored
				},
				"linux-arm64": {
					"docker-29.9.2.tgz",
					"docker-29.8.2.tgz",
					"docker-27.0.0.tgz",
					"docker-30.0.0-rc.1.tgz",
				},
				"darwin-amd64": {
					"docker-29.9.2.tgz",
					"docker-29.8.2.tgz",
					"docker-27.0.0.tgz",
					"docker-30.0.0-rc.1.tgz",
				},
				"darwin-arm64": {
					"docker-29.9.2.tgz",
					"docker-29.8.2.tgz",
					"docker-27.0.0.tgz",
					"docker-30.0.0-rc.1.tgz",
				},
				"windows-amd64": {
					"docker-29.9.2.zip",
					"docker-29.8.2.zip",
					"docker-30.0.0-rc.1.zip",
				},
			},
			buildxReleasesFunc: func(serverURL string) []github.Release {
				return []github.Release{
					{
						TagName: "v0.39.0-rc1", // prerelease, ignored
						Assets: []github.Asset{
							{
								Name:               "checksums.txt",
								BrowserDownloadURL: serverURL + "/buildx/v0.39.0-rc1/checksums.txt",
							},
						},
					},
					{
						TagName: "v0.38.1",
						Assets: []github.Asset{
							{
								Name:               "checksums.txt",
								BrowserDownloadURL: serverURL + "/buildx/v0.38.1/checksums.txt",
							},
							{
								Name:               "checksums-signed.txt",
								BrowserDownloadURL: serverURL + "/buildx/v0.38.1/checksums-signed.txt",
							},
						},
					},
					{
						TagName: "v0.38.0",
						Assets: []github.Asset{
							{
								Name:               "checksums.txt",
								BrowserDownloadURL: serverURL + "/buildx/v0.38.0/checksums.txt",
							},
							{
								Name:               "checksums-signed.txt",
								BrowserDownloadURL: serverURL + "/buildx/v0.38.0/checksums-signed.txt",
							},
						},
					},
					{
						TagName: "v0.37.0", // incomplete platform (missing windows), ignored
						Assets: []github.Asset{
							{
								Name:               "checksums.txt",
								BrowserDownloadURL: serverURL + "/buildx/v0.37.0/checksums.txt",
							},
						},
					},
				}
			},
			wantDockerVersions: []string{"29.9.2", "29.8.2"},
			wantBuildxVersions: []string{"0.38.1", "0.38.0"},
		},
		{
			name: "caps to newest 5 releases",
			dockerListings: map[string][]string{
				"linux-amd64":   {"docker-29.7.0.tgz", "docker-29.6.0.tgz", "docker-29.5.0.tgz", "docker-29.4.0.tgz", "docker-29.3.0.tgz", "docker-29.2.0.tgz", "docker-29.1.0.tgz"},
				"linux-arm64":   {"docker-29.7.0.tgz", "docker-29.6.0.tgz", "docker-29.5.0.tgz", "docker-29.4.0.tgz", "docker-29.3.0.tgz", "docker-29.2.0.tgz", "docker-29.1.0.tgz"},
				"darwin-amd64":  {"docker-29.7.0.tgz", "docker-29.6.0.tgz", "docker-29.5.0.tgz", "docker-29.4.0.tgz", "docker-29.3.0.tgz", "docker-29.2.0.tgz", "docker-29.1.0.tgz"},
				"darwin-arm64":  {"docker-29.7.0.tgz", "docker-29.6.0.tgz", "docker-29.5.0.tgz", "docker-29.4.0.tgz", "docker-29.3.0.tgz", "docker-29.2.0.tgz", "docker-29.1.0.tgz"},
				"windows-amd64": {"docker-29.7.0.zip", "docker-29.6.0.zip", "docker-29.5.0.zip", "docker-29.4.0.zip", "docker-29.3.0.zip", "docker-29.2.0.zip", "docker-29.1.0.zip"},
			},
			buildxReleasesFunc: func(serverURL string) []github.Release {
				vers := []string{"0.37.0", "0.36.0", "0.35.0", "0.34.0", "0.33.0", "0.32.0", "0.31.0"}
				var rels []github.Release
				for _, v := range vers {
					rels = append(rels, github.Release{
						TagName: "v" + v,
						Assets: []github.Asset{
							{
								Name:               "checksums.txt",
								BrowserDownloadURL: fmt.Sprintf("%s/buildx/v%s/checksums.txt", serverURL, v),
							},
							{
								Name:               "checksums-signed.txt",
								BrowserDownloadURL: fmt.Sprintf("%s/buildx/v%s/checksums-signed.txt", serverURL, v),
							},
						},
					})
				}
				return rels
			},
			wantDockerVersions: []string{"29.7.0", "29.6.0", "29.5.0", "29.4.0", "29.3.0"},
			wantBuildxVersions: []string{"0.37.0", "0.36.0", "0.35.0", "0.34.0", "0.33.0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := r.URL.Path

				// Handle GitHub API releases endpoint
				if strings.HasPrefix(path, "/repos/docker/buildx/releases") {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(tt.buildxReleasesFunc(server.URL))
					return
				}

				// Handle Buildx checksum files
				if strings.HasPrefix(path, "/buildx/") {
					w.Header().Set("Content-Type", "text/plain")
					parts := strings.Split(strings.TrimPrefix(path, "/buildx/"), "/")
					if len(parts) >= 2 {
						tag := parts[0]
						file := parts[1]
						if file == "checksums.txt" {
							if tag == "v0.37.0" && tt.name != "caps to newest 5 releases" {
								// Incomplete release missing windows
								_, _ = fmt.Fprintf(w, "1111111111111111111111111111111111111111111111111111111111111111 *buildx-%s.linux-amd64\n", tag)
								_, _ = fmt.Fprintf(w, "2222222222222222222222222222222222222222222222222222222222222222 *buildx-%s.linux-arm64\n", tag)
								_, _ = fmt.Fprintf(w, "3333333333333333333333333333333333333333333333333333333333333333 *buildx-%s.darwin-amd64\n", tag)
								_, _ = fmt.Fprintf(w, "4444444444444444444444444444444444444444444444444444444444444444 *buildx-%s.darwin-arm64\n", tag)
								return
							}
							_, _ = fmt.Fprintf(w, "1111111111111111111111111111111111111111111111111111111111111111 *buildx-%s.linux-amd64\n", tag)
							_, _ = fmt.Fprintf(w, "2222222222222222222222222222222222222222222222222222222222222222 *buildx-%s.linux-arm64\n", tag)
							_, _ = fmt.Fprintf(w, "5555555555555555555555555555555555555555555555555555555555555555 *buildx-%s.windows-amd64.exe\n", tag)
							return
						}
						if file == "checksums-signed.txt" {
							_, _ = fmt.Fprintf(w, "3333333333333333333333333333333333333333333333333333333333333333 *buildx-%s.darwin-amd64\n", tag)
							_, _ = fmt.Fprintf(w, "4444444444444444444444444444444444444444444444444444444444444444 *buildx-%s.darwin-arm64\n", tag)
							return
						}
					}
					http.NotFound(w, r)
					return
				}

				// Handle Docker asset downloads
				if strings.HasSuffix(path, ".tgz") || strings.HasSuffix(path, ".zip") {
					w.Header().Set("Content-Type", "application/octet-stream")
					content := "mock-docker-binary-content-for-" + filepath.Base(path)
					_, _ = w.Write([]byte(content))
					return
				}

				// Handle Docker directory listings
				for _, p := range dockerListingPlatforms {
					listingPath := fmt.Sprintf("/%s/static/stable/%s/", p.osPart, p.archPart)
					if path == listingPath {
						w.Header().Set("Content-Type", "text/html")
						var sb strings.Builder
						sb.WriteString("<html><body>\n")
						for _, fn := range tt.dockerListings[p.platform] {
							_, _ = fmt.Fprintf(&sb, "<a href=\"%s\">%s</a><br/>\n", fn, fn)
						}
						sb.WriteString("</body></html>\n")
						_, _ = w.Write([]byte(sb.String()))
						return
					}
				}

				http.NotFound(w, r)
			}))
			defer server.Close()

			t.Setenv("DOCKER_DOWNLOAD_BASE_URL", server.URL)
			t.Setenv("BUILDX_GITHUB_API_BASE_URL", server.URL)

			tempRepo := t.TempDir()
			targetDir := filepath.Join(tempRepo, "ap", "pkg", "tool", "docker")
			if err := os.MkdirAll(targetDir, 0755); err != nil {
				t.Fatal(err)
			}

			ctx := scoped.WithHTTPClient(t.Context(), server.Client())
			err := UpdateVersionsFile(ctx, tempRepo)
			if err != nil {
				t.Fatalf("UpdateVersionsFile failed: %v", err)
			}

			written, err := os.ReadFile(filepath.Join(targetDir, "versions.yaml"))
			if err != nil {
				t.Fatalf("failed to read written versions.yaml: %v", err)
			}

			var parsed versionsData
			if err := yaml.Unmarshal(written, &parsed); err != nil {
				t.Fatalf("failed to unmarshal written versions.yaml: %v", err)
			}

			if len(parsed.Docker) != len(tt.wantDockerVersions) {
				t.Fatalf("got %d docker versions, want %d", len(parsed.Docker), len(tt.wantDockerVersions))
			}
			for i, want := range tt.wantDockerVersions {
				if parsed.Docker[i].Version != want {
					t.Errorf("docker[%d] = %q, want %q", i, parsed.Docker[i].Version, want)
				}
				// Verify all platforms have sha256 digests
				for _, p := range dockerListingPlatforms {
					d, ok := parsed.Docker[i].Assets[p.platform]
					if !ok || !strings.HasPrefix(d, "sha256:") {
						t.Errorf("docker[%d] missing valid digest for %s: %q", i, p.platform, d)
					}
					// Verify computed digest matches sha256 of mock content
					expectedContent := "mock-docker-binary-content-for-" + fmt.Sprintf("docker-%s%s", want, p.ext)
					sum := sha256.Sum256([]byte(expectedContent))
					expectedDigest := "sha256:" + hex.EncodeToString(sum[:])
					if d != expectedDigest {
						t.Errorf("docker[%d] %s digest = %q, want %q", i, p.platform, d, expectedDigest)
					}
				}
			}

			if len(parsed.Buildx) != len(tt.wantBuildxVersions) {
				t.Fatalf("got %d buildx versions, want %d", len(parsed.Buildx), len(tt.wantBuildxVersions))
			}
			for i, want := range tt.wantBuildxVersions {
				if parsed.Buildx[i].Version != want {
					t.Errorf("buildx[%d] = %q, want %q", i, parsed.Buildx[i].Version, want)
				}
				for _, plat := range supportedPlatforms {
					d, ok := parsed.Buildx[i].Assets[plat]
					if !ok || !strings.HasPrefix(d, "sha256:") {
						t.Errorf("buildx[%d] missing valid digest for %s: %q", i, plat, d)
					}
				}
			}
		})
	}
}

func TestUpdate_SkipFlags(t *testing.T) {
	initialYAML := `docker:
    - version: 29.9.2
      assets:
        darwin-amd64: sha256:089c3f618df88bdef4f1257d9d604499136a1ec5168b67d194c260acad2b522f
        darwin-arm64: sha256:ea8a2ce5ac31f9f5f3fb711a7a485e1e46cc7a69798a629a70306f6e6af25b7a
        linux-amd64: sha256:9ac225999fd5e863f7694c728ef23d480616b1e5639328fa309befd4243d69eb
        linux-arm64: sha256:873b3e0207857c3b2f28392fb922f3ea6efe37620c16d4d4aaf2fef409c90b73
        windows-amd64: sha256:b2be959bfed1281ddaf665c0972cba9a62ce30c542b6bcce82f055bf8886bc7d
buildx:
    - version: 0.38.1
      assets:
        darwin-amd64: sha256:76e7a809cc98d34a4c1b7db23feac673def5b8dcdddd9e713ef26598689aa796
        darwin-arm64: sha256:6f20ba2437865311ef99015bf5d1039c3c8e4d2177c1c64aa41b134b5ecd3012
        linux-amd64: sha256:e25d5cd675092bb5d1d03632fdc46cdf3fede7b514dc5f023a105e00d3ce5ad4
        linux-arm64: sha256:3b83b3731916ba433feccaaa689fe5bcdc81c641d92c8e6c2ea865dfa141c97a
        windows-amd64: sha256:fd2e18501910dd535ea3aa7f69010b3cfdfba0f47aa27674124554386dfa26f3
`

	t.Run("skip docker touches no docker endpoints and leaves docker section bytes unchanged", func(t *testing.T) {
		tempRepo := t.TempDir()
		targetDir := filepath.Join(tempRepo, "ap", "pkg", "tool", "docker")
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(targetDir, "versions.yaml"), []byte(initialYAML), 0644); err != nil {
			t.Fatal(err)
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "static/stable") || strings.HasSuffix(r.URL.Path, ".tgz") || strings.HasSuffix(r.URL.Path, ".zip") {
				t.Fatalf("docker endpoint accessed when --docker=false: %s", r.URL.Path)
			}
			if strings.HasPrefix(r.URL.Path, "/repos/docker/buildx/releases") {
				w.Header().Set("Content-Type", "application/json")
				rels := []github.Release{
					{
						TagName: "v0.38.2",
						Assets: []github.Asset{
							{Name: "checksums.txt", BrowserDownloadURL: "http://" + r.Host + "/buildx/v0.38.2/checksums.txt"},
							{Name: "checksums-signed.txt", BrowserDownloadURL: "http://" + r.Host + "/buildx/v0.38.2/checksums-signed.txt"},
						},
					},
				}
				_ = json.NewEncoder(w).Encode(rels)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/buildx/") {
				w.Header().Set("Content-Type", "text/plain")
				if strings.HasSuffix(r.URL.Path, "checksums.txt") {
					_, _ = fmt.Fprintln(w, "1111111111111111111111111111111111111111111111111111111111111111 *buildx-v0.38.2.linux-amd64")
					_, _ = fmt.Fprintln(w, "2222222222222222222222222222222222222222222222222222222222222222 *buildx-v0.38.2.linux-arm64")
					_, _ = fmt.Fprintln(w, "5555555555555555555555555555555555555555555555555555555555555555 *buildx-v0.38.2.windows-amd64.exe")
					return
				}
				if strings.HasSuffix(r.URL.Path, "checksums-signed.txt") {
					_, _ = fmt.Fprintln(w, "3333333333333333333333333333333333333333333333333333333333333333 *buildx-v0.38.2.darwin-amd64")
					_, _ = fmt.Fprintln(w, "4444444444444444444444444444444444444444444444444444444444444444 *buildx-v0.38.2.darwin-arm64")
					return
				}
			}
			http.NotFound(w, r)
		}))
		defer server.Close()

		t.Setenv("DOCKER_DOWNLOAD_BASE_URL", server.URL)
		t.Setenv("BUILDX_GITHUB_API_BASE_URL", server.URL)

		ctx := scoped.WithHTTPClient(t.Context(), server.Client())
		err := Update(ctx, tempRepo, UpdateOptions{Docker: false, Buildx: true})
		if err != nil {
			t.Fatalf("Update failed: %v", err)
		}

		data, err := os.ReadFile(filepath.Join(targetDir, "versions.yaml"))
		if err != nil {
			t.Fatal(err)
		}

		// Check that the docker: section bytes are identical
		dockerInitial := strings.Split(initialYAML, "buildx:\n")[0]
		dockerAfter := strings.Split(string(data), "buildx:\n")[0]
		if dockerAfter != dockerInitial {
			t.Errorf("docker section bytes changed:\ngot:\n%s\nwant:\n%s", dockerAfter, dockerInitial)
		}

		var parsed versionsData
		if err := yaml.Unmarshal(data, &parsed); err != nil {
			t.Fatal(err)
		}
		if len(parsed.Buildx) != 1 || parsed.Buildx[0].Version != "0.38.2" {
			t.Errorf("expected buildx to be updated to 0.38.2, got: %v", parsed.Buildx)
		}
	})

	t.Run("skip buildx touches no github endpoints and leaves buildx section untouched", func(t *testing.T) {
		tempRepo := t.TempDir()
		targetDir := filepath.Join(tempRepo, "ap", "pkg", "tool", "docker")
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(targetDir, "versions.yaml"), []byte(initialYAML), 0644); err != nil {
			t.Fatal(err)
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "buildx") || strings.Contains(r.URL.Path, "repos") {
				t.Fatalf("buildx endpoint accessed when --buildx=false: %s", r.URL.Path)
			}
			if strings.HasSuffix(r.URL.Path, "/") {
				w.Header().Set("Content-Type", "text/html")
				_, _ = fmt.Fprintln(w, `<a href="docker-30.0.0.tgz">docker-30.0.0.tgz</a>`)
				_, _ = fmt.Fprintln(w, `<a href="docker-30.0.0.zip">docker-30.0.0.zip</a>`)
				return
			}
			if strings.HasSuffix(r.URL.Path, ".tgz") || strings.HasSuffix(r.URL.Path, ".zip") {
				w.Header().Set("Content-Type", "application/octet-stream")
				_, _ = w.Write([]byte("mock-30-content"))
				return
			}
			http.NotFound(w, r)
		}))
		defer server.Close()

		t.Setenv("DOCKER_DOWNLOAD_BASE_URL", server.URL)
		t.Setenv("BUILDX_GITHUB_API_BASE_URL", server.URL)

		ctx := scoped.WithHTTPClient(t.Context(), server.Client())
		err := Update(ctx, tempRepo, UpdateOptions{Docker: true, Buildx: false})
		if err != nil {
			t.Fatalf("Update failed: %v", err)
		}

		data, err := os.ReadFile(filepath.Join(targetDir, "versions.yaml"))
		if err != nil {
			t.Fatal(err)
		}

		buildxInitial := "buildx:\n" + strings.Split(initialYAML, "buildx:\n")[1]
		buildxAfter := "buildx:\n" + strings.Split(string(data), "buildx:\n")[1]
		if buildxAfter != buildxInitial {
			t.Errorf("buildx section bytes changed:\ngot:\n%s\nwant:\n%s", buildxAfter, buildxInitial)
		}

		var parsed versionsData
		if err := yaml.Unmarshal(data, &parsed); err != nil {
			t.Fatal(err)
		}
		if len(parsed.Docker) != 1 || parsed.Docker[0].Version != "30.0.0" {
			t.Errorf("expected docker to be updated to 30.0.0, got: %v", parsed.Docker)
		}
	})
}

func TestVersionsYAML_Canonical(t *testing.T) {
	content, err := os.ReadFile("versions.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var parsed versionsData
	if err := yaml.Unmarshal(content, &parsed); err != nil {
		t.Fatal(err)
	}
	canonical, err := yaml.Marshal(&parsed)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != string(canonical) {
		t.Errorf("versions.yaml is not in canonical format;\ngot:\n%s\nwant:\n%s", string(content), string(canonical))
	}
}
