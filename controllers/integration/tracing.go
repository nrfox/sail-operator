// Copyright Istio Authors
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

package integration

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	v1 "github.com/istio-ecosystem/sail-operator/api/v1"
	"github.com/istio-ecosystem/sail-operator/api/v1alpha1"
	"github.com/istio-ecosystem/sail-operator/pkg/config"
	"github.com/istio-ecosystem/sail-operator/pkg/enqueuelogger"
	"github.com/istio-ecosystem/sail-operator/pkg/reconciler"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	telemetryapiv1 "istio.io/api/telemetry/v1"
	telemetryv1 "istio.io/client-go/pkg/apis/telemetry/v1"
)

const (
	fieldManager           = "sail-operator-tracing-integration"
	otelCollectorKind      = "OpenTelemetryCollector"
	otelCollectorAPIGroup  = "opentelemetry.io"
	otelCollectorAPIVer    = "v1beta1"
	defaultOTLPGRPCPort    = 4317
	telemetryResourceLabel = "sailoperator.io/tracing-integration"
)

// TracingReconciler reconciles TracingIntegration objects.
type TracingReconciler struct {
	client.Client
	Config config.ReconcilerConfig
	Scheme *runtime.Scheme
}

func NewTracingReconciler(cfg config.ReconcilerConfig, client client.Client, scheme *runtime.Scheme) *TracingReconciler {
	return &TracingReconciler{
		Config: cfg,
		Client: client,
		Scheme: scheme,
	}
}

// +kubebuilder:rbac:groups=sailoperator.io,resources=tracingintegrations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sailoperator.io,resources=tracingintegrations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=sailoperator.io,resources=istios,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=telemetry.istio.io,resources=telemetries,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opentelemetry.io,resources=opentelemetrycollectors,verbs=get;list;watch

func (r *TracingReconciler) Reconcile(ctx context.Context, tracingIntegration *v1alpha1.TracingIntegration) (ctrl.Result, error) {
	reconcileErr := r.doReconcile(ctx, tracingIntegration)
	statusErr := r.updateStatus(ctx, tracingIntegration, reconcileErr)

	if apierrors.IsConflict(reconcileErr) {
		// Server-side apply conflicts should be visible in status, but the controller
		// must not force ownership away from users or other controllers.
		return ctrl.Result{}, statusErr
	}
	return ctrl.Result{}, errors.Join(reconcileErr, statusErr)
}

func (r *TracingReconciler) doReconcile(ctx context.Context, tracingIntegration *v1alpha1.TracingIntegration) error {
	providerName, service, port, err := r.tracingProvider(ctx, tracingIntegration)
	if err != nil {
		return err
	}

	if err := r.validateNoNewerConflict(ctx, tracingIntegration); err != nil {
		return err
	}

	for _, targetRef := range tracingIntegration.Spec.TargetRefs {
		switch targetRef.Kind {
		case v1.IstioKind:
			if targetRef.Namespace != "" {
				return reconciler.NewValidationError(fmt.Sprintf("targetRef %s/%s must not specify namespace for cluster-scoped Istio", targetRef.Kind, targetRef.Name))
			}
			if err := r.reconcileIstioTarget(ctx, tracingIntegration, targetRef.Name, providerName, service, port); err != nil {
				return err
			}
		default:
			return reconciler.NewValidationError(fmt.Sprintf("unsupported tracing target kind %q", targetRef.Kind))
		}
	}

	return nil
}

func (r *TracingReconciler) tracingProvider(
	ctx context.Context, tracingIntegration *v1alpha1.TracingIntegration,
) (name, service string, port uint32, err error) {
	switch tracingIntegration.Spec.Type {
	case v1alpha1.TracingTypeOpenTelemetry:
		if tracingIntegration.Spec.OpenTelemetry == nil {
			return "", "", 0, reconciler.NewValidationError("spec.openTelemetry must be set when spec.type is OpenTelemetry")
		}
		ref := tracingIntegration.Spec.OpenTelemetry.OTELCollectorRef
		if err := r.validateReferenceExists(ctx, otelCollectorAPIGroup, otelCollectorAPIVer, otelCollectorKind, ref); err != nil {
			return "", "", 0, err
		}
		// TODO: confirm whether this should be derived from the OpenTelemetryCollector
		// resource status/spec/service instead of using the operator service convention.
		return ref.Name, fmt.Sprintf("%s-collector.%s.svc.cluster.local", ref.Name, ref.Namespace), defaultOTLPGRPCPort, nil
	case v1alpha1.TracingTypeTempoStack:
		return "", "", 0, reconciler.NewValidationError("TempoStack tracing integration is not implemented yet")
	default:
		return "", "", 0, reconciler.NewValidationError(fmt.Sprintf("unsupported tracing integration type %q", tracingIntegration.Spec.Type))
	}
}

func (r *TracingReconciler) validateReferenceExists(ctx context.Context, group, version, kind string, ref v1alpha1.NamespacedReference) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: group, Version: version, Kind: kind})
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name}, obj); err != nil {
		if apierrors.IsNotFound(err) || apimeta.IsNoMatchError(err) {
			return reconciler.NewValidationError(fmt.Sprintf("referenced %s %s/%s was not found", kind, ref.Namespace, ref.Name))
		}
		return err
	}
	return nil
}

