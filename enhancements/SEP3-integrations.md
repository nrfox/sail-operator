|Status                                             | Authors      | Created    | 
|---------------------------------------------------|--------------|------------|
| WIP                                               | @nrfox       | 2026-07-22 |

# Integrations API

## Overview
Configuring Istio to work with various integrations, especially on OpenShift, often requires following rote procedures that are easy to get wrong. This SEP aims to provide a new CRD(s) to simplify this process while still giving users full control over their resources.

## Goals
- Simplify integrations with Istio, especially on OpenShift
- Allow for full customization without fighting against a controller

## Non-goals
- Modifying the existing Istio CRD.
- Provisioning or productizing `PersesDashboard` resources. The Integrations controller may provision a `PersesDatasource` when `MetricsIntegration` targets Perses (see [Perses datasource](#perses-datasource-metricsintegration) below). Dashboard content and lifecycle are defined in [SEP4-perses-dashboards.md](./SEP4-perses-dashboards.md).

## Design

Note that the Integrations controller detailed below will be the same one implemented as part of the [metrics integration SEP](https://github.com/istio-ecosystem/sail-operator/pull/2028). See the Implementation Plan for more details.

A new Integrations controller will be introduced along with new `Integration` types. Each type will be grouped by function. The `Integration` types will have a `targetRefs` field that specifies the resources the integration configures. Each target reference specifies the `kind` (e.g. `Istio`, `Kiali`), `name`, and optionally `namespace` of the target resource. A single `Integration` resource can target multiple resources, such as both an `Istio` and a `Kiali` resource. The Integration controller will configure the target resources and any other resources necessary to manage the integration based on which integrations are configured. For example, a UWM integration would look like this:
```yaml
kind: MetricsIntegration
apiVersion: sailoperator.io/v1alpha1
metadata:
  name: openshift-obserability
spec:
  targetRefs:
    - kind: Istio
      name: default
    - kind: Kiali
      name: kiali
      namespace: istio-system
  type: UserWorkloadMonitoring
  userWorkloadMonitoring: {}
```
A COO integration would look like this:
```yaml
kind: MetricsIntegration
apiVersion: sailoperator.io/v1alpha1
metadata:
  name: openshift-obserability
spec:
  targetRefs:
    - kind: Istio
      name: default
  type: ClusterObservabilityOperator
  clusterObservabilityOperator:
    monitoringStackRef:
      name: my-custom-prom
      namespace: custom-metrics
```
the Integrations controller would use the `monitoringStackRef` to reference the `MonitoringStack` for the prometheus instance and copy over any relevant fields such as the `resourceSelector` labels needed for the controller to label the `PodMonitor` and `ServiceMonitor` resources correctly.

Integrating with OpenShift COO and distributed tracing would involve a `MetricsIntegration` and a `TracingIntegration`:
```yaml
kind: MetricsIntegration
apiVersion: sailoperator.io/v1alpha1
metadata:
  name: openshift-obserability
spec:
  targetRefs:
    - kind: Istio
      name: default
  type: ClusterObservabilityOperator
  clusterObservabilityOperator:
    monitoringStackRef:
      name: my-custom-prom
      namespace: custom-metrics
---
kind: TracingIntegration
apiVersion: sailoperator.io/v1alpha1
metadata:
  name: openshift-obserability
spec:
  targetRefs:
    - kind: Istio
      name: default
  type: OpenTelemetry
  openTelemetry:
    otelCollectorRef:
      name: otel
      namespace: istio-system
```

The controller would then configure the following fields on the `Istio` and `Telemetry` resources:
```yaml
kind: Istio
apiVersion: sailoperator.io/v1
metadata:
  name: default
spec:
  values:
    meshConfig:
      enableTracing: true
      extensionProviders:
      - name: otel
        opentelemetry:
          port: 4317
          service: otel-collector.istio-system.svc.cluster.local
---
apiVersion: telemetry.istio.io/v1
kind: Telemetry
metadata:
  name: otel-demo
  namespace: istio-system
spec:
  metrics:
    - providers:
      - name: prometheus
  tracing:
    - providers:
        - name: otel
```

### Dealing with conflicts

A key part of the design is using **Server Side Apply** for all controller updates to resources. This will allow the Integrations controller to manage the fields of the `Istio` and `Telemetry` resources necessary to setup the integration while allowing users to manage other parts of the resources without users fighting against the controller. If users want to take full control over some of the controller managed fields, they can also do this cleanly with Server Side Apply. When [dealing with conflicts](https://kubernetes.io/docs/reference/using-api/server-side-apply/#conflicts), the controller will either give up management or become a shared manager. The controller will never overwrite values specified by the user or other controllers.

For users that manage their resources through Argo CD, the [Server Side Apply sync option](https://argo-cd.readthedocs.io/en/stable/user-guide/sync-options/#server-side-apply) must be enabled. This will cause Argo CD to use `kubectl apply --server-side --force-conflicts` and any fields that are written by both the Integrations controller and Argo CD will be owned by Argo CD. Since Server Side Apply is a stable, mature feature, it's assumed that other gitops solutions support a similar option.

### User Stories

- A mesh admin wants to configure Istio to work with UserWorkloadMonitoring on OpenShift. The admin wants Istio to work with UserWorkloadMonitoring without having to do any manual steps.
- A mesh admin wants to configure Kiali to read from UserWorkloadMonitoring and distributed tracing without having to perform any manual steps.
- A mesh admin on OpenShift wants a `PersesDatasource` in their Perses project wired to the same User Workload Monitoring Thanos endpoint as Kiali, without manually authoring datasource YAML.
- A mesh admin wants to maintain full control over the configuration of all resources in case any customizations are needed.

### API Changes

Three new CRDs will be added corresponding broadly to different integration types for Istio. These can be added separately and new `type` for each group added over time but it's not expected that we will add many new `Integration` CRDs. The new CRDs and subtypes are:

- `MetricsIntegration`
  - UserWorkloadMonitoring
  - ClusterObservabilityOperator
  - PrometheusOperator
- `TracingIntegration`
  - OpenTelemetry
  - TempoStack
- `CertificateIntegration`
  - ZeroTrustWorkloadIdentityManagement
  - IstioCSR
  - CertManager

Each `Integration` resource has a `targetRefs` field that specifies the resources the integration configures. Each target reference specifies the `kind` (e.g. `Istio`, `Kiali`, `Perses`), `name`, and optionally `namespace` of the target.

For `Istio` and `Kiali`, the controller patches an existing custom resource. For `Perses`, `targetRefs` identifies the **Perses project namespace** (the Kubernetes namespace where `PersesDatasource` resources are created and that maps to a Perses project). The controller does not patch a `Perses` custom resource; it creates `perses.dev` resources in that namespace.

A single `Integration` resource can target multiple resources, such as both an `Istio` and a `Kiali` resource. If there are multiple `Integration` resources of the same Kind that target the same ref, the one that is created later is considered invalid and this will be reflected in the status.

Here are examples of each type:

A `MetricsIntegration` targeting Istio, Kiali, and a Perses project for UWM:
```yaml
kind: MetricsIntegration
apiVersion: sailoperator.io/v1alpha1
metadata:
  name: openshift-observability
spec:
  targetRefs:
    - kind: Istio
      name: default
    - kind: Kiali
      name: kiali
      namespace: istio-system
    - kind: Perses
      name: perses-dev
      namespace: perses-dev
  type: UserWorkloadMonitoring
  userWorkloadMonitoring: {}
```

The `Perses` target ref uses `namespace` (and `name`, which must match `namespace`) as the Perses project where the Integrations controller provisions `PersesDatasource` resources. See [Perses datasource](#perses-datasource-metricsintegration).

A `TracingIntegration` targeting both Istio and Kiali:
```yaml
kind: TracingIntegration
apiVersion: sailoperator.io/v1alpha1
metadata:
  name: openshift-observability
spec:
  targetRefs:
    - kind: Istio
      name: default
    - kind: Kiali
      name: kiali
      namespace: istio-system
  type: TempoStack
  tempoStack:
    tempoStackRef:
      name: tempo
      namespace: tracing
```

Using the resources above, the controller would configure the following on the `Kiali` resource:
```yaml
apiVersion: kiali.io/v1alpha1
kind: Kiali
metadata:
  name: kiali
  namespace: istio-system
spec:
  external_services:
    prometheus:
      auth:
        type: bearer
        use_kiali_token: true
      thanos_proxy:
        enabled: true
      url: https://thanos-querier.openshift-monitoring.svc.cluster.local:9091
    tracing:
      enabled: true
      provider: tempo
      use_grpc: false
      internal_url: https://tempo-sample-gateway.tempo.svc.cluster.local:8080/api/traces/v1/default/tempo
      external_url: https://tempo-sample-gateway-tempo.apps-crc.testing/api/traces/v1/default/search 
      health_check_url: https://tempo-sample-gateway-tempo.apps-crc.testing/api/traces/v1/default/tempo/api/echo
      auth: 
        ca_file: /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
        insecure_skip_verify: false
        type: bearer
        use_kiali_token: true
      tempo_config:
         url_format: "jaeger"
    perses:
      enabled: true
      url: https://perses-ui-perses-dev.apps-crc.testing
      project: perses-dev
      auth:
        type: bearer
        use_kiali_token: true
```

When `targetRefs` includes `Perses`, the controller also patches `external_services.perses` on `Kiali` so deep links can resolve against dashboards in that Perses project. Dashboard CRs themselves are provisioned separately; see [SEP4-perses-dashboards.md](./SEP4-perses-dashboards.md).

#### Perses datasource (MetricsIntegration)

When a `MetricsIntegration` includes a `targetRefs` entry with `kind: Perses`, the Integrations controller provisions a `PersesDatasource` custom resource (`perses.dev/v1alpha2`) in the Perses project namespace. This wires Perses to the same metrics backend selected by the integration `type`, analogous to how a `Kiali` target wires `external_services.prometheus`.

| `MetricsIntegration.type` | `PersesDatasource` endpoint |
|---------------------------|-----------------------------|
| `UserWorkloadMonitoring` | OpenShift User Workload Monitoring Thanos querier (same URL/auth as Kiali `external_services.prometheus` for UWM) |
| `ClusterObservabilityOperator` | Prometheus/Thanos URL derived from the referenced `MonitoringStack` (same as the COO metrics path for Istio scraping) |

The productized datasource resource name is `prometheus-datasource`, matching [perses/community-mixins](https://github.com/perses/community-mixins) dashboard queries. The controller applies datasource fields with Server Side Apply so users can customize proxy URL, authentication, or TLS without fighting the controller.

Example `PersesDatasource` created for UWM (fields set by the controller on OpenShift may include bearer auth and service CA paths):

```yaml
apiVersion: perses.dev/v1alpha2
kind: PersesDatasource
metadata:
  name: prometheus-datasource
  namespace: perses-dev
  labels:
    app.kubernetes.io/part-of: sail-operator
spec:
  config:
    display:
      name: Prometheus
    default: true
    plugin:
      kind: PrometheusDatasource
      spec:
        proxy:
          kind: HTTPProxy
          spec:
            url: https://thanos-querier.openshift-monitoring.svc.cluster.local:9091
```

Before creating `PersesDatasource` resources, the controller detects that `perses.dev/v1alpha2` CRDs are installed (typically via Cluster Observability Operator). If CRDs are missing, reconciliation continues for other targets (`Istio`, `Kiali`, monitors) and status reports a `PersesAvailable=False` condition with reason `MissingCRDs`. Sail Operator does not install the Perses Operator or COO.

The Integrations controller **does not** create `PersesDashboard` resources. Users install dashboards via [SEP4-perses-dashboards.md](./SEP4-perses-dashboards.md) or manual YAML; panels must reference a datasource named `prometheus-datasource` in the same namespace unless customized.

Integrating Istio with Zero Trust Workload Identity Management:
```yaml
kind: CertificateIntegration
apiVersion: sailoperator.io/v1alpha1
metadata:
  name: ztwim
spec:
  targetRefs:
    - kind: Istio
      name: default
  type: ZeroTrustWorkloadIdentityManagement
  ztwim: {}
```

Integrating Istio with Istio CSR:
```yaml
kind: CertificateIntegration
apiVersion: sailoperator.io/v1alpha1
metadata:
  name: istio-csr
spec:
  targetRefs:
    - kind: Istio
      name: default
  type: IstioCSR
  istioCSR:
    istioCSRRef:
      name: default
      namespace: istio-csr
```

These are broadly what the golang API changes would be:
```go
// TargetReference identifies a resource that the integration configures or a Perses
// project namespace where the controller provisions PersesDatasource resources.
type TargetReference struct {
	// Kind specifies the target kind: "Istio", "Kiali", or "Perses".
	// For Perses, name and namespace identify the Perses project namespace; the controller
	// creates PersesDatasource resources there rather than patching an existing CR.
	Kind string `json:"kind"`

	// Name is the name of the target resource.
	Name string `json:"name"`

	// Namespace is the namespace of the target resource.
	// Only required for namespace-scoped resources like Kiali.
	Namespace string `json:"namespace,omitempty"`
}

// MetricsIntegrationSpec defines the desired state of MetricsIntegration.
type MetricsIntegrationSpec struct {
	// TargetRefs specifies the resources that this integration configures.
	TargetRefs []TargetReference `json:"targetRefs"`

	MetricsConfig `json:",inline"`
}

// MetricsType identifies the type of metrics integration.
type MetricsType string

const (
	MetricsTypeUserWorkloadMonitoring      MetricsType = "UserWorkloadMonitoring"
	MetricsTypeClusterObservabilityOperator MetricsType = "ClusterObservabilityOperator"
)

// MetricsConfig configures a metrics backend.
type MetricsConfig struct {
	// Type specifies the metrics integration type.
	Type MetricsType `json:"type"`

	// UserWorkloadMonitoring configures integration with OpenShift User Workload Monitoring.
	UserWorkloadMonitoring *UserWorkloadMonitoringConfig `json:"userWorkloadMonitoring,omitempty"`

	// ClusterObservabilityOperator configures integration with the Cluster Observability
	// Operator's MonitoringStack resource for metrics collection.
	ClusterObservabilityOperator *ClusterObservabilityOperatorConfig `json:"clusterObservabilityOperator,omitempty"`
}

type UserWorkloadMonitoringConfig struct{}

// ClusterObservabilityOperatorConfig configures the Cluster Observability Operator integration.
type ClusterObservabilityOperatorConfig struct {
	// MonitoringStackRef is a reference to a MonitoringStack resource that defines
	// the Prometheus stack used for scraping Istio metrics.
	MonitoringStackRef NamespacedReference `json:"monitoringStackRef"`
}

// TracingIntegrationSpec defines the desired state of TracingIntegration.
type TracingIntegrationSpec struct {
	// TargetRefs specifies the resources that this integration configures.
	TargetRefs []TargetReference `json:"targetRefs"`

	TracingConfig `json:",inline"`
}

// TracingType identifies the type of tracing integration.
type TracingType string

const (
	TracingTypeOpenTelemetry TracingType = "OpenTelemetry"
	TracingTypeTempoStack    TracingType = "TempoStack"
)

// TracingConfig configures a tracing backend.
type TracingConfig struct {
	// Type specifies the tracing integration type.
	Type TracingType `json:"type"`

	// OpenTelemetry configures integration with an OpenTelemetry Collector.
	OpenTelemetry *OpenTelemetryConfig `json:"openTelemetry,omitempty"`

	// TempoStack configures integration with a TempoStack resource.
	TempoStack *TempoStackConfig `json:"tempoStack,omitempty"`
}

// OpenTelemetryConfig configures the OpenTelemetry integration.
type OpenTelemetryConfig struct {
	// OTELCollectorRef is a reference to an OpenTelemetry Collector resource.
	OTELCollectorRef NamespacedReference `json:"otelCollectorRef"`
}

// TempoStackConfig configures the TempoStack integration.
type TempoStackConfig struct {
	// TempoStackRef is a reference to a TempoStack resource.
	TempoStackRef NamespacedReference `json:"tempoStackRef"`
}

// CertificateIntegrationSpec defines the desired state of CertificateIntegration.
type CertificateIntegrationSpec struct {
	// TargetRefs specifies the resources that this integration configures.
	TargetRefs []TargetReference `json:"targetRefs"`

	// Type specifies the identity integration type.
	Type IdentityType `json:"type"`

	// ZTWIM configures integration with Zero Trust Workload Identity Management.
	ZTWIM *ZTWIMConfig `json:"ztwim,omitempty"`

	// IstioCSR configures integration with cert-manager istio-csr.
	IstioCSR *IstioCSRConfig `json:"istioCSR,omitempty"`
}

// IdentityType identifies the type of identity integration.
type IdentityType string

const (
	IdentityTypeZTWIM    IdentityType = "ZeroTrustWorkloadIdentityManagement"
	IdentityTypeIstioCSR IdentityType = "IstioCSR"
)

// ZTWIMConfig configures the Zero Trust Workload Identity Management integration.
type ZTWIMConfig struct{}

// IstioCSRConfig configures the cert-manager istio-csr integration.
type IstioCSRConfig struct {
	// IstioCSRRef is a reference to an IstioCSR resource.
	IstioCSRRef NamespacedReference `json:"istioCSRRef"`
}
```

#### Status
Integrations will report `Status`. Non-exhaustive list of what should be in `Status`:
- Validations: do the refs exist?
- Success/failure to update resources.
- Possibly report if the update was partially applied i.e. some other controller owns part of the fields.

#### Migration

Some users will already have configured their integrations. They may already have `PodMonitor` and `ServiceMonitor` resources created for example. How should the Integrations controller handles this?

The integrations controller could either:

1. Adopt the resources i.e. add the Integrations controller's OwnerRef to them. 
2. Do nothing.
3. Create parallel resources. In the case of `PodMonitor` creating a second `PodMonitor`.

The Integrations controller will implement option 3. and will create new resources regardless of any existing resources. This is the simplest option. Users who have already configured their integrations may not need this API and if they want to utilize this API they can remove their existing resources before or after creating the `Integration` resource.

#### Resource Ownership

The Integrations controller should own the resources that it directly creates as part of the integration. This will tie the lifecycle of these resources to the `Integration` ensuring resources are properly cleaned up when the `Integration` is removed. The Integrations controller will **not** put an `ownerRef` on any of the resources that it references. The Integrations controller will never put an `ownerRef` on an `Istio` resource or on an `OpenTelemetry` resource. 

#### Permissions

Adding this API will require adding new permissions to the Sail Operator for each `Integration`. In general, the Sail Operator will need `READ` permissions for all of the `Integration` types and subtypes, `PATCH` permissions for any `targetRef`, and `CREATE`/`PATCH` for all resources the Integrations controller creates e.g. `PodMonitor`. Other permissions may also be needed for different integrations.

Adding the Cluster Observability Operator integration would require adding:
- `GET`/`WATCH`/`LIST` for `MonitoringStack` resources
- `PATCH` for `Kiali` resources (the Sail Operator already has permission to patch `Istio` resources)
- `CREATE`/`PATCH` for `PodMonitor`/`ServiceMonitor` resources.

When `MetricsIntegration` targets Perses, the Integrations controller also needs:
- `CREATE`/`PATCH`/`DELETE` for `PersesDatasource` resources (`perses.dev/v1alpha2`)

This will greatly increase the scope of the Sail Operator's Service Account but the operator already has full control of `Secret` and `ClusterRole`/`ClusterRoleBinding` resources effectively giving it cluster admin for the cluster.

### Architecture

```mermaid
flowchart TD
    subgraph "Resources"
        MI["MetricsIntegration"]
        TI["TracingIntegration"]
    end

    subgraph "Targets patched"
        Istio["Istio"]
        Kiali["Kiali"]
    end

    subgraph "Resources created"
        PDS["PersesDatasource"]
        Monitors["PodMonitor / ServiceMonitor"]
    end

    subgraph "References"
        MS["MonitoringStack\n(COO)"]
        TS["TempoStack\n(Tempo Operator)"]
    end

    MI -- "targetRefs:\nIstio" --> Istio
    MI -- "targetRefs:\nKiali" --> Kiali
    MI -- "targetRefs:\nPerses" --> PDS
    MI --> Monitors
    MI -. "metrics:\nClusterObservability" .-> MS

    TI -- "targetRefs:\nIstio" --> Istio
    TI -- "targetRefs:\nKiali" --> Kiali
    TI -. "tracing:\nTempoStack" .-> TS

    style MI fill:#4a9eff,color:#fff
    style TI fill:#4a9eff,color:#fff
```

### Performance Impact

The exact performance impact will partially depend on the implementation details of each integration type. For example, the UWM integration requires creating/watching more resources so it will likely have a greater impact but the tracing integration only requires updating some fields on the `Istio` resource and reconciling `Telemetry` objects.

### Kubernetes vs OpenShift vs Other Distributions

Some of this controller will only be applicable to OpenShift. The UWM and COO types are OpenShift specific but other types, such as `TempoStack` would be valid on either Kubernetes or OpenShift. `PersesDatasource` provisioning via `MetricsIntegration` is reconciled only when `perses.dev` CRDs are present; the default UWM and COO datasource endpoints are OpenShift-specific.

## Alternatives Considered
- The main alternative to having a separate CRD for the integrations is to add fields to the `Istio` spec directly. One drawback of this approach is that there isn't a clear separation of concerns. Today the istio controller alone reconciles the `Istio` spec. If the integrations controller began to reconcile parts of the `Istio` spec, care would need to be taken to ensure the two controllers do not fight with one another. Having a separate resources also allows the API to evolve and rapidly add new types without affecting the stable `Istio` API.

- Two types with many subtypes. In this scenario you would have a `IstioIntegration` type and a `KialiIntegration` type rather than grouping the integrations by their function.

  ```yaml
  kind: IstioIntegration
  apiVersion: sailoperator.io/v1alpha1
  metadata:
    name: openshift-obserability
  spec:
    istioRef:
      name: default
    metrics:
      type: ClusterObservabilityOperator
      clusterObservabilityOperator:
        monitoringStackRef:
          name: my-custom-prom
          namespace: custom-metrics
    tracing:
      type: OpenTelemetry
      openTelemetry:
        otelCollectorRef:
          name: otel
          namespace: istio-system
  ```
  
  The main disadvantage of this API is that you end up with a single large CR with many different fields and subtypes.

- No subtypes and only CRDs e.g.
  ```yaml
  kind: ClusterObservabilityOperator
  apiVersion: sailoperator.io/v1alpha1
  metadata:
    name: openshift-obserability
  spec:
    monitoringStackRef:
      name: my-custom-prom
      namespace: custom-metrics
  ```

  With this API you end up with a large number of CRDs that have very few fields.

- A separate `DashboardIntegration` or `PersesIntegration` CRD was considered for Perses dashboards and datasource wiring. It was not adopted: Perses dashboards are cluster-wide deliverables (see [SEP4-perses-dashboards.md](./SEP4-perses-dashboards.md)), while metrics backend wiring for Perses belongs in `MetricsIntegration` alongside Kiali prometheus configuration.

## Implementation Plan
The implementation for UWM is already complete as part of the [monitoring controller](https://github.com/istio-ecosystem/sail-operator/pull/1959). The only user facing change would be switching the enablement from an annotation on the `Istio` resource to creating a separate `MetricsIntegration` resource. The monitoring controller implementation would change slightly to reconcile `MetricsIntegration` resources and use Server Side Apply to update the `Istio` and `Telemetry` resources. A rough timeline would be:

- [ ] Add `MetricsIntegration` CRD
- [ ] Update monitoring controller for UWM to reconcile `MetricsIntegration` resources.
- [ ] Add `TracingIntegration` CRD
- [ ] Add `CertificateIntegration` CRD
- [ ] Add a `Kiali` target on the `Integration` resources.
- [ ] When `MetricsIntegration` targets `Perses`, provision `PersesDatasource` aligned with UWM or COO `MonitoringStack` and patch Kiali `external_services.perses` when `Kiali` is also targeted.

## Test Plan
- A key aspect of this design is utilizing Server Side Apply to ensure that users can override values that the operator sets if need be without fighting against the controller. This needs to be an integral part of the test suite and will be included in e2e testing. Specifically e2e testing should ensure that the operator can Apply a configuration partially and ignore any conflict errors.
- Some of the integrations types will only be available on OpenShift like the UWM and COO types. e2e tests for these can only be run in an OpenShift environment. These will be filtered out of the kind based suite with the openshift label similar to the TLS profile tests.
- e2e tests for `PersesDatasource` provisioning should verify SSA behavior, `PersesAvailable` status when CRDs are missing, and alignment between Kiali prometheus URL and datasource proxy URL for UWM.

## Change History (only required when making changes after SEP has been accepted)
- Changed the API from `IstioIntegration` --> `<Component>Integration`.
- Replaced `istioRef` + `dashboard` fields with a unified `target` discriminated union (Istio | Kiali).
- Replaced `target` discriminated union with `targetRefs` array of references.
- Updated migration section to ignore any existing resources.