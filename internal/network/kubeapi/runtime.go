package kubeapi

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Runtime is the narrow Kubernetes API surface network overlay drivers need.
type Runtime interface {
	Namespace() string
	GetService(ctx context.Context, name string) (*corev1.Service, error)
	ApplyService(ctx context.Context, svc *corev1.Service) (*corev1.Service, error)
	UpdateService(ctx context.Context, svc *corev1.Service) (*corev1.Service, error)
	GetResource(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error)
	ApplyResource(ctx context.Context, gvr schema.GroupVersionResource, namespace string, obj *unstructured.Unstructured) (*unstructured.Unstructured, error)
	DeleteResource(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error
	ListResources(ctx context.Context, gvr schema.GroupVersionResource, namespace string, opts metav1.ListOptions) (*unstructured.UnstructuredList, error)
}