func (r *TracingReconciler) validateNoNewerConflict(ctx context.Context, tracingIntegration *v1alpha1.TracingIntegration) error {
	integrations := &v1alpha1.TracingIntegrationList{}
	if err := r.Client.List(ctx, integrations); err != nil {
		return err
	}

	for _, other := range integrations.Items {
		if other.Name == tracingIntegration.Name {
			continue
		}
		if !createdBefore(&other, tracingIntegration) {
			continue
		}
		for _, targetRef := range tracingIntegration.Spec.TargetRefs {
			for _, otherTargetRef := range other.Spec.TargetRefs {
				if targetRef == otherTargetRef {
					return reconciler.NewValidationError(fmt.Sprintf(
						"targetRef %s/%s is already managed by TracingIntegration %q", targetRef.Kind, targetRef.Name, other.Name))
				}
			}
		}
	}
	return nil
}

func createdBefore(a, b *v1alpha1.TracingIntegration) bool {
	if !a.CreationTimestamp.Equal(&b.CreationTimestamp) {
		return a.CreationTimestamp.Before(&b.CreationTimestamp)
	}
	return a.Name < b.Name
}

func (r *TracingReconciler) reconcileIstioTarget(
	ctx context.Context, tracingIntegration *v1alpha1.TracingIntegration, istioName, providerName, service string, port uint32,
) error {
	istio := &v1.Istio{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: istioName}, istio); err != nil {
		if apierrors.IsNotFound(err) {
			return reconciler.NewValidationError(fmt.Sprintf("target Istio %q was not found", istioName))
		}
		return err
	}

	if err := r.applyIstioTracing(ctx, istioName, providerName, service, port); err != nil {
		return err
	}
	return r.applyTelemetry(ctx, tracingIntegration, istio.Spec.Namespace, providerName)
}

func (r *TracingReconciler) applyIstioTracing(ctx context.Context, istioName, providerName, service string, port uint32) error {
	obj := &v1.Istio{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.GroupVersion.String(),
			Kind:       v1.IstioKind,
		},
		ObjectMeta: metav1.ObjectMeta{Name: istioName},
		Spec: v1.IstioSpec{
			Values: &v1.Values{
				MeshConfig: &v1.MeshConfig{
					EnableTracing: ptr.To(true),
					ExtensionProviders: []*v1.MeshConfigExtensionProvider{
						{
							Name: ptr.To(providerName),
							Opentelemetry: &v1.MeshConfigExtensionProviderOpenTelemetryTracingProvider{
								Service: ptr.To(service),
								Port:    ptr.To(port),
							},
						},
					},
				},
			},
		},
	}
	return r.applyObject(ctx, obj)
}

func (r *TracingReconciler) applyTelemetry(ctx context.Context, tracingIntegration *v1alpha1.TracingIntegration, namespace, providerName string) error {
	if namespace == "" {
		namespace = "istio-system"
	}

	obj := &telemetryv1.Telemetry{
		TypeMeta: metav1.TypeMeta{
			APIVersion: telemetryv1.SchemeGroupVersion.String(),
			Kind:       "Telemetry",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      telemetryName(),
			Namespace: namespace,
			Labels: map[string]string{
				telemetryResourceLabel: tracingIntegration.Name,
			},
		},
		Spec: telemetryapiv1.Telemetry{
			Tracing: []*telemetryapiv1.Tracing{
				{
					Providers: []*telemetryapiv1.ProviderRef{
						{Name: providerName},
					},
				},
			},
		},
	}
	if err := controllerutil.SetControllerReference(tracingIntegration, obj, r.Scheme); err != nil {
		return fmt.Errorf("setting controller reference on Telemetry %s/%s: %w", namespace, telemetryName(), err)
	}
	clearBlockOwnerDeletion(obj)

	return r.applyObject(ctx, obj)
}

func (r *TracingReconciler) applyObject(ctx context.Context, obj client.Object) error {
	unstructuredObj, err := toUnstructured(obj)
	if err != nil {
		return err
	}
	return r.Client.Apply(ctx, client.ApplyConfigurationFromUnstructured(unstructuredObj), client.FieldOwner(fieldManager))
}

func toUnstructured(obj client.Object) (*unstructured.Unstructured, error) {
	content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, fmt.Errorf(
			"converting %s %s/%s to unstructured apply configuration: %w",
			obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName(), err,
		)
	}
	delete(content, "status")
	unstructured.RemoveNestedField(content, "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(content, "metadata", "managedFields")
	return &unstructured.Unstructured{Object: content}, nil
}

func clearBlockOwnerDeletion(obj client.Object) {
	ownerReferences := obj.GetOwnerReferences()
	for i := range ownerReferences {
		ownerReferences[i].BlockOwnerDeletion = nil
	}
	obj.SetOwnerReferences(ownerReferences)
}

func telemetryName() string {
	return "mesh-default"
}

