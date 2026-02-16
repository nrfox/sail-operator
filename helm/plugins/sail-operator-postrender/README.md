# sail-operator-postrender

A Helm v4 WASM postrender plugin that replaces Go-templated YAML with programmatic
Kubernetes resource construction using typed Go structs (`k8s.io/api`).

## How It Works

Instead of rendering Kubernetes resources from Go-templated YAML files, this plugin
constructs them programmatically:

1. A single Helm template (`chart/templates/values-carrier.yaml`) renders all chart
   values and release metadata into a ConfigMap.
2. Helm v4 invokes this plugin as a postrender step via the Extism WASM runtime.
3. The plugin parses the carrier ConfigMap, extracts the values, builds all 10
   Kubernetes resources using typed Go structs, and returns the resulting YAML
   (discarding the carrier).

CRDs in `chart/crds/` are handled by Helm directly and are not part of this plugin.

### Data Flow

```
values.yaml
    |
    v
Helm template engine
    |
    v
values-carrier.yaml (ConfigMap with serialized values + release metadata)
    |
    v
plugin.wasm (postrender)
    |
    v
10 Kubernetes resource manifests (YAML)
```

## Resources Generated

The plugin produces these 10 resources:

| # | Kind               | Name                              | Scope     |
|---|--------------------|-----------------------------------|-----------|
| 1 | ServiceAccount     | `<serviceAccountName>`            | Namespaced|
| 2 | Deployment         | `<deployment.name>`               | Namespaced|
| 3 | Service            | `<deployment.name>-metrics-service`| Namespaced|
| 4 | ClusterRole        | `<name>-role`                     | Cluster   |
| 5 | ClusterRoleBinding | `<name>-rolebinding`              | Cluster   |
| 6 | Role               | `leader-election-role`            | Namespaced|
| 7 | RoleBinding        | `leader-election-rolebinding`     | Namespaced|
| 8 | ClusterRole        | `<name>-proxy-role`               | Cluster   |
| 9 | ClusterRoleBinding | `<name>-proxy-rolebinding`        | Cluster   |
|10 | ClusterRole        | `metrics-reader`                  | Cluster   |

OLM resources (ClusterServiceVersion, samples, scorecard) are excluded.

## Source Files

| File                 | Purpose                                              |
|----------------------|------------------------------------------------------|
| `main.go`            | WASM entry point; exports `helm_plugin_main` via Extism PDK |
| `main_native.go`     | Empty `main()` stub for native builds/testing        |
| `values.go`          | Go structs matching `chart/values.yaml`              |
| `labels.go`          | Shared label/annotation helpers and `ptr[T]` generic |
| `resources.go`       | Orchestrator: carrier parsing and resource assembly  |
| `deployment.go`      | `apps/v1.Deployment` construction                    |
| `service.go`         | `v1.Service` construction                            |
| `serviceaccount.go`  | `v1.ServiceAccount` construction                     |
| `rbac.go`            | All 7 RBAC resources (3 ClusterRoles, 2 ClusterRoleBindings, 1 Role, 1 RoleBinding) |
| `resources_test.go`  | Native Go tests for resource construction            |
| `plugin.yaml`        | Helm v4 plugin descriptor                            |

## Prerequisites

- Go 1.24+ (for `//go:wasmexport` support)

## Building

```bash
make build
```

This compiles the plugin to `plugin.wasm` using `GOOS=wasip1 GOARCH=wasm`.

## Testing

```bash
make test
```

Tests run natively (not in WASM) by using build tags:
- `main.go` is guarded with `//go:build wasip1` so the Extism PDK import is excluded
- `main_native.go` provides an empty `main()` for native compilation
- `resources_test.go` directly calls `ParseValuesFromManifests` and `BuildAllResources`

Test cases cover:
- Default values (all 10 resources present with correct names/namespaces)
- Custom `nodeSelector`
- Extra operator args (`operator.extraArgs`)
- Image pull secrets
- Tolerations

## Linting

```bash
make vet
```

Runs `go vet` targeting the WASM platform.

## Carrier ConfigMap

The plugin expects a ConfigMap named `sail-operator-values-carrier` in the input
manifests. This is produced by `chart/templates/values-carrier.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: sail-operator-values-carrier
  namespace: {{ .Release.Namespace }}
  annotations:
    sail-operator/release-name: {{ .Release.Name }}
    sail-operator/release-namespace: {{ .Release.Namespace }}
data:
  values.yaml: |
    {{ .Values | toYaml | indent 4 }}
```

The plugin reads the values from `.data["values.yaml"]` and the release name/namespace
from the annotations. The carrier ConfigMap is not included in the plugin's output.

## Plugin Interface

The plugin communicates with the Helm v4 Extism runtime via JSON:

**Input:**
```json
{"manifests": "<base64-encoded rendered YAML>"}
```

**Output:**
```json
{"manifests": "<base64-encoded output YAML>"}
```

The exported WASM function is `helm_plugin_main`, which returns `0` on success
or `1` on error (with the error message set via `pdk.SetError`).
