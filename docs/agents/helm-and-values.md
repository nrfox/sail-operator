---
scribe:
  scan: "0b046f8c25b023933f6f7045bbb1955f7ea70b49"
  freshness: 100
  human_input: 0
  completeness: 100
  inferred_sections:
    - id: key-entry-points
      heading: "## Key Entry Points"
    - id: patterns--conventions
      heading: "## Patterns & Conventions"
    - id: patterns--conventions/values-pipeline
      heading: "### Values Pipeline"
    - id: patterns--conventions/chart-loading
      heading: "### Chart Loading"
    - id: patterns--conventions/install-and-upgrade
      heading: "### Install and Upgrade"
    - id: patterns--conventions/post-rendering
      heading: "### Post-Rendering"
    - id: patterns--conventions/vendor-defaults
      heading: "### Vendor Defaults"
    - id: patterns--conventions/platform-and-fips
      heading: "### Platform and FIPS"
    - id: gotchas
      heading: "## Gotchas"
    - id: dependencies--context
      heading: "## Dependencies & Context"
    - id: links
      heading: "## Links"
  watch_paths:
    - "pkg/helm/"
    - "pkg/istiovalues/"
    - "chart/"
  stale_flags: []
---

# Helm and Values Processing

> Covers how the operator loads Helm charts, processes values through profiles/vendor defaults/overrides, and installs releases. For how controllers invoke these operations see [controllers-architecture.md](controllers-architecture.md). For version resolution and chart discovery see [version-management.md](version-management.md).

## Key Entry Points
- `pkg/helm/chartmanager.go`: `ChartManager` — manages Helm install/upgrade/uninstall operations via the Helm v4 Go SDK
- `pkg/helm/values.go`: `Values` type (`map[string]any`) with typed accessors (`GetBool`, `Set`, `SetIfAbsent`, `SetStringSlice`), plus `FromValues`/`ToValues` for converting between typed API structs and generic maps
- `pkg/helm/fsloader.go`: `LoadChart()` loads charts from `fs.FS` (embed.FS or os.DirFS); `RenderChart()` does pure template rendering without cluster access
- `pkg/helm/postrenderer.go`: `HelmPostRenderer` — adds owner references, managed-by labels, and strips webhook failurePolicy on upgrades
- `pkg/istiovalues/profiles.go`: `ApplyProfilesAndPlatform()` — loads profile YAML from `<version>/profiles/` and merges with user values
- `pkg/istiovalues/vendor_defaults.go`: `ApplyIstioVendorDefaults()` / `ApplyIstioCNIVendorDefaults()` — applies version-specific and resource-type-specific vendor defaults from embedded `vendor_defaults.yaml`
- `pkg/istiovalues/overrides.go`: `ApplyOverrides()` — sets revision name, namespace, and clears deprecated `defaultRevision` field
- `pkg/istiovalues/digests.go`: `ApplyDigests()` — applies image digests from operator config unless user specified hub/tag
- `pkg/istiovalues/fips.go`: `ApplyFipsValues()` / `ApplyZTunnelFipsValues()` — sets FIPS compliance env vars when system FIPS mode is detected
- `pkg/istiovalues/tls.go`: `ApplyTLSConfig()` — propagates operator TLS cipher suites and min version to mesh config and istiod args
- `chart/` — the Helm chart for deploying the operator itself (not the Istio charts)

## Patterns & Conventions

### Values Pipeline
Values are assembled through a multi-stage pipeline. The exact order varies by component:

**Istiod (via `pkg/revision.ComputeValues`):**
1. Start with user-provided `spec.values` from the Istio/IstioRevision CR
2. Apply image digests from operator config (`ApplyDigests`) — skipped if user set `global.hub` or `global.tag`
3. Apply vendor defaults (`ApplyIstioVendorDefaults`) — version-specific and resource-type-specific
4. Apply FIPS values (`ApplyFipsValues`) — sets `COMPLIANCE_POLICY=fips-140-2` if `/proc/sys/crypto/fips_enabled` is `1`
5. Apply TLS config (`ApplyTLSConfig`) — propagates cipher suites and min TLS version
6. Apply overrides (`ApplyOverrides`) — sets revision name, namespace, clears `defaultRevision`
7. Apply profiles and platform (`ApplyProfilesAndPlatform`) — merges profile defaults with user values, sets `global.platform`

**CNI (via `pkg/reconcile.CNIReconciler.ComputeValues`):**
1. Apply image digests (`ApplyCNIImageDigests`)
2. Apply vendor defaults (`ApplyIstioCNIVendorDefaults`)
3. Apply profiles and platform (`ApplyProfilesAndPlatform`)

**ZTunnel (via `pkg/reconcile.ZTunnelReconciler.ComputeValues`):**
1. Apply image digests (`ApplyZTunnelImageDigests`)
2. Apply FIPS values (`ApplyZTunnelFipsValues`) — sets `TLS12_ENABLED=true`
3. Apply profiles and platform with optional base values from a referenced IstioRevision
4. Extra `ApplyUserValues` step for `values.ztunnel` descoping (see Gotchas)

### Chart Loading
Charts are loaded from an `fs.FS` (typically the embedded `resources/` directory) using `helm.LoadChart()`. The function walks the directory at `<version>/charts/<chartName>`, reads all files, strips the chart path prefix to get relative paths, and passes them to `chartv2loader.LoadFiles()`.

