# TracingIntegration Implementation Notes

## Current state

Work was stopped before final review. After this initial pass, I read the local Claude convention files:

- `/home/nfox/.claude/programming/best_practices.md`
- `/home/nfox/.claude/programming/go/best_practices.md`
- `/home/nfox/.claude/programming/go/operators.md`

Key convention to apply before resuming: use real Go/Kubernetes API types wherever possible rather than building unstructured `map[string]any` payloads. For Kubernetes operators, prefer typed objects, `client.ObjectKey`, typed metadata fields where possible, `apierrors` for API errors, and `controllerutil.SetControllerReference` for owner refs.

The working tree currently contains an initial implementation of:

- `api/v1alpha1/tracingintegration_types.go`
- `controllers/integration/tracing.go`
- registration updates in:
  - `api/v1alpha1/groupversion_info.go`
  - `pkg/scheme/scheme.go`
  - `cmd/main.go`
- generated deepcopy updates in `api/v1alpha1/zz_generated.deepcopy.go`
- generated CRD files:
  - `chart/crds/sailoperator.io_tracingintegrations.yaml`
  - `bundle/manifests/sailoperator.io_tracingintegrations.yaml`
- RBAC updates in:
  - `chart/templates/rbac/role.yaml`
  - `bundle/manifests/sailoperator.clusterserviceversion.yaml`

## Validation run

`make all` was run and passed after the current code changes. It emitted existing/non-blocking warnings from bundle validation and CRD compatibility checks, including `NoEnumRemoval` warnings for existing version enum differences and warnings about existing owned CRDs missing descriptions.

## Gotchas hit

1. **DeepCopy generation is required for new API types**
   - Adding `TracingIntegration` to `api/v1alpha1/groupversion_info.go` fails compilation until `make gen-code` is run because the new type must implement `runtime.Object` via generated `DeepCopyObject`.

2. **Telemetry v1 scheme was not registered**
   - The controller watches/applies `telemetry.istio.io/v1` `Telemetry` resources, so `istio.io/client-go/pkg/apis/telemetry/v1` needed to be added to `pkg/scheme/scheme.go`.

3. **CRD condition schema requirements**
   - The project’s CRD compatibility checker expects standard condition SSA markers:
     - `x-kubernetes-list-type: map`
     - `x-kubernetes-list-map-keys: [type]`
     - `observedGeneration` on conditions
   - Reusing the project’s `api/v1.StatusCondition` did not include these fields/markers, and changing it would have modified all existing CRDs. I switched only `TracingIntegrationStatus.Conditions` to `[]metav1.Condition` to satisfy this for the new API without touching existing APIs.

4. **`metav1.SetStatusCondition` does not exist**
   - The helper is `apimeta.SetStatusCondition` from `k8s.io/apimachinery/pkg/api/meta`.

5. **Server-side apply conflicts**
   - The SEP says the controller should never force conflicts. The controller uses `types.ApplyPatchType` with `client.FieldOwner(...)`, but intentionally does not use `client.ForceOwnership`.
   - On SSA conflict, the reconciler records status and returns no reconcile error so it does not hot-loop trying to take ownership.

6. **SSA creation of owned Telemetry and owner refs**
   - SSA patches include `ownerReferences` for the generated `Telemetry` resource because the SEP says directly created resources should be owned by the integration.
   - `blockOwnerDeletion` was removed from that owner ref to avoid requiring extra permissions on finalizers/admission behavior.

7. **Cluster-scoped owner to namespaced object**
   - `TracingIntegration` is cluster-scoped and `Telemetry` is namespaced. Kubernetes supports a cluster-scoped owner owning a namespaced dependent, but this is worth confirming as an intended lifecycle model.

8. **CRD generation touched existing CRDs at one point**
   - Running `make gen-manifests` regenerated existing CRDs due to local generated/version-list differences. I reverted those unrelated existing CRD changes and kept only the new `TracingIntegration` CRD.

9. **Bundle/CSV updates are partly manual**
   - `make gen-manifests` creates the chart CRD, but the bundle CRD and CSV owned CRD/RBAC/sample entries were updated manually. A full `make gen` may later rewrite these.

10. **Provider service naming is assumed**
    - OpenTelemetry service is currently assumed to be `<collector-name>-collector.<namespace>.svc.cluster.local`.
    - TempoStack service is currently assumed to be `<tempostack-name>-distributor.<namespace>.svc.cluster.local`.
    - These may need to be derived from operator status or configurable fields instead.

