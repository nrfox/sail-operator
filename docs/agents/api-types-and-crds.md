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
    - id: patterns--conventions/resource-hierarchy
      heading: "### Resource Hierarchy"
    - id: patterns--conventions/status-conditions
      heading: "### Status Conditions"
    - id: patterns--conventions/kubebuilder-validation
      heading: "### Kubebuilder Validation"
    - id: patterns--conventions/values-types
      heading: "### Values Types"
    - id: gotchas
      heading: "## Gotchas"
    - id: dependencies--context
      heading: "## Dependencies & Context"
    - id: links
      heading: "## Links"
  watch_paths:
    - "api/v1/"
    - "api/v1alpha1/"
    - "pkg/validation/"
  stale_flags: []
---

# API Types and CRDs

> Covers the five custom resources (Istio, IstioRevision, IstioRevisionTag, IstioCNI, ZTunnel), their Go types, validation rules, status conditions, and the shared condition utilities. For how controllers reconcile these resources see [controllers-architecture.md](controllers-architecture.md).

## Key Entry Points
- `api/v1/istio_types.go`: `Istio` CRD — the primary user-facing resource for deploying a control plane
- `api/v1/istiorevision_types.go`: `IstioRevision` CRD — represents a single control plane deployment (managed by the Istio controller, not created directly by users)
- `api/v1/istiorevisiontags_types.go`: `IstioRevisionTag` CRD — creates stable revision aliases for canary deployments; also defines the shared `TargetReference` type
- `api/v1/istiocni_types.go`: `IstioCNI` CRD — deploys the CNI plugin (required for OpenShift and Ambient mesh)
- `api/v1/ztunnel_types.go`: `ZTunnel` CRD — deploys the ztunnel DaemonSet for Ambient mesh
- `api/v1/condition.go`: Shared `StatusCondition` type with `SetCondition()`/`GetCondition()` utilities
- `api/v1/values_types.gen.go`: Auto-generated Go types from Istio's Helm values schema (do not edit)
- `api/v1/values_types_extra.go`: Hand-written value type extensions
- `api/v1alpha1/ztunnel_types.go`: Legacy v1alpha1 ZTunnel type (backwards compatibility)
- `pkg/validation/validation.go`: `ValidateTargetNamespace()` and `ResourceTakesPrecedence()` utilities

## Patterns & Conventions

### Resource Hierarchy
All five CRDs are cluster-scoped (`+kubebuilder:resource:scope=Cluster`) and categorized under `istio-io`.

**Ownership chain:**
- `Istio` creates and owns `IstioRevision` resources (controller reference with `BlockOwnerDeletion`)
- `IstioRevisionTag` references an `Istio` or `IstioRevision` via `spec.targetRef` (not an owner reference — a lookup reference)
- `ZTunnel` optionally references an `Istio` or `IstioRevision` via `spec.targetRef` to inherit values
- `IstioCNI` and `ZTunnel` are independent resources with no ownership relationship to `Istio`

**Naming constraints:**
- `IstioCNI` and `ZTunnel` must be named `default` (enforced by CRD-level XValidation: `self.metadata.name == 'default'`)
- `IstioRevision` validates that `spec.values.revision` matches `metadata.name` (or is empty when name is `default`)

### Status Conditions
All resources use `StatusCondition` from `api/v1/condition.go` (not the standard `metav1.Condition`). Key differences:
- Uses custom `ConditionType` and `ConditionReason` string types
- `SetCondition()` preserves `LastTransitionTime` when status hasn't changed, sets it to current time (truncated to seconds) on transition
- `GetCondition()` returns a default condition with `Status: Unknown` when the type isn't found

**Condition types by resource:**

| Resource | Reconciled | Ready | InUse | DependenciesHealthy |
|----------|-----------|-------|-------|-------------------|
| Istio | Yes | Yes (from revision) | No | Yes (from revision) |
| IstioRevision | Yes | Yes (istiod/webhook) | Yes | Yes (CNI+ZTunnel) |
| IstioCNI | Yes | Yes (DaemonSet) | No | No |
| ZTunnel | Yes | Yes (DaemonSet) | No | No |
| IstioRevisionTag | Yes | No | Yes | No |

