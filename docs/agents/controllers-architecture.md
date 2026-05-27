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
    - id: patterns--conventions/standard-reconciler
      heading: "### Standard Reconciler"
    - id: patterns--conventions/status-determination
      heading: "### Status Determination"
    - id: patterns--conventions/error-types-and-retry
      heading: "### Error Types and Retry"
    - id: patterns--conventions/watch-setup-and-event-handling
      heading: "### Watch Setup and Event Handling"
    - id: patterns--conventions/predicates
      heading: "### Predicates"
    - id: patterns--conventions/shared-reconcilers
      heading: "### Shared Reconcilers"
    - id: gotchas
      heading: "## Gotchas"
    - id: dependencies--context/inter-controller-coordination
      heading: "### Inter-controller coordination"
    - id: links
      heading: "## Links"
  watch_paths:
    - "controllers/istio/"
    - "controllers/istiorevision/"
    - "controllers/istiocni/"
    - "controllers/ztunnel/"
    - "controllers/istiorevisiontag/"
    - "controllers/webhook/"
    - "pkg/reconciler/"
    - "pkg/reconcile/"
    - "pkg/predicate/"
    - "pkg/enqueuelogger/"
  stale_flags: []
---

# Controllers Architecture

> Covers the six Kubernetes controllers, their reconciliation logic, shared reconciler infrastructure, error handling, status management, and watch/predicate patterns. For API type definitions see [api-types-and-crds.md](api-types-and-crds.md). For Helm chart installation details see [helm-and-values.md](helm-and-values.md).

## Key Entry Points
- `controllers/istio/istio_controller.go`: Reconciles `Istio` CRs — creates/updates `IstioRevision` resources and prunes inactive revisions
- `controllers/istiorevision/istiorevision_controller.go`: Reconciles `IstioRevision` CRs — validates, installs istiod via Helm, tracks readiness and in-use status
- `controllers/istiocni/istiocni_controller.go`: Reconciles `IstioCNI` CRs — installs the CNI DaemonSet via Helm
- `controllers/ztunnel/ztunnel_controller.go`: Reconciles `ZTunnel` CRs — installs ztunnel DaemonSet, optionally inherits values from a referenced `IstioRevision`
- `controllers/istiorevisiontag/istiorevisiontag_controller.go`: Reconciles `IstioRevisionTag` CRs — installs the `revisiontags` Helm chart, manages base chart for `default` tags
- `controllers/webhook/webhook_controller.go`: Probes remote istiod readiness via MutatingWebhookConfiguration annotations
- `pkg/reconciler/reconciler.go`: Generic `StandardReconciler[T]` that wraps object fetch, finalizer management, and error classification
- `pkg/reconcile/`: Shared component reconcilers (`IstiodReconciler`, `CNIReconciler`, `ZTunnelReconciler`) used by both the operator controllers and a standalone install library

## Patterns & Conventions

### Standard Reconciler
Every controller delegates to `reconciler.NewStandardReconciler[T]` (or `NewStandardReconcilerWithFinalizer[T]`). This generic wrapper handles:
1. Fetching the object from the API server (returns early on NotFound)
2. Adding/removing a finalizer (`sailoperator.io/sail-operator` via `constants.FinalizerName`) on create/delete
3. Calling the controller's typed `Reconcile(ctx, obj)` function
4. Classifying errors and deciding retry behavior:
   - `Forbidden` with "RESTMapping" — requeue immediately (API server not ready)
   - `Conflict` / `NotFound` — requeue immediately
   - `TransientError` — requeue immediately
   - `ValidationError` — log and stop (no requeue)

Controllers never implement `ctrl.Reconciler` directly; they provide a `ReconcileFunc[T]` and optional `FinalizeFunc[T]`.

### Status Determination
Each controller follows the same pattern: `Reconcile()` calls `doReconcile()`, then `updateStatus()` regardless of whether reconciliation succeeded. Both errors are joined with `errors.Join()`.

Status updates use `reconciler.UpdateStatus()`, which compares old and new status via `reflect.DeepEqual` and patches only if changed (using `kube.NewStatusPatch`).

State derivation uses `reconciler.DeriveState()`: it iterates condition list in order and returns the reason of the first non-True condition, or the healthy reason if all are True.

**Condition types per resource:**
- `Istio`: Reconciled, Ready (derived from active IstioRevision), plus DependenciesHealthy (from revision)
- `IstioRevision`: Reconciled, Ready (istiod Deployment or remote webhook probe), DependenciesHealthy (IstioCNI + ZTunnel), InUse (namespace/pod/tag references)
- `IstioCNI`: Reconciled, Ready (DaemonSet readiness via `CheckDaemonSetReadiness`)
- `ZTunnel`: Reconciled, Ready (DaemonSet readiness via `CheckDaemonSetReadiness`)
- `IstioRevisionTag`: Reconciled, InUse

### Error Types and Retry
The `pkg/reconciler/errors.go` file defines four error types:
- `ValidationError` — stops reconciliation without requeue; surfaced in status conditions
- `TransientError` — requeued immediately; used for expected temporary failures
- `NameAlreadyExistsError` — when an IstioRevision and IstioRevisionTag have the same name; precedence is determined by `validation.ResourceTakesPrecedence`
- `ReferenceNotFoundError` — when a targetRef points to a nonexistent resource

### Watch Setup and Event Handling
Controllers use `Watches()` instead of `For()`/`Owns()` to wrap event handlers with `enqueuelogger.WrapIfNecessary()`. When `enqueuelogger.LogEnqueueEvents` is true, the wrapper logs every object enqueued for reconciliation along with the triggering event type and source object.

