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
	otelv1beta1 "github.com/open-telemetry/opentelemetry-operator/apis/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
	defaultOTLPGRPCPort    = uint32(4317)
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
	if err := r.validateProvider(ctx, tracingIntegration); err != nil {
		return err
	}

	if err := r.validateNoDuplicateRefs(tracingIntegration); err != nil {
		return err
	}
	if err := r.validateTargetRefUniqueness(ctx, tracingIntegration); err != nil {
		return err
	}

	for _, targetRef := range tracingIntegration.Spec.TargetRefs {
		switch targetRef.Kind {
		case v1.IstioKind:
			if err := r.reconcileIstioTarget(ctx, tracingIntegration, targetRef.Name); err != nil {
				return err
			}
		default:
			return reconciler.NewValidationError(fmt.Sprintf("unsupported tracing target kind %q", targetRef.Kind))
		}
	}

	return nil
}

func (r *TracingReconciler) validateProvider(ctx context.Context, tracingIntegration *v1alpha1.TracingIntegration) error {
	if tracingIntegration.Spec.OpenTelemetry == nil {
		return reconciler.NewValidationError("spec.openTelemetry must be set when spec.type is OpenTelemetry")
	}

	ref := tracingIntegration.Spec.OpenTelemetry.OTELCollectorRef
	collector := &otelv1beta1.OpenTelemetryCollector{}
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name}, collector); err != nil {
		if apierrors.IsNotFound(err) || apimeta.IsNoMatchError(err) {
			// TODO: Add an E2E test covering status updates for a missing OpenTelemetryCollector.
			return reconciler.NewValidationError(fmt.Sprintf("referenced %s %s/%s was not found", otelCollectorKind, ref.Namespace, ref.Name))
		}
		return err
	}

	return nil
}

func (r *TracingReconciler) validateNoDuplicateRefs(tracingIntegration *v1alpha1.TracingIntegration) error {
	targetRefsByKind := map[string]v1alpha1.TargetReference{}
	for _, targetRef := range tracingIntegration.Spec.TargetRefs {
		if existingTargetRef, found := targetRefsByKind[targetRef.Kind]; found && existingTargetRef != targetRef {
			return reconciler.NewValidationError(fmt.Sprintf(
				"targetRefs must not contain multiple targets of kind %q", targetRef.Kind))
		}
		targetRefsByKind[targetRef.Kind] = targetRef
	}
	return nil
}

// validateTargetRefUniqueness ensures that each target reference has a single owner.
// When multiple TracingIntegrations target the same resource, the oldest one owns it;
// newer integrations receive a validation error. Creation timestamps are resolved by name.
func (r *TracingReconciler) validateTargetRefUniqueness(ctx context.Context, tracingIntegration *v1alpha1.TracingIntegration) error {
	integrations := &v1alpha1.TracingIntegrationList{}
	if err := r.Client.List(ctx, integrations); err != nil {
		return err
	}

	targetRefs := map[v1alpha1.TargetReference]struct{}{}
	for _, ref := range tracingIntegration.Spec.TargetRefs {
		// Assuming we've already validated that there's not multiple targetRefs targeting the same kind.
		targetRefs[ref] = struct{}{}
	}

	for _, integration := range integrations.Items {
		// Skip self.
		if integration.Name == tracingIntegration.Name {
			continue
		}

		for _, ref := range integration.Spec.TargetRefs {
			if _, alreadyTargeted := targetRefs[ref]; alreadyTargeted {
				// If this is already targeted, emit a validation error for the newer TracingIntegration.
				if integration.CreationTimestamp.Before(&tracingIntegration.CreationTimestamp) ||
					(integration.CreationTimestamp.Equal(&tracingIntegration.CreationTimestamp) && integration.Name < tracingIntegration.Name) {
					return reconciler.NewValidationError(fmt.Sprintf(
						"targetRef %s/%s is already managed by TracingIntegration %q", ref.Kind, ref.Name, integration.Name))
				}
			}
		}
	}

	return nil
}

