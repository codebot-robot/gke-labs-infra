# Architecture Decision: Kubernetes Cluster Access for `ap`

## Status
Decided. Prototype implemented on deploy path (`ap/pkg/k8s`).

## Decision
1. **Move production cluster access in `ap` to `client-go`**: Use the dynamic client, discovery-backed `RESTMapper`, and Server-Side Apply (SSA) for production code paths (`pkg/k8s`, `pkg/sandbox`).
2. **Keep test harness on `kubectl`**: `pkg/e2e` may remain on `kubectl` to test real CLI user workflows, cluster creation/deletion, and context switching.
3. **Retain `ap` in the root `go.mod`**: Do not split `ap` into a separate Go module at this time. Go's linker tree-shakes unused dependencies, ensuring non-Kubernetes binaries in the root module (`kubelint`, `github-admin`, `tools/git-search`) incur zero binary size or build time overhead, while preserving the standard `go run ./ap` invocation across CI presubmits (`version: "!self"`).

---

## Context & Background
`ap` has historically interacted with Kubernetes clusters by shelling out to the `kubectl` CLI across multiple production sites:
- **`pkg/k8s/deploy.go`**: `kubectl apply -f -`, `kubectl wait --for=condition=Established crd/...`
- **`pkg/k8s/namespace.go` (#468, #474)**: `kubectl api-resources` (table parsing for scope discovery), `kubectl get namespace`
- **`pkg/k8s/portforward.go` (#415, #444)**: `kubectl port-forward` subprocess management
- **`pkg/sandbox/sandbox.go`**: `kubectl get pod`, `kubectl run`, `kubectl wait`, `kubectl port-forward`
- **`pkg/e2e/harness.go`**: kind cluster creation, namespace setup/teardown, `kubectl apply`, wait, logs

In PR #474 (namespace retargeting), scope discovery was implemented via `kubectl api-resources` table parsing to avoid introducing `k8s.io/client-go` and `k8s.io/apimachinery` into the root `go.mod`. However, issue #476 requested a definitive architectural decision between maintaining `kubectl` process execution versus transitioning to `client-go` in-process access, backed by a prototype on the deploy path and quantitative dependency impact measurements.

---

## Trade-off Analysis

| Concern | Shelling out to `kubectl` | `client-go` (dynamic + discovery + SSA) |
| :--- | :--- | :--- |
| **Parsing & Schema Discovery** | Fragile. Human-oriented ASCII table (`api-resources`) can shift across Kubernetes/kubectl releases, column widths, or flags. | Robust. Directly queries `/api` and `/apis` discovery endpoints into typed `metav1.APIResourceList` and `meta.RESTMapper`. |
| **Apply Semantics** | Relies on client-side three-way merge or kubectl SSA flags via stdin pipe. Subprocess per manifest file. | True Server-Side Apply (`types.ApplyPatchType`, `fieldManager: "ap"`, `Force: true`). In-process, granular per-object control, and real server-side dry-run (`metav1.DryRunAll`). |
| **CRD Establishment** | Spawns `kubectl wait --for=condition=Established crd/... --timeout=60s`. Fragile string exit codes. | Event-driven condition inspection (`wait.PollUntilContextTimeout` or `Watch`). Typed condition check (`Established == True`), immediate return upon readiness, and context cancellation. |
| **Error Handling** | Unstructured stderr strings. Distinguishing `NotFound`, `AlreadyExists`, or `Conflict` requires regex/string scraping. | Typed errors via `k8s.io/apimachinery/pkg/api/errors` (`apierrors.IsNotFound(err)`, `apierrors.IsConflict(err)`). |
| **Port Forwarding** | Subprocess management. Requires parsing stdout for port readiness, PID tracking, kill traps, and macOS Docker bridge workarounds. | In-process forwarding via `k8s.io/client-go/tools/portforward` with native Go `Ready` and `Stop` channels. |
| **Process Overhead** | Heavy: 1+ child process spawn per manifest document and per CRD wait. | Zero child process spawns; HTTP/2 connection pooling via REST client. |
| **Module Dependencies** | Zero new dependencies in `go.mod`. | Adds `k8s.io/client-go`, `k8s.io/apimachinery`, and `k8s.io/api`. |

---

## Dependency & Module Impact Measurements

Empirical measurements taken on Linux x86_64 with Go 1.27:

### 1. Module Count
- **Root `go.mod` (Baseline)**: 64 modules (direct + indirect)
- **Root `go.mod` (With client-go + apimachinery + api)**: 118 modules (+54 transitive modules)
*Note: Almost all transitive dependencies (e.g. `google/gnostic-models`, `fxamacker/cbor`, `sigs.k8s.io/structured-merge-diff`) were already downloaded and cached in this repository by `autodeploy/go.mod`.*

### 2. Binary Sizes
- **`ap` CLI**:
  - Baseline: **26,121,076 bytes** (~25MB)
  - With client-go deploy path: **61,341,899 bytes** (~59MB)
  - Net increase: **+34MB**, standard for Go binaries statically embedding client-go and HTTP/2 transport.
- **Sibling tools sharing the root `go.mod`**:
  - `kubelint`: Baseline 4,314,203 bytes -> Post-change **4,313,883 bytes** (0% change)
  - `github-admin`: Baseline 12,150,298 bytes -> Post-change **12,150,514 bytes** (0% change)
  - `tools/git-search`: Baseline 4,292,885 bytes -> Post-change **4,292,765 bytes** (0% change)

Because Go compiles and links only packages transitively imported by the target binary, non-Kubernetes tools sharing the root `go.mod` incur **zero** binary size or runtime overhead.

### 3. Build Times
- **`ap` clean build**: ~8.1s (one-time cold compile)
- **`ap` incremental rebuild**:
  - Baseline: **~0.15s**
  - With client-go: **~0.20s** (+0.05s)
- **Sibling tools incremental rebuild**: Identical (~0.10s)

### 4. Root `go.mod` vs Separate `ap/go.mod`
We evaluated splitting `ap` into a standalone nested Go module (`ap/go.mod`). We determined that keeping `ap` in the root module is currently the superior architecture:
1. **Developer & CI Experience**: Presubmit workflows across the repo execute `go run ./ap ...` from the repository root (configured via `version: "!self"` in `.ap/ap.yaml`). A nested module would break `go run ./ap` from the root directory unless `go.work` workspaces were adopted.
2. **Isolation Guarantee**: As demonstrated above, sibling binaries (`kubelint`, `github-admin`) suffer no binary bloat or performance degradation from dependencies in the root module that they do not import.

---

## Prototype Implementation: Deploy Path (`pkg/k8s`)

The prototype validates the `client-go` approach on the deploy path without modifying upstream manifest files:

1. **`ap/pkg/k8s/client.go`**:
   - `NewRESTConfig()`: Automatically loads configuration using `clientcmd.NewDefaultClientConfigLoadingRules()` (honoring `KUBECONFIG`, `~/.kube/config`, active contexts, and authentication plugins), falling back to in-cluster service account configuration when executing in a pod.
   - `GetDefaultClientAndMapper()`: Lazily initializes and caches dynamic client (`dynamic.NewForConfig`) and in-memory cached discovery REST mapper (`memory.NewMemCacheClient`, `restmapper.NewDeferredDiscoveryRESTMapper`).
   - `CheckNamespaceExists()`: Directly queries `namespaces` via dynamic client `Get()`, using `apierrors.IsNotFound(err)` to return clean, typed error messages.

2. **`ap/pkg/k8s/scope.go`**:
   - `ScopeResolver` interface: `IsNamespaced(gvk GroupVersionKind) (bool, error)`.
   - `DiscoveryScopeResolver`: Resolves scope dynamically through the Kubernetes API server discovery REST mapping without human-readable table parsing.
   - `MapScopeResolver`: Map-based implementation enabling 100% offline, deterministic unit testing.
   - `ValidateNamespaceName`: Validates namespace names against DNS-1123 label rules with an allocation-free loop.

3. **`ap/pkg/k8s/deploy.go`**:
   - `KubectlApplyTask`: Retains its task interface and task naming convention (`kubectl-apply-<manifest>`), but executes Server-Side Apply in-process. Supports injecting `DynamicClient` and `RESTMapper` for unit testing.
   - `ApplyManifest()`: Decodes multi-document YAML manifests into `unstructured.Unstructured` streams, queries the `RESTMapper` for GVR and scope, and executes `dr.Patch` with `types.ApplyPatchType`, `fieldManager: "ap"`, and `Force: true`.
   - `WaitForCRDEstablished()`: When applying a `CustomResourceDefinition`, polls the CRD status condition `Established == True` using `wait.PollUntilContextTimeout`, returning immediately when established and invalidating the REST mapper cache so subsequent manifest resources can resolve immediately.

4. **Tests (`deploy_ssa_test.go`, `scope_test.go`)**:
   - Single- and multi-document Server-Side Apply verified using `client-go/dynamic/fake`.
   - FieldManager `"ap"`, patch type `ApplyPatchType`, and cluster- vs namespace-scoped routing verified.
   - CRD establishment polling verified.
   - End-to-end `task.Run()` verified with image placeholder substitution.

---

## Rollout Plan for Remaining Sites

Following the "one PR per site" guideline:
1. **PR 1 (This PR)**: Record architectural decision note and deploy-path prototype (`pkg/k8s/deploy.go`, `pkg/k8s/client.go`, `pkg/k8s/scope.go`).
2. **PR 2**: Rebase PR #474 (`--namespace` flag) onto main, updating it to use `DiscoveryScopeResolver` and `CheckNamespaceExists` from `pkg/k8s`.
3. **PR 3**: Migrate port-forwarding (`pkg/k8s/portforward.go`) to `k8s.io/client-go/tools/portforward`.
4. **PR 4**: Migrate sandbox pod management (`pkg/sandbox/sandbox.go`) to client-go typed pod client and in-process port-forwarding.