func (r *TracingReconciler) updateStatus(ctx context.Context, tracingIntegration *v1alpha1.TracingIntegration, reconcileErr error) error {
	status := *tracingIntegration.Status.DeepCopy()
	status.ObservedGeneration = tracingIntegration.Generation
	status.SetCondition(r.determineReconciledCondition(tracingIntegration.Generation, reconcileErr))
	status.SetCondition(r.determineConflictedCondition(tracingIntegration.Generation, reconcileErr))
	status.State = determineState(status)
	return reconciler.UpdateStatus(ctx, r.Client, tracingIntegration, tracingIntegration.Status, status, nil)
}

func (r *TracingReconciler) determineReconciledCondition(generation int64, err error) metav1.Condition {
	condition := metav1.Condition{
		ObservedGeneration: generation,
		Type:               string(v1alpha1.TracingIntegrationConditionReconciled),
	}
	if err == nil || apierrors.IsConflict(err) {
		condition.Status = metav1.ConditionTrue
		condition.Reason = string(v1alpha1.TracingIntegrationConditionReconciled)
		return condition
	}

	condition.Status = metav1.ConditionFalse
	condition.Message = fmt.Sprintf("error reconciling resource: %v", err)
	if reconciler.IsValidationError(err) {
		condition.Reason = string(v1alpha1.TracingIntegrationReasonInvalidConfiguration)
	} else {
		condition.Reason = string(v1alpha1.TracingIntegrationReasonReconcileError)
	}
	return condition
}

func (r *TracingReconciler) determineConflictedCondition(generation int64, err error) metav1.Condition {
	condition := metav1.Condition{
		ObservedGeneration: generation,
		Type:               string(v1alpha1.TracingIntegrationConditionConflicted),
	}
	if apierrors.IsConflict(err) {
		condition.Status = metav1.ConditionTrue
		condition.Reason = string(v1alpha1.TracingIntegrationReasonApplyConflict)
		condition.Message = fmt.Sprintf("server-side apply conflict: %v", err)
		return condition
	}

	condition.Status = metav1.ConditionFalse
	condition.Reason = string(v1alpha1.TracingIntegrationReasonNoConflict)
	return condition
}

func determineState(status v1alpha1.TracingIntegrationStatus) v1alpha1.TracingIntegrationConditionReason {
	reconciledCondition := status.GetCondition(v1alpha1.TracingIntegrationConditionReconciled)
	if reconciledCondition.Status != metav1.ConditionTrue {
		return v1alpha1.TracingIntegrationConditionReason(reconciledCondition.Reason)
	}
	conflictedCondition := status.GetCondition(v1alpha1.TracingIntegrationConditionConflicted)
	if conflictedCondition.Status == metav1.ConditionTrue {
		return v1alpha1.TracingIntegrationReasonApplyConflict
	}
	return v1alpha1.TracingIntegrationReasonHealthy
}

// SetupWithManager sets up the controller with the Manager.
func (r *TracingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	logger := mgr.GetLogger().WithName("ctrlr").WithName("tracingintegration")

	mainObjectHandler := wrapEventHandler(logger, &handler.EnqueueRequestForObject{})
	istioHandler := wrapEventHandler(logger, handler.EnqueueRequestsFromMapFunc(r.mapIstioToReconcileRequest))
	telemetryHandler := wrapEventHandler(logger,
		handler.EnqueueRequestForOwner(r.Scheme, r.RESTMapper(), &v1alpha1.TracingIntegration{}, handler.OnlyControllerOwner()))

	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			LogConstructor: func(req *reconcile.Request) logr.Logger {
				log := logger
				if req != nil {
					log = log.WithValues("tracingintegration", req.Name)
				}
				return log
			},
			MaxConcurrentReconciles: r.Config.MaxConcurrentReconciles,
		}).
		Watches(&v1alpha1.TracingIntegration{}, mainObjectHandler).
		Watches(&v1.Istio{}, istioHandler).
		Watches(&telemetryv1.Telemetry{}, telemetryHandler).
		Named("tracingintegration").
		Complete(reconciler.NewStandardReconciler[*v1alpha1.TracingIntegration](r.Client, r.Reconcile))
}

func (r *TracingReconciler) mapIstioToReconcileRequest(ctx context.Context, obj client.Object) []reconcile.Request {
	log := logf.FromContext(ctx)
	integrations := &v1alpha1.TracingIntegrationList{}
	if err := r.Client.List(ctx, integrations); err != nil {
		log.Error(err, "failed to list TracingIntegrations")
		return nil
	}

	requests := []reconcile.Request{}
	for _, tracingIntegration := range integrations.Items {
		for _, targetRef := range tracingIntegration.Spec.TargetRefs {
			if targetRef.Kind == v1.IstioKind && targetRef.Name == obj.GetName() {
				requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKey{Name: tracingIntegration.Name}})
				break
			}
		}
	}
	return requests
}

func wrapEventHandler(logger logr.Logger, handler handler.EventHandler) handler.EventHandler {
	return enqueuelogger.WrapIfNecessary(v1alpha1.TracingIntegrationKind, logger, handler)
}
