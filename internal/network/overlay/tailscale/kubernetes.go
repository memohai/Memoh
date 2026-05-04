package tailscale

import (
	"context"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"

	netctl "github.com/memohai/memoh/internal/network"
	"github.com/memohai/memoh/internal/network/kubeapi"
	"github.com/memohai/memoh/internal/network/overlay/internal/configutil"
	"github.com/memohai/memoh/internal/network/overlay/internal/kubeoperator"
)

var connectorGVR = k8sschema.GroupVersionResource{Group: "tailscale.com", Version: "v1alpha1", Resource: "connectors"}

type kubernetesDriver struct {
	config  netctl.BotOverlayConfig
	runtime kubeapi.Runtime
}

func newKubernetesDriver(cfg netctl.BotOverlayConfig, runtime kubeapi.Runtime) netctl.OverlayDriver {
	return &kubernetesDriver{config: cfg, runtime: runtime}
}

func (*kubernetesDriver) Kind() string { return "tailscale" }

func (d *kubernetesDriver) EnsureAttached(ctx context.Context, req netctl.AttachmentRequest) (netctl.OverlayStatus, error) {
	if d.runtime == nil {
		return netctl.OverlayStatus{Provider: d.Kind(), State: "unsupported", Message: "Kubernetes API runtime is not configured."}, nil
	}
	if !req.Overlay.Enabled || !d.config.Enabled {
		return netctl.OverlayStatus{Provider: d.Kind(), State: "disabled"}, nil
	}
	if configutil.Bool(d.config.Config, "expose_service", true) {
		if _, err := kubeoperator.ApplyServiceAnnotations(ctx, d.runtime, req.ContainerID, d.serviceAnnotations(req)); err != nil {
			return netctl.OverlayStatus{}, err
		}
	}
	if routes := splitCSV(configutil.String(d.config.Config, "connector_routes")); len(routes) > 0 {
		if _, err := d.applyConnector(ctx, req, routes); err != nil {
			return netctl.OverlayStatus{}, err
		}
	}
	return d.Status(ctx, req)
}

func (d *kubernetesDriver) Detach(ctx context.Context, req netctl.AttachmentRequest) error {
	if d.runtime == nil {
		return nil
	}
	if err := kubeoperator.RemoveServiceAnnotations(ctx, d.runtime, req.ContainerID, []string{
		"tailscale.com/expose",
		"tailscale.com/hostname",
		"tailscale.com/tags",
		"tailscale.com/proxy-class",
	}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	name := kubeoperator.ManagedName(req.ContainerID, "tailscale")
	if err := d.runtime.DeleteResource(ctx, connectorGVR, d.runtime.Namespace(), name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (d *kubernetesDriver) Status(ctx context.Context, req netctl.AttachmentRequest) (netctl.OverlayStatus, error) {
	if d.runtime == nil {
		return netctl.OverlayStatus{Provider: d.Kind(), State: "unsupported", Message: "Kubernetes API runtime is not configured."}, nil
	}
	status := netctl.OverlayStatus{
		Provider: d.Kind(),
		State:    "ready",
		Attached: true,
		Details: map[string]any{
			"kubernetes_native": true,
		},
	}
	svc, err := d.runtime.GetService(ctx, req.ContainerID)
	if err != nil {
		if apierrors.IsNotFound(err) {
			status.State = "missing"
			status.Attached = false
			status.Message = "workspace Service is not created"
			return status, nil
		}
		return netctl.OverlayStatus{}, err
	}
	if svc.Annotations["tailscale.com/expose"] != "true" {
		status.State = "starting"
		status.Message = "Tailscale Service exposure annotation is not applied yet."
	}
	status.Details["service"] = svc.Name
	name := kubeoperator.ManagedName(req.ContainerID, "tailscale")
	connector, err := d.runtime.GetResource(ctx, connectorGVR, d.runtime.Namespace(), name)
	if err == nil {
		status.Details["connector"] = connector.GetName()
		if statusMap, ok := connector.Object["status"].(map[string]any); ok {
			if conditions, ok := statusMap["conditions"]; ok {
				status.Details["connector_conditions"] = conditions
			}
		}
	} else if !apierrors.IsNotFound(err) {
		status.State = "degraded"
		status.Message = err.Error()
	}
	return status, nil
}

func (d *kubernetesDriver) serviceAnnotations(req netctl.AttachmentRequest) map[string]string {
	cfg := d.config.Config
	annotations := map[string]string{"tailscale.com/expose": "true"}
	if hostname := configutil.FirstNonEmpty(configutil.String(cfg, "hostname"), req.BotID); hostname != "" {
		annotations["tailscale.com/hostname"] = hostname
	}
	if tags := configutil.String(cfg, "tags"); tags != "" {
		annotations["tailscale.com/tags"] = tags
	}
	if proxyClass := configutil.String(cfg, "proxy_class"); proxyClass != "" {
		annotations["tailscale.com/proxy-class"] = proxyClass
	}
	return annotations
}

func (d *kubernetesDriver) applyConnector(ctx context.Context, req netctl.AttachmentRequest, routes []string) (*unstructured.Unstructured, error) {
	name := kubeoperator.ManagedName(req.ContainerID, "tailscale")
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "tailscale.com/v1alpha1",
		"kind":       "Connector",
		"metadata": map[string]any{
			"name":      name,
			"namespace": d.runtime.Namespace(),
			"labels":    kubeoperator.ManagedLabelMap(req.BotID, "tailscale"),
		},
		"spec": map[string]any{
			"hostname": configutil.FirstNonEmpty(configutil.String(d.config.Config, "hostname"), req.BotID),
			"routes":   stringSliceToAny(routes),
		},
	}}
	return d.runtime.ApplyResource(ctx, connectorGVR, d.runtime.Namespace(), obj)
}

func stringSliceToAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func splitCSV(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' })
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
