package kubeoperator

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/memohai/memoh/internal/network/kubeapi"
)

const (
	LabelManaged = "memoh.network.managed"
	LabelBotID   = "memoh.network.bot_id"
	LabelKind    = "memoh.network.provider_kind"
)

func ManagedName(containerID, kind string) string {
	name := strings.TrimSpace(containerID) + "-" + strings.TrimSpace(kind)
	if len(name) <= 63 {
		return name
	}
	return name[:63]
}

func ManagedLabels(botID, kind string) map[string]string {
	return map[string]string{
		LabelManaged: "true",
		LabelBotID:   botID,
		LabelKind:    kind,
	}
}

func ManagedLabelMap(botID, kind string) map[string]any {
	return map[string]any{
		LabelManaged: "true",
		LabelBotID:   botID,
		LabelKind:    kind,
	}
}

func ApplyServiceAnnotations(ctx context.Context, runtime kubeapi.Runtime, name string, annotations map[string]string) (map[string]string, error) {
	svc, err := runtime.GetService(ctx, name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		svc = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   runtime.Namespace(),
				Annotations: map[string]string{},
				Labels: map[string]string{
					"memoh.workspace": "v3",
				},
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"memoh.bot_id": botIDFromWorkspaceName(name)},
				Ports: []corev1.ServicePort{{
					Name: "bridge",
					Port: 8080,
				}},
			},
		}
	}
	if svc.Annotations == nil {
		svc.Annotations = map[string]string{}
	}
	for key, value := range annotations {
		if strings.TrimSpace(value) == "" {
			delete(svc.Annotations, key)
			continue
		}
		svc.Annotations[key] = value
	}
	updated, err := runtime.ApplyService(ctx, svc)
	if err != nil {
		return nil, err
	}
	return updated.Annotations, nil
}

func botIDFromWorkspaceName(name string) string {
	return strings.TrimPrefix(strings.TrimSpace(name), "workspace-")
}

func RemoveServiceAnnotations(ctx context.Context, runtime kubeapi.Runtime, name string, keys []string) error {
	svc, err := runtime.GetService(ctx, name)
	if err != nil {
		return err
	}
	for _, key := range keys {
		delete(svc.Annotations, key)
	}
	_, err = runtime.UpdateService(ctx, svc)
	return err
}