func (r *TracingReconciler) reconcileIstioTarget(
	ctx context.Context, tracingIntegration *v1alpha1.TracingIntegration, istioName string,
) error {
	istio := &v1.Istio{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: istioName}, istio); err != nil {
		if apierrors.IsNotFound(err) {
			return reconciler.NewValidationError(fmt.Sprintf("target Istio %q was not found", istioName))
		}
		return err
	}

	if err := r.applyIstioTracing(ctx, istio, tracingIntegration); err != nil {
		return err
	}
	return r.applyTelemetry(ctx, tracingIntegration, istio.Spec.Namespace, tracingIntegration.Spec.OpenTelemetry.OTELCollectorRef.Name)
}

func (r *TracingReconciler) applyIstioTracing(ctx context.Context, istio *v1.Istio, tracingIntegration *v1alpha1.TracingIntegration) error {
	collectorRef := tracingIntegration.Spec.OpenTelemetry.OTELCollectorRef
	providerName := collectorRef.Name
	// TODO: Pull this from the otel collector directly if need be.
	service := fmt.Sprintf("%s-collector.%s.svc.cluster.local", collectorRef.Name, collectorRef.Namespace)

	obj := &v1.Istio{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.GroupVersion.String(),
			Kind:       v1.IstioKind,
		},
		ObjectMeta: metav1.ObjectMeta{Name: istio.Name},
		Spec: v1.IstioSpec{
			Values: &v1.Values{
				MeshConfig: &v1.MeshConfig{
					EnableTracing: new(true),
					ExtensionProviders: []*v1.MeshConfigExtensionProvider{
						{
							Name: new(providerName),
							Opentelemetry: &v1.MeshConfigExtensionProviderOpenTelemetryTracingProvider{
								Service: new(service),
								Port:    new(defaultOTLPGRPCPort),
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
	otelCollectorHandler := wrapEventHandler(logger, handler.EnqueueRequestsFromMapFunc(r.mapOTELCollectorToReconcileRequest))
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
		Watches(&otelv1beta1.OpenTelemetryCollector{}, otelCollectorHandler).
		Watches(&telemetryv1.Telemetry{}, telemetryHandler).
		Named("tracingintegration").
		Complete(reconciler.NewStandardReconciler[*v1alpha1.TracingIntegration](r.Client, r.Reconcile))
}

func (r *TracingReconciler) mapIstioToReconcileRequest(ctx context.Context, obj client.Object) []reconcile.Request {
	return r.mapIntegrations(ctx, func(tracingIntegration v1alpha1.TracingIntegration) bool {
		for _, targetRef := range tracingIntegration.Spec.TargetRefs {
			if targetRef.Kind == v1.IstioKind && targetRef.Name == obj.GetName() {
				return true
			}
		}
		return false
	})
}

func (r *TracingReconciler) mapOTELCollectorToReconcileRequest(ctx context.Context, obj client.Object) []reconcile.Request {
	return r.mapIntegrations(ctx, func(tracingIntegration v1alpha1.TracingIntegration) bool {
		if tracingIntegration.Spec.OpenTelemetry == nil {
			return false
		}
		ref := tracingIntegration.Spec.OpenTelemetry.OTELCollectorRef
		return ref.Name == obj.GetName() && ref.Namespace == obj.GetNamespace()
	})
}

func (r *TracingReconciler) mapIntegrations(
	ctx context.Context, matches func(v1alpha1.TracingIntegration) bool,
) []reconcile.Request {
	log := logf.FromContext(ctx)
	integrations := &v1alpha1.TracingIntegrationList{}
	if err := r.Client.List(ctx, integrations); err != nil {
		log.Error(err, "failed to list TracingIntegrations")
		return nil
	}

	requests := []reconcile.Request{}
	for _, tracingIntegration := range integrations.Items {
		if matches(tracingIntegration) {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKey{Name: tracingIntegration.Name}})
		}
	}
	return requests
}

func wrapEventHandler(logger logr.Logger, handler handler.EventHandler) handler.EventHandler {
	return enqueuelogger.WrapIfNecessary(v1alpha1.TracingIntegrationKind, logger, handler)
}