`RenderChart()` and `RenderLoadedChart()` provide pure template rendering without cluster access, used for offline chart inspection.

### Install and Upgrade
`ChartManager.UpgradeOrInstallChart()` handles the full lifecycle:
1. Load chart from `fs.FS`
2. Check for existing release via `action.NewGet`
3. If release is in a pending state (from a crash/timeout), unlock it by setting status to Failed
4. Handle failed releases: rollback if version > 1, uninstall if version <= 1 or status is Uninstalling
5. Upgrade existing deployed releases or install new ones

Key settings on install/upgrade actions:
- `SkipCRDs: true` — CRDs are managed separately
- `DisableOpenAPIValidation: true` — avoids validation issues with Istio's complex schemas
- `MaxHistory: 1` — keeps only the latest release revision
- `WaitStrategy: kube.HookOnlyStrategy` — does not wait for all resources to be ready
- `ServerSideApply: "false"` / `false` — uses client-side apply

### Post-Rendering
Every install/upgrade uses `HelmPostRenderer` which modifies each rendered manifest:
- Adds the owner reference (CR that triggered the install) to namespaced resources in the same namespace; for cross-namespace resources, uses `operator-sdk/primary-resource` annotations instead
- Adds `app.kubernetes.io/managed-by: sail-operator` label (configurable via `WithManagedByValue`)
- On upgrades, strips `failurePolicy` from `ValidatingWebhookConfiguration` webhooks to preserve the in-cluster value set by istiod

### Vendor Defaults
Vendor defaults are embedded from `pkg/istiovalues/vendor_defaults.yaml` at compile time. The YAML structure is:

```yaml
<version>:
  istio:
    <helm values>
  istiocni:
    <helm values>
```

Defaults are merged as a base layer — user values take precedence via `mergeOverwrite`. The merge is recursive for maps but replaces non-map values entirely. If no vendor defaults exist for a version or resource type, user values pass through unchanged.

### Platform and FIPS
Platform detection sets `global.platform` unless the platform is vanilla Kubernetes or undefined. FIPS mode is auto-detected from `/proc/sys/crypto/fips_enabled` at operator startup. When enabled:
- Istiod: `COMPLIANCE_POLICY=fips-140-2` in pilot env
- ZTunnel: `TLS12_ENABLED=true` in ztunnel env

Both are set only if not already present in user values.

## Gotchas
- **Helm v4**: The operator uses `helm.sh/helm/v4`, not v3. Chart types, loader APIs, and action configuration differ from Helm v3 documentation.
- **Pending release recovery**: If the operator crashes mid-install, the next reconciliation finds the release in a pending state. The ChartManager unlocks it by force-setting the status to Failed, then either rolls back (if version > 1) or uninstalls (if version <= 1).
- **ZTunnel double-merge**: The ZTunnel reconciler applies `ApplyUserValues` twice — once for the full values and once specifically for `values.ztunnel`. This is because the ztunnel Helm chart lacks the `zzy_descope_legacy.yaml` template that the CNI chart uses to automatically extract nested values.
- **Image digest precedence**: Setting `global.hub` or `global.tag` disables all automatic image digest injection for that component. This is intentional — if the user specifies a registry, the operator assumes they want full control over images.
- **Profile path traversal protection**: `getValuesFromProfiles` validates that the profile file path stays within the profiles directory to prevent path traversal attacks.
- **The `defaultRevision` field is always cleared**: `ApplyOverrides` sets `values.DefaultRevision = nil` regardless of user input. The validating webhook is still created when the revision name is `default`, but the deprecated field is intentionally suppressed.
- **TLS min version flag**: The `--tls-min-version` istiod container arg is only set for Istio >= 1.29. Older versions silently skip this flag.

## Dependencies & Context
The Helm integration uses the Helm v4 Go SDK directly — there is no shell-out to the `helm` CLI. The `ChartManager` holds a `RESTClientGetter` built from the operator's in-cluster `rest.Config`, with a memory-cached discovery client and deferred REST mapper.

Charts are not downloaded at runtime. They are embedded in the operator binary via `resources/` (an `embed.FS` or equivalent `fs.FS`). Each supported version has its charts at `resources/<version>/charts/<chartName>/`. Profile YAML files live at `resources/<version>/profiles/<profileName>.yaml`.

The `chart/` directory at the repo root is the operator's own Helm chart (for deploying the operator itself to a cluster), not the Istio component charts.

The merge strategy (`mergeOverwrite` in `profiles.go`) performs recursive map merging: if both base and override have a map at the same key, it recurses; otherwise the override replaces the base value entirely. This matches Helm's standard merge behavior.

## Links
- [controllers-architecture.md](controllers-architecture.md) — controllers that invoke chart installation
- [version-management.md](version-management.md) — version resolution that determines chart paths
- [api-types-and-crds.md](api-types-and-crds.md) — typed value structs (`v1.Values`, `v1.CNIValues`, `v1.ZTunnelValues`)
- `pkg/istiovalues/vendor_defaults.yaml` — embedded vendor default values
- `pkg/revision/` — `ComputeValues()` orchestrates the full istiod values pipeline
- `resources/` — embedded Istio charts and profiles per version
