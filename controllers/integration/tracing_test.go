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
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/istio-ecosystem/sail-operator/pkg/scheme"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	toolscache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func tracingTestCRDs() []client.Object {
	var objects []client.Object
	for name, version := range map[string]string{
		"opentelemetrycollectors.opentelemetry.io": "v1beta1",
		"telemetries.telemetry.istio.io":           "v1",
	} {
		objects = append(objects, &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: apiextensionsv1.CustomResourceDefinitionSpec{
				Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{Name: version, Served: true}},
			},
			Status: apiextensionsv1.CustomResourceDefinitionStatus{
				Conditions: []apiextensionsv1.CustomResourceDefinitionCondition{{
					Type: apiextensionsv1.Established, Status: apiextensionsv1.ConditionTrue,
				}},
			},
		})
	}
	return objects
}

func TestCRDsReady(t *testing.T) {
	for _, tc := range []struct {
		name   string
		modify func([]client.Object) []client.Object
		want   bool
	}{
		{name: "ready", want: true},
		{name: "missing", modify: func(objects []client.Object) []client.Object { return objects[:1] }},
		{name: "not established", modify: func(objects []client.Object) []client.Object {
			objects[0].(*apiextensionsv1.CustomResourceDefinition).Status.Conditions = nil
			return objects
		}},
		{name: "not served", modify: func(objects []client.Object) []client.Object {
			objects[0].(*apiextensionsv1.CustomResourceDefinition).Spec.Versions[0].Served = false
			return objects
		}},
		{name: "any served version", want: true, modify: func(objects []client.Object) []client.Object {
			objects[0].(*apiextensionsv1.CustomResourceDefinition).Spec.Versions[0].Name = "v99"
			return objects
		}},
		{name: "terminating", modify: func(objects []client.Object) []client.Object {
			objects[0].SetFinalizers([]string{"test"})
			objects[0].SetDeletionTimestamp(new(metav1.Now()))
			return objects
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			objects := tracingTestCRDs()
			if tc.modify != nil {
				objects = tc.modify(objects)
			}
			reader := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(objects...).Build()
			names := []string{"opentelemetrycollectors.opentelemetry.io", "telemetries.telemetry.istio.io"}
			ready, err := crdsReady(t.Context(), reader, names)
			if err != nil || ready != tc.want {
				t.Fatalf("ready = %v, err = %v; want %v", ready, err, tc.want)
			}
		})
	}
}

func TestWaitForCRDsEmpty(t *testing.T) {
	if err := waitForCRDs(t.Context(), nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestWaitForCRDsCustomNames(t *testing.T) {
	obj := tracingTestCRDs()[0]
	obj.SetName("examples.example.com")
	cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(obj).Build()
	informer := &tracingTestInformer{handlers: make(chan toolscache.ResourceEventHandler, 1)}
	crdCache := &tracingTestCache{Reader: cl, informer: informer}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := waitForCRDs(ctx, crdCache, []string{obj.GetName()}); err != nil {
		t.Fatal(err)
	}
	if !informer.removed {
		t.Error("handler was not removed")
	}
}

type tracingTestInformer struct {
	cache.Informer
	handlers chan toolscache.ResourceEventHandler
	removed  bool
}

func (i *tracingTestInformer) AddEventHandler(handler toolscache.ResourceEventHandler) (toolscache.ResourceEventHandlerRegistration, error) {
	i.handlers <- handler
	return nil, nil
}

func (i *tracingTestInformer) RemoveEventHandler(toolscache.ResourceEventHandlerRegistration) error {
	i.removed = true
	return nil
}

type tracingTestCache struct {
	cache.Cache
	client.Reader
	informer *tracingTestInformer
}

func (c *tracingTestCache) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return c.Reader.Get(ctx, key, obj, opts...)
}

func (c *tracingTestCache) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return c.Reader.List(ctx, list, opts...)
}

func (c *tracingTestCache) GetInformer(context.Context, client.Object, ...cache.InformerGetOption) (cache.Informer, error) {
	return c.informer, nil
}

func TestStartTracingController(t *testing.T) {
	for _, mode := range []string{"already ready", "added", "updated", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			objects := tracingTestCRDs()
			cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
			if mode == "already ready" || mode == "updated" {
				for _, obj := range objects {
					initial := obj.DeepCopyObject().(client.Object)
					if mode == "updated" {
						initial.(*apiextensionsv1.CustomResourceDefinition).Spec.Versions[0].Served = false
					}
					if err := cl.Create(t.Context(), initial); err != nil {
						t.Fatal(err)
					}
				}
			}
			informer := &tracingTestInformer{handlers: make(chan toolscache.ResourceEventHandler, 1)}
			crdCache := &tracingTestCache{Reader: cl, informer: informer}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			wantErr := errors.New("controller stopped")
			starts := 0
			c := manager.RunnableFunc(func(gotCtx context.Context) error {
				starts++
				if gotCtx != ctx {
					t.Error("controller did not receive manager context")
				}
				if !informer.removed {
					t.Error("handler was not removed before controller startup")
				}
				ready, err := crdsReady(ctx, cl, []string{"opentelemetrycollectors.opentelemetry.io", "telemetries.telemetry.istio.io"})
				if err != nil || !ready {
					t.Error("controller started before both CRDs were ready")
				}
				return wantErr
			})
			done := make(chan error, 1)
			go func() { done <- startTracingController(ctx, crdCache, c, logr.Discard()) }()
			var handler toolscache.ResourceEventHandler
			select {
			case handler = <-informer.handlers:
			case <-ctx.Done():
				t.Fatal("handler was not registered")
			}
			switch mode {
			case "added", "updated":
				for _, obj := range objects {
					if mode == "added" {
						if err := cl.Create(ctx, obj); err != nil {
							t.Fatal(err)
						}
						handler.OnAdd(obj, false)
					} else {
						current := &apiextensionsv1.CustomResourceDefinition{}
						if err := cl.Get(ctx, client.ObjectKeyFromObject(obj), current); err != nil {
							t.Fatal(err)
						}
						old := current.DeepCopy()
						current.Spec.Versions[0].Served = true
						if err := cl.Update(ctx, current); err != nil {
							t.Fatal(err)
						}
						handler.OnUpdate(old, current)
					}
				}
			case "canceled":
				cancel()
			}
			select {
			case err := <-done:
				if mode == "canceled" {
					if err != nil || starts != 0 {
						t.Fatalf("err = %v, starts = %d; want nil and no starts", err, starts)
					}
				} else if !errors.Is(err, wantErr) || starts != 1 {
					t.Fatalf("err = %v, starts = %d; want controller error and one start", err, starts)
				}
				if !informer.removed {
					t.Error("handler was not removed")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("startup did not finish")
			}
		})
	}
}
