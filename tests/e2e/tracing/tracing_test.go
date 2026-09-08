//go:build e2e

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

package tracing

import (
	"context"
	"fmt"
	"time"

	"github.com/istio-ecosystem/sail-operator/api/v1alpha1"
	"github.com/istio-ecosystem/sail-operator/pkg/env"
	"github.com/istio-ecosystem/sail-operator/pkg/kube"
	"github.com/istio-ecosystem/sail-operator/pkg/test/project"
	. "github.com/istio-ecosystem/sail-operator/pkg/test/util/ginkgo"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	otelCollectorCRD = "opentelemetrycollectors.opentelemetry.io"
	telemetryCRD     = "telemetries.telemetry.istio.io"
	integrationName  = "e2e-tracing-controller"

	otelOperatorNamespace  = "opentelemetry-operator-system"
	otelOperatorDeployment = "opentelemetry-operator-controller-manager"
	certManagerNamespace   = "cert-manager"
)

var _ = Describe("Tracing integration controller", Label("tracing", "integration"), Ordered, Serial, func() {
	var (
		certManagerManifest   string
		otelManifest          string
		integration           *v1alpha1.TracingIntegration
		certManagerInstalled  bool
		otelOperatorInstalled bool
	)

	BeforeAll(func(ctx SpecContext) {
		certManagerVersion := env.Get("CERT_MANAGER_VERSION", "v1.17.2")
		otelOperatorVersion := env.Get("OTEL_OPERATOR_VERSION", "v0.158.0")
		certManagerManifest = fmt.Sprintf(
			"https://github.com/cert-manager/cert-manager/releases/download/%s/cert-manager.yaml", certManagerVersion)
		otelManifest = fmt.Sprintf(
			"https://github.com/open-telemetry/opentelemetry-operator/releases/download/%s/opentelemetry-operator.yaml",
			otelOperatorVersion)

		By("removing the Istio Telemetry CRD before restarting the Sail Operator")
		Expect(k.DeleteIgnoreNotFound("crd", telemetryCRD)).To(Succeed())
		Eventually(crdIsNotFound).WithArguments(ctx, telemetryCRD).Should(BeTrue())
		Expect(crdIsNotFound(ctx, otelCollectorCRD)).To(BeTrue(),
			"the test requires the OpenTelemetry operator not to be installed initially")

		// Restore everything changed by this suite even when a later setup step fails.
		DeferCleanup(func(ctx SpecContext) {
			if integration != nil {
				Expect(client.IgnoreNotFound(cl.Delete(ctx, integration))).To(Succeed())
			}
			if otelOperatorInstalled {
				Expect(k.DeleteFromFile(otelManifest)).To(Succeed())
			}
			if certManagerInstalled {
				Expect(k.DeleteFromFile(certManagerManifest)).To(Succeed())
			}
			Expect(k.Apply(project.RootDir + "/chart/crds/telemetry.istio.io_telemetries.yaml")).To(Succeed())
			Eventually(crdIsEstablished).WithArguments(ctx, telemetryCRD).Should(BeTrue())
		})

		By("restarting the Sail Operator with both optional CRDs absent")
		_, err := k.WithNamespace(operatorNamespace).RolloutRestart("deployment/" + operatorDeployment)
		Expect(err).NotTo(HaveOccurred())
		Eventually(deploymentIsRolledOut).WithArguments(ctx, operatorNamespace, operatorDeployment).Should(BeTrue())

		By("creating a TracingIntegration while its controller is waiting for the CRDs")
		integration = &v1alpha1.TracingIntegration{
			ObjectMeta: metav1.ObjectMeta{Name: integrationName},
			Spec: v1alpha1.TracingIntegrationSpec{
				TargetRefs: []v1alpha1.TargetReference{{Kind: "Istio", Name: "default"}},
				TracingConfig: v1alpha1.TracingConfig{
					Type: v1alpha1.TracingTypeOpenTelemetry,
					OpenTelemetry: &v1alpha1.OpenTelemetryConfig{
						OTELCollectorRef: v1alpha1.NamespacedReference{Name: "missing", Namespace: "istio-system"},
					},
				},
			},
		}
		Expect(cl.Create(ctx, integration)).To(Succeed())
	})

	It("runs without error while the OpenTelemetry Collector and Istio Telemetry CRDs are absent", func(ctx SpecContext) {
		Consistently(func(g Gomega) {
			g.Expect(crdIsNotFound(ctx, otelCollectorCRD)).To(BeTrue())
			g.Expect(crdIsNotFound(ctx, telemetryCRD)).To(BeTrue())
			g.Expect(deploymentIsRolledOut(ctx, operatorNamespace, operatorDeployment)).To(BeTrue())

			actual := &v1alpha1.TracingIntegration{}
			g.Expect(cl.Get(ctx, kube.Key(integrationName), actual)).To(Succeed())
			g.Expect(actual.Status.ObservedGeneration).To(BeZero(), "the controller must still be waiting for its CRDs")
		}).WithTimeout(10 * time.Second).WithPolling(time.Second).Should(Succeed())
		Success("The Sail Operator remains healthy while the tracing controller waits for its CRDs")
	})

	It("continues waiting when only the OpenTelemetry Collector CRD is installed", func(ctx SpecContext) {
		By("installing cert-manager, which the OpenTelemetry Operator webhooks require")
		Expect(k.Apply(certManagerManifest)).To(Succeed())
		certManagerInstalled = true
		Eventually(deploymentIsRolledOut).WithArguments(ctx, certManagerNamespace, "cert-manager-webhook").Should(BeTrue())

		By("installing the OpenTelemetry Operator")
		Expect(k.Apply(otelManifest)).To(Succeed())
		otelOperatorInstalled = true
		Eventually(crdIsEstablished).WithArguments(ctx, otelCollectorCRD).Should(BeTrue())
		Eventually(deploymentIsRolledOut).
			WithArguments(ctx, otelOperatorNamespace, otelOperatorDeployment).Should(BeTrue())

		Consistently(func(g Gomega) {
			actual := &v1alpha1.TracingIntegration{}
			g.Expect(cl.Get(ctx, kube.Key(integrationName), actual)).To(Succeed())
			g.Expect(actual.Status.ObservedGeneration).To(BeZero(),
				"the controller must wait until the Istio Telemetry CRD is also installed")
		}).WithTimeout(10 * time.Second).WithPolling(time.Second).Should(Succeed())
	})

	It("starts reconciling TracingIntegrations after both CRDs are installed", func(ctx SpecContext) {
		By("installing the Istio Telemetry CRD")
		Expect(k.Apply(project.RootDir + "/chart/crds/telemetry.istio.io_telemetries.yaml")).To(Succeed())
		Eventually(crdIsEstablished).WithArguments(ctx, telemetryCRD).Should(BeTrue())

		By("waiting for the pre-existing TracingIntegration to be reconciled")
		Eventually(func(g Gomega) {
			actual := &v1alpha1.TracingIntegration{}
			g.Expect(cl.Get(ctx, kube.Key(integrationName), actual)).To(Succeed())
			g.Expect(actual.Status.ObservedGeneration).To(Equal(actual.Generation))
			g.Expect(actual.Status.State).To(Equal(v1alpha1.TracingIntegrationReasonInvalidConfiguration))
			condition := actual.Status.GetCondition(v1alpha1.TracingIntegrationConditionReconciled)
			g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(condition.Message).To(ContainSubstring("referenced OpenTelemetryCollector istio-system/missing was not found"))
		}).Should(Succeed())
		Success("The tracing controller began reconciling after both CRDs became available")
	})
})

func crdIsNotFound(ctx context.Context, name string) bool {
	crd := &apiextensionsv1.CustomResourceDefinition{}
	return apierrors.IsNotFound(cl.Get(ctx, client.ObjectKey{Name: name}, crd))
}

func crdIsEstablished(ctx context.Context, name string) bool {
	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := cl.Get(ctx, client.ObjectKey{Name: name}, crd); err != nil {
		return false
	}
	for _, condition := range crd.Status.Conditions {
		if condition.Type == apiextensionsv1.Established && condition.Status == apiextensionsv1.ConditionTrue {
			return true
		}
	}
	return false
}

func deploymentIsRolledOut(ctx context.Context, namespace, name string) bool {
	deployment := &appsv1.Deployment{}
	if err := cl.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, deployment); err != nil {
		return false
	}
	desired := int32(1)
	if deployment.Spec.Replicas != nil {
		desired = *deployment.Spec.Replicas
	}
	return deployment.Status.ObservedGeneration == deployment.Generation &&
		deployment.Status.UpdatedReplicas == desired &&
		deployment.Status.AvailableReplicas == desired
}
