package tailscale

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"

	netctl "github.com/memohai/memoh/internal/network"
)

type fakeKubeRuntime struct {
	namespace string
	services  map[string]*corev1.Service
	resources map[string]*unstructured.Unstructured
}

func newFakeKubeRuntime() *fakeKubeRuntime {
	return &fakeKubeRuntime{
		namespace: "memoh",
		services:  map[string]*corev1.Service{},
		resources: map[string]*unstructured.Unstructured{},
	}
}

func (f *fakeKubeRuntime) Namespace() string { return f.namespace }

func (f *fakeKubeRuntime) GetService(_ context.Context, name string) (*corev1.Service, error) {
	if svc, ok := f.services[name]; ok {
		return svc.DeepCopy(), nil
	}
	return nil, apierrors.NewNotFound(k8sschema.GroupResource{Group: "", Resource: "services"}, name)
}

func (f *fakeKubeRuntime) ApplyService(_ context.Context, svc *corev1.Service) (*corev1.Service, error) {
	f.services[svc.Name] = svc.DeepCopy()
	return svc.DeepCopy(), nil
}

func (f *fakeKubeRuntime) UpdateService(_ context.Context, svc *corev1.Service) (*corev1.Service, error) {
	f.services[svc.Name] = svc.DeepCopy()
	return svc.DeepCopy(), nil
}

func (f *fakeKubeRuntime) GetResource(_ context.Context, _ k8sschema.GroupVersionResource, _ string, name string) (*unstructured.Unstructured, error) {
	if obj, ok := f.resources[name]; ok {
		return obj.DeepCopy(), nil
	}
	return nil, apierrors.NewNotFound(k8sschema.GroupResource{Group: "tailscale.com", Resource: "connectors"}, name)
}

func (f *fakeKubeRuntime) ApplyResource(_ context.Context, _ k8sschema.GroupVersionResource, _ string, obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	f.resources[obj.GetName()] = obj.DeepCopy()
	return obj.DeepCopy(), nil
}

func (f *fakeKubeRuntime) DeleteResource(_ context.Context, _ k8sschema.GroupVersionResource, _ string, name string, _ metav1.DeleteOptions) error {
	delete(f.resources, name)
	return nil
}

func (*fakeKubeRuntime) ListResources(context.Context, k8sschema.GroupVersionResource, string, metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return &unstructured.UnstructuredList{}, nil
}

func TestKubernetesDriverEnsureAttachedAppliesServiceAndConnector(t *testing.T) {
	rt := newFakeKubeRuntime()
	driver := newKubernetesDriver(netctl.BotOverlayConfig{
		Enabled:  true,
		Provider: "tailscale",
		Config: map[string]any{
			"hostname":         "bot-tail",
			"tags":             "tag:memoh",
			"connector_routes": "10.0.0.0/24",
			"expose_service":   true,
		},
	}, rt)
	status, err := driver.EnsureAttached(context.Background(), netctl.AttachmentRequest{
		BotID:       "bot-1",
		ContainerID: "workspace-bot-1",
		Overlay:     netctl.BotOverlayConfig{Enabled: true, Provider: "tailscale"},
	})
	if err != nil {
		t.Fatalf("EnsureAttached returned error: %v", err)
	}
	if status.Provider != "tailscale" {
		t.Fatalf("unexpected status: %+v", status)
	}
	svc := rt.services["workspace-bot-1"]
	if svc == nil || svc.Annotations["tailscale.com/hostname"] != "bot-tail" {
		t.Fatalf("expected tailscale service annotations, got %+v", svc)
	}
	if _, ok := rt.resources["workspace-bot-1-tailscale"]; !ok {
		t.Fatalf("expected connector to be applied")
	}
}