### Kubebuilder Validation
Key validation rules enforced at the CRD level:
- `spec.namespace` is immutable on Istio, IstioRevision, and IstioCNI (`rule: "self == oldSelf"`)
- `spec.version` is constrained to an enum of supported versions (including EOL versions, which are rejected at the controller level)
- `spec.values.global.istioNamespace` must match `spec.namespace` on Istio and IstioRevision (XValidation rule)
- Profile values are constrained to a fixed enum: `ambient`, `default`, `demo`, `empty`, `external`, `openshift`, `openshift-ambient`, `preview`, `remote`, `stable`

### Values Types
Each component has its own typed values struct:
- `*v1.Values` — used by Istio and IstioRevision (the full Istio Helm values)
- `*v1.CNIValues` — used by IstioCNI
- `*v1.ZTunnelValues` — used by ZTunnel

The main `Values` type and its nested types (`GlobalConfig`, `PilotConfig`, `ProxyConfig`, `MeshConfig`, etc.) are auto-generated in `values_types.gen.go` from Istio's Helm values JSON schema. Hand-written extensions live in `values_types_extra.go`.

Values are converted to/from `helm.Values` (`map[string]any`) via JSON marshaling in `helm.FromValues()` and `helm.ToValues()`.

## Gotchas
- **Cluster-scoped resources**: All CRDs are cluster-scoped, not namespaced. The `spec.namespace` field determines the target namespace for deployed components, but the CRs themselves live at the cluster level.
- **IstioRevision is operator-managed**: Users should create `Istio` resources and let the operator create `IstioRevision` resources. Direct IstioRevision creation is supported but not the intended workflow.
- **Version enum includes EOL versions**: The kubebuilder enum on `spec.version` includes end-of-life versions. These pass CRD validation but are rejected at the controller level with a `ValidationError` explaining the version is EOL.
- **Name conflict between IstioRevision and IstioRevisionTag**: Both types validate that no resource of the other type exists with the same name. Precedence is determined by `validation.ResourceTakesPrecedence()`: the older resource (by creation timestamp) wins; UIDs break ties.
- **ZTunnel has two API versions**: `v1` is the storage version (`+kubebuilder:storageversion`); `v1alpha1` exists for backwards compatibility. The ZTunnel controller watches both.
- **Custom StatusCondition type**: The project uses its own `StatusCondition` type rather than the standard `metav1.Condition`. The `Type` field is `ConditionType` (string alias), not a string. The `Reason` field is `ConditionReason`, not a string.
- **UpdateStrategy defaults**: `InPlace` is the default update strategy. The code has a TODO to change this to `RevisionBased` in the future. `InactiveRevisionDeletionGracePeriodSeconds` defaults to 30 with a minimum of 0.

## Dependencies & Context
The API types are built with kubebuilder markers and follow standard Kubernetes API conventions. Types are registered via `api/v1/groupversion_info.go` under `sailoperator.io/v1`.

The `TargetReference` type (defined in `istiorevisiontags_types.go`) is shared between `IstioRevisionTag` and `ZTunnel`. It allows referencing either an `Istio` (resolved to its active revision) or an `IstioRevision` directly.

The `pkg/validation/` package provides two utilities:
- `ValidateTargetNamespace()` checks that the target namespace exists and is not being deleted
- `ResourceTakesPrecedence()` determines which resource wins when an IstioRevision and IstioRevisionTag share the same name, using creation timestamp with UID as tie-breaker

CRD generation is done via `make gen`, which runs kubebuilder's controller-gen to produce CRD YAML from the Go type definitions and markers.

## Links
- [controllers-architecture.md](controllers-architecture.md) — how these types are reconciled
- [helm-and-values.md](helm-and-values.md) — values processing pipeline that operates on `v1.Values`, `v1.CNIValues`, `v1.ZTunnelValues`
- [version-management.md](version-management.md) — how `spec.version` enum values are maintained
- `api/v1/values_types.gen.go` — auto-generated values types (do not edit)
- `chart/crds/` — generated CRD YAML manifests
