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

package linediff

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

const fixtureA = `version = 2
required_plugins = ["io.containerd.grpc.v1.cri"]
# Kubernetes doesn't use containerd restart manager.
disabled_plugins = ["io.containerd.internal.v1.restart"]
oom_score = -999

[debug]
  level = "info"

[grpc]
  gid = 412

[plugins."io.containerd.grpc.v1.cri"]
  enable_cdi = true
  stream_server_address = "127.0.0.1"
  max_container_log_line_size = 262144
  sandbox_image = "gke.gcr.io/pause:3.8@sha256:880e63f94b145e46f1b1082bb71b85e21f16b99b180b9996407d61240ceb9830"
  image_pull_progress_timeout = "5m"
[plugins."io.containerd.grpc.v1.cri".cni]
  bin_dir = "/home/kubernetes/bin"
  conf_dir = "/etc/cni/net.d"
  conf_template = ""
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc.options]
  SystemdCgroup = true
[plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]
  endpoint = ["https://mirror.gcr.io","https://registry-1.docker.io"]
[metrics]
 address = "127.0.0.1:1338"
[plugins."io.containerd.grpc.v1.cri".containerd]
  default_runtime_name = "runc"
  discard_unpacked_layers = true
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc]
  runtime_type = "io.containerd.runc.v2"

[plugins."io.containerd.cri.v1.images".registry]
  config_path = "/etc/containerd/certs.d"
`

const expectedOutputA = `version = 2
required_plugins = ["io.containerd.grpc.v1.cri"]
# Kubernetes doesn't use containerd restart manager.
disabled_plugins = ["io.containerd.internal.v1.restart"]
oom_score = -999

[debug]
  level = "info"

[grpc]
  gid = 412

[plugins."io.containerd.grpc.v1.cri"]
  enable_cdi = true
  stream_server_address = "127.0.0.1"
  max_container_log_line_size = 262144
  sandbox_image = "gke.gcr.io/pause:3.8@sha256:880e63f94b145e46f1b1082bb71b85e21f16b99b180b9996407d61240ceb9830"
  image_pull_progress_timeout = "5m"
[plugins."io.containerd.grpc.v1.cri".cni]
  bin_dir = "/home/kubernetes/bin"
  conf_dir = "/etc/cni/net.d"
  conf_template = ""
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc.options]
  SystemdCgroup = true
# [plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]
#   endpoint = ["https://mirror.gcr.io","https://registry-1.docker.io"]
[metrics]
 address = "127.0.0.1:1338"
[plugins."io.containerd.grpc.v1.cri".containerd]
  default_runtime_name = "runc"
  discard_unpacked_layers = true
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc]
  runtime_type = "io.containerd.runc.v2"

[plugins."io.containerd.grpc.v1.cri".registry]
  config_path = "/etc/containerd/certs.d"
`

const expectedDiffA = `@@ -23,6 +23,6 @@
 [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc.options]
   SystemdCgroup = true
-[plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]
-  endpoint = ["https://mirror.gcr.io","https://registry-1.docker.io"]
+# [plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]
+#   endpoint = ["https://mirror.gcr.io","https://registry-1.docker.io"]
 [metrics]
  address = "127.0.0.1:1338"
@@ -33,4 +33,4 @@
   runtime_type = "io.containerd.runc.v2"
 
-[plugins."io.containerd.cri.v1.images".registry]
+[plugins."io.containerd.grpc.v1.cri".registry]
   config_path = "/etc/containerd/certs.d"`

func TestDiff(t *testing.T) {
	var sb100kA, sb100kB strings.Builder
	for i := 1; i <= 100000; i++ {
		if i == 50000 {
			sb100kA.WriteString("line 50000 original\n")
			sb100kB.WriteString("line 50000 modified\n")
			continue
		}
		sb100kA.WriteString("line ")
		sb100kA.WriteString(strconv.Itoa(i))
		sb100kA.WriteByte('\n')
		sb100kB.WriteString("line ")
		sb100kB.WriteString(strconv.Itoa(i))
		sb100kB.WriteByte('\n')
	}
	a100k := sb100kA.String()
	b100k := sb100kB.String()

	const expectedDiff100k = `@@ -49998,5 +49998,5 @@
 line 49998
 line 49999
-line 50000 original
+line 50000 modified
 line 50001
 line 50002`

	var sb20kA, sb20kB strings.Builder
	for i := 1; i <= 20000; i++ {
		sb20kA.WriteString("a-")
		sb20kA.WriteString(strconv.Itoa(i))
		sb20kA.WriteByte('\n')
		sb20kB.WriteString("b-")
		sb20kB.WriteString(strconv.Itoa(i))
		sb20kB.WriteByte('\n')
	}
	a20k := sb20kA.String()
	b20k := sb20kB.String()

	const expectedDiff20k = `@@ -1,20000 +1,20000 @@ (diff too large: 20000 lines removed, 20000 lines added)
-a-1
-a-2
-a-3
...
-a-19998
-a-19999
-a-20000
+b-1
+b-2
+b-3
...
+b-19998
+b-19999
+b-20000`

	tests := []struct {
		name               string
		a, b               string
		want               string
		checkHunks         []string
		assertNoTableAlloc bool
	}{
		{
			name: "identical input",
			a:    fixtureA,
			b:    fixtureA,
			want: "",
		},
		{
			name: "fixture (a) before/after",
			a:    fixtureA,
			b:    expectedOutputA,
			want: expectedDiffA,
			checkHunks: []string{
				"# [plugins.\"io.containerd.grpc.v1.cri\".registry.mirrors.\"docker.io\"]",
				"-[plugins.\"io.containerd.cri.v1.images\".registry]",
				"+[plugins.\"io.containerd.grpc.v1.cri\".registry]",
			},
		},
		{
			name: "100k-line pair differing by one line in the middle",
			a:    a100k,
			b:    b100k,
			want: expectedDiff100k,
			checkHunks: []string{
				"-line 50000 original",
				"+line 50000 modified",
			},
		},
		{
			name: "two unrelated 20k-line inputs degraded summary",
			a:    a20k,
			b:    b20k,
			want: expectedDiff20k,
			checkHunks: []string{
				"(diff too large: 20000 lines removed, 20000 lines added)",
				"-a-1",
				"-a-20000",
				"+b-1",
				"+b-20000",
			},
			assertNoTableAlloc: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			got := Diff(tc.a, tc.b)
			if elapsed := time.Since(start); elapsed > time.Second {
				t.Errorf("Diff() took %v, want under 1 second", elapsed)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("Diff() mismatch (-want +got):\n%s", diff)
			}
			for _, hunk := range tc.checkHunks {
				if !strings.Contains(got, hunk) {
					t.Errorf("Diff() missing expected hunk line %q in output:\n%s", hunk, got)
				}
			}
			if tc.assertNoTableAlloc {
				allocs := testing.AllocsPerRun(5, func() {
					Diff(tc.a, tc.b)
				})
				// If the 20k x 20k LCS table were allocated, it would require 20,001+ slice allocations.
				if allocs > 100 {
					t.Errorf("Diff() allocated %v times, expected no table allocation (<100)", allocs)
				}
			}
		})
	}
}