11. **Only Istio target is implemented**
    - `Kiali` target handling is not implemented. Unsupported target kinds currently mark the `TracingIntegration` invalid.

12. **No unit/integration/e2e tests were added**
    - Per request, no e2e tests were implemented. There are also no unit tests yet.

## Follow-up changes in this pass

- Reworked controller apply paths to build typed `Istio` and `Telemetry` objects instead of hand-written `map[string]any` payloads. The typed objects are converted to unstructured apply configurations only at the controller-runtime SSA boundary.
- Changed generated `Telemetry` name to `mesh-default`.
- Left `TempoStack` in the API, but made controller behavior explicitly return `InvalidConfiguration`/not implemented and removed TempoStack read RBAC from the role/CSV.
- Added an OpenTelemetry service-convention TODO near the service name derivation.
- Added a separate `Conflicted` condition. SSA conflicts no longer make `Reconciled=False`; they set `Reconciled=True`, `Conflicted=True`, and state `ApplyConflict` without retrying/forcing ownership.
- Used `client.ObjectKey` and `controllerutil.SetControllerReference` in controller code; `blockOwnerDeletion` is cleared after setting the controller reference to avoid requiring finalizer/update admission permissions for this initial implementation.
- Ran `make gen-manifests`; the chart and bundle `TracingIntegration` CRDs are identical.
- Ran `make all` successfully. Existing/non-blocking bundle validation warnings and existing CRD compatibility `NoEnumRemoval` messages were still emitted.

## Open questions for Nick

1. Should `TracingIntegration` be cluster-scoped as currently implemented, or namespace-scoped?

2. For `OpenTelemetry`, should the provider name be derived from `otelCollectorRef.name`, fixed to `otel`, or configurable in `spec.openTelemetry`?

3. For `OpenTelemetry`, is `<name>-collector.<namespace>.svc.cluster.local:4317` the correct service/port convention for all supported OpenTelemetry Operator versions, or should the controller read the `OpenTelemetryCollector` status/spec/service?

4. For `TempoStack`, should this initial implementation support it now, or should `TempoStack` be API-only until the Tempo service/status contract is better defined?

5. For `TempoStack`, what should the Istio `extensionProviders` service point at? The current assumption is `<name>-distributor.<namespace>.svc.cluster.local:4317`.

6. What name should the generated `Telemetry` resource use? The current implementation uses `tracing-<TracingIntegration name>` in the Istio control plane namespace.

7. Should the generated `Telemetry` resource be mesh-wide in the control plane namespace with no selector, as currently implemented, or should it target a particular revision/workload selection?

8. On SSA conflicts, should status distinguish partial success from complete failure? Currently any conflict marks `Reconciled=False` with reason `ReconcileError`, while avoiding retry.

9. Should duplicate target conflicts consider only `kind/name/namespace`, as currently implemented, or also API group/version once the API grows beyond simple `kind`?

10. Do we want the common `TargetReference` and `NamespacedReference` types in a separate shared API file now, anticipating `MetricsIntegration` and `CertificateIntegration`, or is keeping them in `tracingintegration_types.go` acceptable until those APIs are added?

## Answers

1. `TracingIntegration` should be cluster-scoped.

2. OpenTelemetry provider name should be derived from `otelCollectorRef.name`.

3. Use the `<otelCollectorRef.name>-collector.<namespace>.svc.cluster.local:4317` service convention for now. Add a TODO near that code asking whether the controller should read the collector resource/status/spec/service instead.

4. Do not implement `TempoStack` behavior now.

5. Do not implement `TempoStack` behavior now, including service derivation.

6. The generated `Telemetry` resource should be named `mesh-default`.

7. The generated `Telemetry` resource should be mesh-wide in the Istio control plane namespace.

8. SSA conflicts should have a separate condition, but conflicts are not a failure state. The overall integration should not be marked failed just because conflicts exist.

9. For now, duplicate target conflict detection should use only `kind`, `name`, and optional `namespace`. API group/version may be considered in the future if `TargetReference` grows those fields or if ambiguous target kinds become a concern.

10. Keep `TargetReference` and `NamespacedReference` in `tracingintegration_types.go` for now; do not move them to a separate shared API file yet.

11. Controller apply code should use real Kubernetes/API types rather than `map[string]any` patches where possible. For example, `applyTelemetry` should construct a typed `telemetryv1.Telemetry` instead of a `map[string]any` payload.
