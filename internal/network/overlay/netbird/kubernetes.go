package netbird

import (
	"context"
	"strconv"
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

var routingPeerGVR = k8sschema.GroupVersionResource{Group: "netbird.io", Version: "v1", Resource: "nbroutingpeers"}

type kubernetesDriver struct {
	config  netctl.BotOverlayConfig
	runtime kubeapi.Runtime
}

func newKubernetesDriver(cfg netctl.BotOverlayConfig, runtime kubeapi.Runtime) netctl.OverlayDriver {
	return &kubernetesDriver{config: cfg, runtime: runtime}
}

func (*kubernetesDriver) Kind() string { return "netbird" }

func (d *kubernetesDriver) EnsureAttached(ctx context.Context, req netctl.AttachmentRequest) (netctl.OverlayStatus, error) {
	if d.runtime == nil {
		return netctl.OverlayStatus{Provider: d.Kind(), State: "unsupported", Message: "Kubernetes API runtime is not configured."}, nil
	}
	if !req.Overlay.Enabled || !d.config.Enabled {
		return netctl.OverlayStatus{Provider: d.Kind(), State: "disabled"}, nil
	}
	if _, err := kubeoperator.ApplyServiceAnnotations(ctx, d.runtime, req.ContainerID, d.serviceAnnotations(req)); err != nil {
		return netctl.OverlayStatus{}, err
	}
	if configutil.Bool(d.config.Config, "routing_peer_enabled") {
		if _, err := d.applyRoutingPeer(ctx, req); err != nil {
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
		"netbird.io/expose",
		"netbird.io/groups",
		"netbird.io/resource-name",
		"netbird.io/policy",
		"netbird.io/policy-ports",
	}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	name := kubeoperator.ManagedName(req.ContainerID, "netbird")
	if err := d.runtime.DeleteResource(ctx, routingPeerGVR, d.runtime.Namespace(), name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
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
	if svc.Annotations["netbird.io/expose"] != "true" {
		status.State = "starting"
		status.Message = "NetBird Service exposure annotation is not applied yet."
	}
	status.Details["service"] = svc.Name
	name := kubeoperator.ManagedName(req.ContainerID, "netbird")
	routingPeer, err := d.runtime.GetResource(ctx, routingPeerGVR, d.runtime.Namespace(), name)
	if err == nil {
		status.Details["routing_peer"] = routingPeer.GetName()
		if statusMap, ok := routingPeer.Object["status"].(map[string]any); ok {
			status.Details["routing_peer_status"] = statusMap
		}
	} else if !apierrors.IsNotFound(err) {
		status.State = "degraded"
		status.Message = err.Error()
	}
	return status, nil
}

func (d *kubernetesDriver) serviceAnnotations(req netctl.AttachmentRequest) map[string]string {
	cfg := d.config.Config
	annotations := map[string]string{
		"netbird.io/expose":        "true",
		"netbird.io/resource-name": configutil.FirstNonEmpty(configutil.String(cfg, "resource_name"), req.BotID),
	}
	if groups := configutil.String(cfg, "groups"); groups != "" {
		annotations["netbird.io/groups"] = groups
	}
	if policy := configutil.String(cfg, "policy"); policy != "" {
		annotations["netbird.io/policy"] = policy
	}
	if ports := configutil.String(cfg, "policy_ports"); ports != "" {
		annotations["netbird.io/policy-ports"] = ports
	}
	return annotations
}

func (d *kubernetesDriver) applyRoutingPeer(ctx context.Context, req netctl.AttachmentRequest) (*unstructured.Unstructured, error) {
	name := kubeoperator.ManagedName(req.ContainerID, "netbird")
	replicas := configutil.Int(d.config.Config, "routing_peer_replicas", 1)
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "netbird.io/v1",
		"kind":       "NBRoutingPeer",
		"metadata": map[string]any{
			"name":      name,
			"namespace": d.runtime.Namespace(),
			"labels":    kubeoperator.ManagedLabelMap(req.BotID, "netbird"),
		},
		"spec": map[string]any{
			"replicas": int64(replicas),
			"labels": map[string]any{
				"memoh.network.bot_id": req.BotID,
			},
		},
	}}
	if groups := splitCSV(configutil.String(d.config.Config, "groups")); len(groups) > 0 {
		_ = unstructured.SetNestedSlice(obj.Object, stringSliceToAny(groups), "spec", "groups")
	}
	if policy := strings.TrimSpace(configutil.String(d.config.Config, "policy")); policy != "" {
		_ = unstructured.SetNestedField(obj.Object, policy, "spec", "policy")
	}
	if replicas > 0 {
		_ = unstructured.SetNestedMap(obj.Object, map[string]any{"memoh.network.replicas": strconv.Itoa(replicas)}, "metadata", "annotations")
	}
	return d.runtime.ApplyResource(ctx, routingPeerGVR, d.runtime.Namespace(), obj)
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
