package k8s

import (
	"context"
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"

	"github.com/memohai/memoh/internal/config"
	containerapi "github.com/memohai/memoh/internal/container"
)

func TestCreateContainerCreatesPVCAndPod(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client := k8sfake.NewSimpleClientset()
	svc := testService(client, nil)
	go markPodRunning(t, client, svc.namespace(), "workspace-bot")

	_, err := svc.CreateContainer(ctx, containerapi.CreateContainerRequest{
		ID:       "workspace-bot",
		ImageRef: "memoh/workspace:latest",
		Labels: map[string]string{
			"memoh.workspace": "v3",
			"memoh.bot_id":    "bot",
		},
		Spec: containerapi.ContainerSpec{
			Cmd: []string{"/opt/memoh/bridge"},
			Env: []string{"FOO=bar"},
		},
	})
	if err != nil {
		t.Fatalf("CreateContainer returned error: %v", err)
	}

	pvc, err := client.CoreV1().PersistentVolumeClaims(svc.namespace()).Get(ctx, "workspace-bot-data", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get pvc: %v", err)
	}
	if pvc.Labels[containerapi.StorageKeyLabel] != "workspace-bot-data" {
		t.Fatalf("pvc storage label = %q", pvc.Labels[containerapi.StorageKeyLabel])
	}
	if pvc.Annotations[imageAnnotation] != "memoh/workspace:latest" {
		t.Fatalf("pvc image annotation = %q", pvc.Annotations[imageAnnotation])
	}

	pod, err := client.CoreV1().Pods(svc.namespace()).Get(ctx, "workspace-bot", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get pod: %v", err)
	}
	if pod.Labels[containerapi.StorageKeyLabel] != "workspace-bot-data" {
		t.Fatalf("pod storage label = %q", pod.Labels[containerapi.StorageKeyLabel])
	}
	if got := envValue(pod.Spec.Containers[0].Env, "BRIDGE_TCP_ADDR"); got != ":19090" {
		t.Fatalf("BRIDGE_TCP_ADDR = %q, want :19090", got)
	}
	if got := pod.Spec.Containers[0].ImagePullPolicy; got != corev1.PullIfNotPresent {
		t.Fatalf("ImagePullPolicy = %q, want %q", got, corev1.PullIfNotPresent)
	}
}

func TestCreateContainerMapsImagePullPolicy(t *testing.T) {
	tests := []struct {
		name   string
		policy string
		want   corev1.PullPolicy
	}{
		{name: "always", policy: "always", want: corev1.PullAlways},
		{name: "never", policy: "never", want: corev1.PullNever},
		{name: "default", policy: "", want: corev1.PullIfNotPresent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			client := k8sfake.NewSimpleClientset()
			svc := testService(client, nil)
			go markPodRunning(t, client, svc.namespace(), "workspace-bot")
			_, err := svc.CreateContainer(ctx, containerapi.CreateContainerRequest{
				ID:              "workspace-bot",
				ImageRef:        "memoh/workspace:latest",
				ImagePullPolicy: tt.policy,
				Spec: containerapi.ContainerSpec{
					Cmd: []string{"/opt/memoh/bridge"},
				},
			})
			if err != nil {
				t.Fatalf("CreateContainer returned error: %v", err)
			}
			pod, err := client.CoreV1().Pods(svc.namespace()).Get(ctx, "workspace-bot", metav1.GetOptions{})
			if err != nil {
				t.Fatalf("get pod: %v", err)
			}
			if got := pod.Spec.Containers[0].ImagePullPolicy; got != tt.want {
				t.Fatalf("ImagePullPolicy = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBridgeTargetReturnsPodIPAndBridgePort(t *testing.T) {
	client := k8sfake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "workspace-bot", Namespace: "test"},
		Status:     corev1.PodStatus{PodIP: "10.1.2.3"},
	})
	svc := testService(client, nil)
	if got := svc.BridgeTarget("bot"); got != "10.1.2.3:19090" {
		t.Fatalf("BridgeTarget = %q", got)
	}
}

func TestCommitSnapshotCreatesVolumeSnapshotAndWaitsReady(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "snapshot.storage.k8s.io/v1",
			"kind":       "VolumeSnapshotClass",
			"metadata": map[string]any{
				"name": "fast-snapshots",
				"annotations": map[string]any{
					"snapshot.storage.kubernetes.io/is-default-class": "true",
				},
			},
		},
	})
	client := k8sfake.NewSimpleClientset()
	svc := testService(client, dyn)
	go markSnapshotReady(t, dyn, svc.namespace(), "snap-1")

	if err := svc.CommitSnapshot(ctx, containerapi.CommitSnapshotRequest{
		Source: containerapi.StorageRef{Driver: "kubernetes", Key: "workspace-bot-data"},
		Target: containerapi.SnapshotRef{Driver: "kubernetes", Key: "snap-1"},
	}); err != nil {
		t.Fatalf("CommitSnapshot returned error: %v", err)
	}
	obj, err := dyn.Resource(volumeSnapshotsGVR).Namespace(svc.namespace()).Get(ctx, "snap-1", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get volume snapshot: %v", err)
	}
	source, _, _ := unstructured.NestedString(obj.Object, "spec", "source", "persistentVolumeClaimName")
	if source != "workspace-bot-data" {
		t.Fatalf("snapshot source pvc = %q", source)
	}
}

