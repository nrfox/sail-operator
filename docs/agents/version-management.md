---
scribe:
  scan: "0b046f8c25b023933f6f7045bbb1955f7ea70b49"
  freshness: 100
  human_input: 20
  completeness: 100
  inferred_sections:
    - id: key-entry-points
      heading: "## Key Entry Points"
    - id: patterns--conventions
      heading: "## Patterns & Conventions"
    - id: patterns--conventions/versions-yaml-structure
      heading: "### versions.yaml Structure"
    - id: patterns--conventions/version-resolution
      heading: "### Version Resolution"
    - id: patterns--conventions/embedded-resources
      heading: "### Embedded Resources"
    - id: patterns--conventions/operator-build-info
      heading: "### Operator Build Info"
    - id: gotchas
      heading: "## Gotchas"
    - id: dependencies--context/versioning-policy
      heading: "### Versioning policy"
    - id: links
      heading: "## Links"
  watch_paths:
    - "pkg/istioversion/"
    - "pkg/version/"
    - "resources/"
  stale_flags: []
---

# Version Management

> Covers how Istio versions are defined, resolved, and embedded in the operator binary. For how versions are used during reconciliation see [controllers-architecture.md](controllers-architecture.md). For chart loading and values processing see [helm-and-values.md](helm-and-values.md).

## Key Entry Points
- `pkg/istioversion/versions.yaml`: Defines all supported Istio versions — the first non-alias entry is the default
- `pkg/istioversion/version.go`: Parses `versions.yaml` at init time; exports `List`, `Map`, `Default`, `Base`, `New`, `EOL`; provides `Resolve()` and `IsEOLVersion()`
- `resources/resources.go`: Embeds all version directories (`resources/v*/`) as `resources.FS` — contains Helm charts and profiles per version
- `pkg/version/version.go`: Operator build version info (`BuildInfo`) — populated via ldflags
- `pkg/version/semverutils.go`: `Constraint()` helper for creating semver constraints (panics on invalid input)
- `hack/update-istio.sh`: Script for adding new Istio versions to `versions.yaml` and downloading charts
- `hack/download-charts.sh`: Downloads Helm chart tarballs and extracts them into `resources/<version>/charts/`

## Patterns & Conventions

### versions.yaml Structure
Each entry in `versions.yaml` is one of three types:

**Full version entry** — installable version with charts:
```yaml
- name: v1.30.0
  version: 1.30.0
  repo: https://github.com/istio/istio
  commit: 1.30.0
  charts:
    - https://istio-release.storage.googleapis.com/charts/base-1.30.0.tgz
    - https://istio-release.storage.googleapis.com/charts/istiod-1.30.0.tgz
    - https://istio-release.storage.googleapis.com/charts/gateway-1.30.0.tgz
    - https://istio-release.storage.googleapis.com/charts/cni-1.30.0.tgz
    - https://istio-release.storage.googleapis.com/charts/ztunnel-1.30.0.tgz
```

**Alias entry** — points to another version:
```yaml
- name: v1.30-latest
  ref: v1.30.0
```

**EOL entry** — still valid in `spec.version` but not installable:
```yaml
- name: v1.27.9
  eol: true
```

The first non-alias, non-EOL entry is the default version. Ordering within `versions.yaml` matters: `List[0]` becomes `New` (latest) and `Default`; `List[1]` becomes `Base` (previous minor).

Alias entries cannot have any fields other than `name` and `ref` — the parser panics if `version`, `repo`, `commit`, `branch`, or `charts` are set on an alias.

### Version Resolution
`Resolve(version)` looks up a version name in `Map` and returns the resolved name. For aliases, the map entry points to the referenced version's `VersionInfo`. For example, resolving `v1.30-latest` returns `v1.30.0`.

The `Map` includes both direct version entries and alias entries, so aliases are resolved transparently. EOL versions are excluded from `Map` — `Resolve()` returns an error for them, and controllers check `IsEOLVersion()` to produce a specific validation error message.

Helper functions for test infrastructure:
- `GetLatestPatchVersions()` — returns the highest patch version for each major.minor, sorted descending
- `GetTwoConsecutiveMinorVersions(minVersion)` — returns a base/new pair of consecutive minor versions at or above a minimum, for upgrade testing