The `enqueuelogger` package wraps the work queue with `AdditionNotifierQueue`, which calls an `onAdd` callback on every `Add()`, `AddAfter()`, and `AddRateLimited()` call.

The IstioRevision controller has the most complex watch setup — it watches namespaces, pods, IstioRevisionTags, IstioCNI, ZTunnel, EndpointSlices, and many owned resource types (Deployments, Services, ConfigMaps, RBAC, webhooks, HPAs, PDBs, NetworkPolicies).

### Predicates
Two custom predicates in `pkg/predicate/predicate.go`:
- `IgnoreUpdate()` — drops all update events; used for ServiceAccounts to prevent removing pull secrets added by the system
- `IgnoreUpdateWhenAnnotation()` — drops update events when `sailoperator.io/ignore: "true"` is set on the resource

Controllers also define inline predicates:
- `ignoreStatusChange()` — allows updates only when spec, labels, annotations, owner references, or finalizers changed (not just status); uses `metadata.generation` comparison except for HPAs which require `reflect.DeepEqual` on spec
- `webhookConfigPredicate()` — ignores `caBundle` and `failurePolicy` changes on webhook configurations to prevent reconciliation loops with istiod

### Shared Reconcilers
The `pkg/reconcile/` package provides component-specific reconcilers that encapsulate validation, values computation, and Helm chart installation:

- `IstiodReconciler` — validates version/namespace/values, installs `istiod` chart plus `base` chart for the default revision
- `CNIReconciler` — validates version/namespace, computes values (image digests, vendor defaults, profiles), installs `cni` chart
- `ZTunnelReconciler` — validates version/namespace, computes values with optional base values from a referenced IstioRevision, applies FIPS values, installs `ztunnel` chart

All share a `Config` struct containing `ResourceFS`, `Platform`, `DefaultProfile`, `OperatorNamespace`, and `ChartManager`.

Helm release naming follows the pattern `<revision-or-resource-name>-<chart-name>` (e.g., `default-istiod`, `default-base`). CNI and ZTunnel use fixed release names (`istio-cni`, `ztunnel`).

## Gotchas
- **ServiceAccount pull secrets**: `IgnoreUpdate()` is applied to ServiceAccount watches across istiocni, ztunnel, and istiorevision controllers. Without this, the controller would remove pull secrets added by the Kubernetes ServiceAccount admission controller after Helm renders the ServiceAccount without them.
- **Webhook reconciliation loops**: The IstioRevision controller uses `webhookConfigPredicate()` to strip `caBundle`, `failurePolicy`, `resourceVersion`, `generation`, and `managedFields` before comparing webhook configurations. Istiod continuously updates these fields, which would otherwise trigger infinite reconciliation.
- **HPA generation not set**: Kubernetes does not set `metadata.generation` on HorizontalPodAutoscaler objects, so `ignoreStatusChange()` falls back to `reflect.DeepEqual` on the HPA spec to detect real changes.
- **External istiod skip**: The Istio controller skips revision pruning when `EXTERNAL_ISTIOD=true` is set in pilot env values, because it cannot determine if the revision is still in use on the external cluster.
- **IstioRevision/IstioRevisionTag name conflict**: Both resources validate during reconciliation that no resource of the other type exists with the same name. `validation.ResourceTakesPrecedence` determines which one wins.
- **ZTunnel watches both v1 and v1alpha1**: The ZTunnel controller watches both `v1.ZTunnel` and `v1alpha1.ZTunnel` resources for backwards compatibility.
- **ZTunnel values double-merge**: ZTunnel values require an extra `ApplyUserValues` step because the ztunnel Helm chart lacks the automatic descoping template that the CNI chart has (`zzy_descope_legacy.yaml`).

## Dependencies & Context
The controller architecture uses kubebuilder with controller-runtime. Each controller embeds `client.Client` and receives a `config.ReconcilerConfig` containing platform detection, default profiles, resource filesystem, and concurrency settings.

Controllers that install Helm charts (IstioRevision, IstioCNI, ZTunnel, IstioRevisionTag) hold a `*helm.ChartManager` reference. The Istio controller does not — it delegates chart installation entirely to IstioRevision via the shared `revision` package.

The `pkg/reconcile/` package was split from the controllers to allow the reconciliation logic to be exported as a library separate from the CRDs. This keeps the controller layer thin (CRD-specific validation and watches) while the shared layer handles validation, values computation, and Helm operations — enabling consumers to install Istio components without depending on the operator's custom resources.

Inter-controller coordination relies on Kubernetes primitives:
- Owner references: Istio owns IstioRevision; all component CRs own their Helm-rendered resources
- Status conditions: IstioRevision's Ready and DependenciesHealthy conditions aggregate IstioCNI and ZTunnel health
- Labels and annotations: `istio.io/rev` and `istio-injection` labels on namespaces/pods drive InUse detection

## Links
- [api-types-and-crds.md](api-types-and-crds.md) — CRD definitions and field semantics referenced by controllers
- [helm-and-values.md](helm-and-values.md) — Chart management and values processing used by shared reconcilers
- [version-management.md](version-management.md) — Version resolution called during reconciliation
- `pkg/revision/` — Shared revision utilities (ComputeValues, CreateOrUpdate, PruneInactive, ListOwned, dependency checks)
- `pkg/validation/` — Target namespace validation and resource precedence logic
- `pkg/kube/` — Kubernetes helpers (finalizer operations, status patching)