func TestPrepareSnapshotCreatesPVCFromVolumeSnapshot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client := k8sfake.NewSimpleClientset()
	svc := testService(client, nil)
	go markPVCBound(t, client, svc.namespace(), "workspace-active-1")

	if err := svc.PrepareSnapshot(ctx, containerapi.PrepareSnapshotRequest{
		Target: containerapi.StorageRef{Driver: "kubernetes", Key: "workspace-active-1"},
		Parent: containerapi.SnapshotRef{Driver: "kubernetes", Key: "snap-1"},
	}); err != nil {
		t.Fatalf("PrepareSnapshot returned error: %v", err)
	}
	pvc, err := client.CoreV1().PersistentVolumeClaims(svc.namespace()).Get(ctx, "workspace-active-1", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get pvc: %v", err)
	}
	if pvc.Spec.DataSource == nil || pvc.Spec.DataSource.Name != "snap-1" {
		t.Fatalf("PVC DataSource = %#v, want VolumeSnapshot snap-1", pvc.Spec.DataSource)
	}
}

func TestKubernetesDoesNotExposeHostSnapshotCapabilities(t *testing.T) {
	type snapshotMountProvider interface {
		SnapshotMounts(context.Context, string, string) ([]containerapi.MountInfo, error)
	}
	var svc any = NewService(config.Config{})
	if _, ok := svc.(snapshotMountProvider); ok {
		t.Fatal("kubernetes service should not expose host-side snapshot mounts")
	}
}

func TestGetContainerMetricsUsesMetricsAPIWhenAvailable(t *testing.T) {
	ctx := context.Background()
	client := k8sfake.NewSimpleClientset()
	metrics := metricsfake.NewSimpleClientset()
	metrics.PrependReactor("get", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &metricsv1beta1.PodMetrics{
			TypeMeta:   metav1.TypeMeta{APIVersion: "metrics.k8s.io/v1beta1", Kind: "PodMetrics"},
			ObjectMeta: metav1.ObjectMeta{Name: "workspace-bot", Namespace: "test"},
			Timestamp:  metav1.NewTime(time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)),
			Containers: []metricsv1beta1.ContainerMetrics{
				{
					Name: workspaceContainer,
					Usage: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("125m"),
						corev1.ResourceMemory: resource.MustParse("64Mi"),
					},
				},
			},
		}, nil
	})
	svc := testService(client, nil, metrics)

	got, err := svc.GetContainerMetrics(ctx, "workspace-bot")
	if err != nil {
		t.Fatalf("GetContainerMetrics returned error: %v", err)
	}
	if got.CPU == nil || got.CPU.UsageNanocores != 125_000_000 {
		t.Fatalf("CPU metrics = %+v, want 125000000 nanocores", got.CPU)
	}
	if got.Memory == nil || got.Memory.UsageBytes != 64*1024*1024 {
		t.Fatalf("Memory metrics = %+v, want 64Mi", got.Memory)
	}
	if got.SampledAt.IsZero() {
		t.Fatal("SampledAt should be set")
	}
}

func TestGetContainerMetricsIsUnsupportedWithoutMetricsAPI(t *testing.T) {
	svc := testService(k8sfake.NewSimpleClientset(), nil)
	_, err := svc.GetContainerMetrics(context.Background(), "workspace-bot")
	if !errors.Is(err, containerapi.ErrNotSupported) {
		t.Fatalf("GetContainerMetrics error = %v, want ErrNotSupported", err)
	}
}

func testService(client *k8sfake.Clientset, dyn *dynamicfake.FakeDynamicClient, metrics ...metricsclient.Interface) *Service {
	if dyn == nil {
		dyn = dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	}
	var metricsClient metricsclient.Interface
	if len(metrics) > 0 {
		metricsClient = metrics[0]
	}
	return &Service{
		cfg: config.Config{Kubernetes: config.KubernetesConfig{
			Namespace:  "test",
			PVCSize:    "1Gi",
			BridgePort: 19090,
		}},
		client:  client,
		dynamic: dyn,
		metrics: metricsClient,
	}
}

func markPodRunning(t *testing.T, client *k8sfake.Clientset, namespace, name string) {
	t.Helper()
	for {
		pod, err := client.CoreV1().Pods(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err == nil {
			pod.Status.Phase = corev1.PodRunning
			pod.Status.PodIP = "10.1.2.3"
			pod.Status.ContainerStatuses = []corev1.ContainerStatus{
				{Name: workspaceContainer, Ready: true},
			}
			_, _ = client.CoreV1().Pods(namespace).UpdateStatus(context.Background(), pod, metav1.UpdateOptions{})
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func markPVCBound(t *testing.T, client *k8sfake.Clientset, namespace, name string) {
	t.Helper()
	for {
		pvc, err := client.CoreV1().PersistentVolumeClaims(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err == nil {
			pvc.Status.Phase = corev1.ClaimBound
			_, _ = client.CoreV1().PersistentVolumeClaims(namespace).UpdateStatus(context.Background(), pvc, metav1.UpdateOptions{})
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func markSnapshotReady(t *testing.T, dyn *dynamicfake.FakeDynamicClient, namespace, name string) {
	t.Helper()
	for {
		obj, err := dyn.Resource(volumeSnapshotsGVR).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err == nil {
			_ = unstructured.SetNestedField(obj.Object, true, "status", "readyToUse")
			_, _ = dyn.Resource(volumeSnapshotsGVR).Namespace(namespace).Update(context.Background(), obj, metav1.UpdateOptions{})
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func envValue(env []corev1.EnvVar, name string) string {
	for _, item := range env {
		if item.Name == name {
			return item.Value
		}
	}
	return ""
}