### Embedded Resources
Charts and profiles are embedded in the operator binary via `resources.FS` (an `embed.FS` using `//go:embed all:v*`). Each version directory has:

```
resources/v1.30.0/
├── charts/
│   ├── base/
│   ├── cni/
│   ├── gateway/
│   ├── istiod/
│   ├── revisiontags/
│   └── ztunnel/
├── profiles/
│   ├── ambient.yaml
│   ├── default.yaml
│   ├── demo.yaml
│   ├── empty.yaml
│   ├── openshift.yaml
│   ├── openshift-ambient.yaml
│   ├── preview.yaml
│   ├── remote.yaml
│   └── stable.yaml
├── commit
└── *.etag
```

The `commit` file records the source Git commit. The `.etag` files are used by the download scripts to avoid re-downloading unchanged charts.

`resources.SubFS()` and `resources.MustSubFS()` create sub-filesystems for stripping path prefixes when consumers embed charts under a different directory structure.

### Operator Build Info
`pkg/version/version.go` exports `version.Info` (`BuildInfo` struct) containing the operator's own version, Git revision, build status, Git tag, Go version, architecture, and controller-runtime version. The first four fields are set via ldflags at build time; the rest are detected at runtime.

`pkg/version/semverutils.go` provides `Constraint()` — a panic-on-error wrapper for `semver.NewConstraint()` used to define version constraints in declarative code (e.g., feature gates tied to specific Istio versions).

## Gotchas
- **Init-time panic**: `pkg/istioversion/version.go` panics if `versions.yaml` is missing, malformed, or contains an alias referencing a nonexistent version. This is intentional — version configuration is a hard requirement.
- **VERSIONS_YAML_FILE env var**: During tests, the versions filename can be overridden with the `VERSIONS_YAML_FILE` environment variable because ldflags don't work in test binaries (Go issue #64246).
- **ldflags for versionsFilename**: The `versionsFilename` variable is set via ldflags during the build. If unset, it defaults to `versions.yaml` (via the env var fallback).
- **EOL versions remain in CRD enum**: EOL versions stay as valid values in the `spec.version` kubebuilder enum to avoid breaking API guarantees. They are rejected at the controller level, not at the CRD validation level.
- **Chart URLs in versions.yaml are reference only**: The chart URLs in `versions.yaml` are used by `hack/download-charts.sh` to fetch and extract charts into `resources/`. They are not used at runtime — charts are loaded from the embedded filesystem.
- **Default version is the first entry**: The default and `New` version is always `List[0]` (the first non-alias, non-EOL entry). Reordering entries in `versions.yaml` changes the default.
- **Binary size**: Importing `resources` embeds all chart files (~10MB+). The `resources.go` doc comment warns about this.

## Dependencies & Context
Version management follows a pre-baked model: all supported Istio charts and profiles are embedded in the operator binary at build time. There is no runtime chart downloading. Embedding charts into the binary allows library consumers to import the `resources` package and use the charts directly, in addition to guaranteeing air-gapped environments work without network access.

The n-2 versioning policy means the operator supports the current and two previous minor Istio versions. When a new minor version is added, the oldest minor is marked `eol: true` in `versions.yaml`. The `hack/update-istio.sh` script automates adding new versions: it inserts the new entry, downloads charts, extracts profiles, and updates the `resources/` directory.

The `Masterminds/semver/v3` library handles all semantic version parsing and comparison throughout the codebase.

## Links
- [controllers-architecture.md](controllers-architecture.md) — controllers call `istioversion.Resolve()` during reconciliation
- [helm-and-values.md](helm-and-values.md) — chart loading uses `resources.FS` and version paths from `istioversion`
- [api-types-and-crds.md](api-types-and-crds.md) — CRD `spec.version` enum is generated from `versions.yaml`
- `hack/update-istio.sh` — script for adding new Istio versions
- `hack/download-charts.sh` — downloads and extracts Helm charts into `resources/`
- `hack/update-version-list.sh` — updates the version enum in CRD types
