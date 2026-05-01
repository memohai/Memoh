package k8s

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"

	"github.com/memohai/memoh/internal/config"
	containerapi "github.com/memohai/memoh/internal/container"
)

const (
	runtimeName          = "kubernetes"
	workspaceContainer   = "workspace"
	dataVolumeName       = "workspace-data"
	dataMountPath        = "/data"
	defaultWorkspacePref = "workspace-"
	imageAnnotation      = "memoh.image"
	snapshotParentLabel  = "memoh.snapshot_parent"
)

var volumeSnapshotsGVR = schema.GroupVersionResource{
	Group:    "snapshot.storage.k8s.io",
	Version:  "v1",
	Resource: "volumesnapshots",
}

var volumeSnapshotClassesGVR = schema.GroupVersionResource{
	Group:    "snapshot.storage.k8s.io",
	Version:  "v1",
	Resource: "volumesnapshotclasses",
}

type Service struct {
	cfg       config.Config
	mu        sync.Mutex
	client    kubernetes.Interface
	dynamic   dynamic.Interface
	metrics   metricsclient.Interface
	restCfg   *rest.Config
	clientErr error
	inCluster bool
	pfMu      sync.Mutex
	pfStops   map[string]chan struct{}
}

func NewService(cfg config.Config) *Service {
	return &Service{cfg: cfg}
}

func (*Service) PullImage(context.Context, string, *containerapi.PullImageOptions) (containerapi.ImageInfo, error) {
	return containerapi.ImageInfo{}, containerapi.ErrNotSupported
}

func (*Service) GetImage(context.Context, string) (containerapi.ImageInfo, error) {
	return containerapi.ImageInfo{}, containerapi.ErrNotSupported
}

func (*Service) ListImages(context.Context) ([]containerapi.ImageInfo, error) {
	return nil, containerapi.ErrNotSupported
}

func (*Service) DeleteImage(context.Context, string, *containerapi.DeleteImageOptions) error {
	return containerapi.ErrNotSupported
}

func (*Service) ResolveRemoteDigest(context.Context, string) (string, error) {
	return "", containerapi.ErrNotSupported
}

func (s *Service) CreateContainer(ctx context.Context, req containerapi.CreateContainerRequest) (containerapi.ContainerInfo, error) {
	if strings.TrimSpace(req.ID) == "" || strings.TrimSpace(req.ImageRef) == "" {
		return containerapi.ContainerInfo{}, containerapi.ErrInvalidArgument
	}
	if req.Spec.NetworkJoinTarget.Value != "" || len(req.Spec.AddedCapabilities) > 0 || len(req.Spec.CDIDevices) > 0 {
		return containerapi.ContainerInfo{}, containerapi.ErrNotSupported
	}
	if err := s.ensureClients(); err != nil {
		return containerapi.ContainerInfo{}, err
	}
	namespace := s.namespace()
	pvcName := strings.TrimSpace(req.StorageRef.Key)
	if pvcName == "" {
		pvcName = dataPVCName(req.ID)
		if err := s.ensurePVC(ctx, namespace, pvcName, req); err != nil {
			return containerapi.ContainerInfo{}, err
		}
	}
	if err := s.ensurePod(ctx, namespace, pvcName, req); err != nil {
		return containerapi.ContainerInfo{}, err
	}
	return s.GetContainer(ctx, req.ID)
}

func (s *Service) GetContainer(ctx context.Context, id string) (containerapi.ContainerInfo, error) {
	if strings.TrimSpace(id) == "" {
		return containerapi.ContainerInfo{}, containerapi.ErrInvalidArgument
	}
	if err := s.ensureClients(); err != nil {
		return containerapi.ContainerInfo{}, err
	}
	pod, err := s.client.CoreV1().Pods(s.namespace()).Get(ctx, id, metav1.GetOptions{})
	if err != nil {
		return containerapi.ContainerInfo{}, mapK8sErr(err)
	}
	return podToContainerInfo(pod), nil
}

func (s *Service) ListContainers(ctx context.Context) ([]containerapi.ContainerInfo, error) {
	if err := s.ensureClients(); err != nil {
		return nil, err
	}
	pods, err := s.client.CoreV1().Pods(s.namespace()).List(ctx, metav1.ListOptions{
		LabelSelector: "memoh.workspace",
	})
	if err != nil {
		return nil, mapK8sErr(err)
	}
	out := make([]containerapi.ContainerInfo, 0, len(pods.Items))
	for i := range pods.Items {
		out = append(out, podToContainerInfo(&pods.Items[i]))
	}
	return out, nil
}

func (s *Service) DeleteContainer(ctx context.Context, id string, opts *containerapi.DeleteContainerOptions) error {
	if strings.TrimSpace(id) == "" {
		return containerapi.ErrInvalidArgument
	}
	if err := s.ensureClients(); err != nil {
		return err
	}
	namespace := s.namespace()
	propagation := metav1.DeletePropagationBackground
	err := s.client.CoreV1().Pods(namespace).Delete(ctx, id, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
	if err != nil && !apierrors.IsNotFound(err) {
		return mapK8sErr(err)
	}
	if opts != nil && opts.CleanupSnapshot {
		if err := s.client.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, dataPVCName(id), metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return mapK8sErr(err)
		}
	}
	return nil
}

func (s *Service) ListContainersByLabel(ctx context.Context, key, value string) ([]containerapi.ContainerInfo, error) {
	if strings.TrimSpace(key) == "" {
		return nil, containerapi.ErrInvalidArgument
	}
	if err := s.ensureClients(); err != nil {
		return nil, err
	}
	selector := strings.TrimSpace(key)
	if strings.TrimSpace(value) != "" {
		selector += "=" + strings.TrimSpace(value)
	}
	pods, err := s.client.CoreV1().Pods(s.namespace()).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, mapK8sErr(err)
	}
	out := make([]containerapi.ContainerInfo, 0, len(pods.Items))
	for i := range pods.Items {
		out = append(out, podToContainerInfo(&pods.Items[i]))
	}
	return out, nil
}

func (s *Service) RestoreContainer(ctx context.Context, req containerapi.CreateContainerRequest) (containerapi.ContainerInfo, error) {
	if strings.TrimSpace(req.ID) == "" || strings.TrimSpace(req.StorageRef.Key) == "" || strings.TrimSpace(req.ImageRef) == "" {
		return containerapi.ContainerInfo{}, containerapi.ErrInvalidArgument
	}
	return s.CreateContainer(ctx, req)
}

func (s *Service) StartContainer(ctx context.Context, id string, _ *containerapi.StartTaskOptions) error {
	if strings.TrimSpace(id) == "" {
		return containerapi.ErrInvalidArgument
	}
	if err := s.ensureClients(); err != nil {
		return err
	}
	pod, err := s.client.CoreV1().Pods(s.namespace()).Get(ctx, id, metav1.GetOptions{})
	if err == nil {
		if pod.DeletionTimestamp.IsZero() {
			return s.waitPodReady(ctx, pod.Name)
		}
		if err := s.waitPodDeleted(ctx, pod.Name); err != nil {
			return err
		}
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return mapK8sErr(err)
	}
	pvc, err := s.client.CoreV1().PersistentVolumeClaims(s.namespace()).Get(ctx, dataPVCName(id), metav1.GetOptions{})
	if err != nil {
		return mapK8sErr(err)
	}
	imageRef := strings.TrimSpace(pvc.Annotations[imageAnnotation])
	if imageRef == "" {
		return containerapi.ErrInvalidArgument
	}
	req := containerapi.CreateContainerRequest{
		ID:         id,
		ImageRef:   imageRef,
		StorageRef: containerapi.StorageRef{Driver: runtimeName, Key: pvc.Name, Kind: "pvc"},
		Labels:     cloneLabels(pvc.Labels),
		Spec: containerapi.ContainerSpec{
			Cmd: []string{"/opt/memoh/bridge"},
			Env: []string{fmt.Sprintf("BRIDGE_TCP_ADDR=:%d", s.bridgePort())},
			Mounts: []containerapi.MountSpec{
				{
					Destination: "/opt/memoh",
					Type:        "bind",
					Source:      s.cfg.Workspace.RuntimePath(),
					Options:     []string{"rbind", "ro"},
				},
			},
		},
	}
	if err := s.ensurePod(ctx, s.namespace(), pvc.Name, req); err != nil {
		return err
	}
	return s.waitPodReady(ctx, id)
}

func (s *Service) StopContainer(ctx context.Context, id string, _ *containerapi.StopTaskOptions) error {
	if strings.TrimSpace(id) == "" {
		return containerapi.ErrInvalidArgument
	}
	if err := s.ensureClients(); err != nil {
		return err
	}
	grace := int64(10)
	err := s.client.CoreV1().Pods(s.namespace()).Delete(ctx, id, metav1.DeleteOptions{GracePeriodSeconds: &grace})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return mapK8sErr(err)
	}
	return s.waitPodDeleted(ctx, id)
}

func (s *Service) DeleteTask(ctx context.Context, id string, opts *containerapi.DeleteTaskOptions) error {
	return s.StopContainer(ctx, id, &containerapi.StopTaskOptions{Force: opts != nil && opts.Force})
}

func (s *Service) GetTaskInfo(ctx context.Context, id string) (containerapi.TaskInfo, error) {
	if strings.TrimSpace(id) == "" {
		return containerapi.TaskInfo{}, containerapi.ErrInvalidArgument
	}
	if err := s.ensureClients(); err != nil {
		return containerapi.TaskInfo{}, err
	}
	pod, err := s.client.CoreV1().Pods(s.namespace()).Get(ctx, id, metav1.GetOptions{})
	if err != nil {
		return containerapi.TaskInfo{}, mapK8sErr(err)
	}
	return podToTaskInfo(pod), nil
}

func (s *Service) GetContainerMetrics(ctx context.Context, id string) (containerapi.ContainerMetrics, error) {
	if strings.TrimSpace(id) == "" {
		return containerapi.ContainerMetrics{}, containerapi.ErrInvalidArgument
	}
	if err := s.ensureClients(); err != nil {
		return containerapi.ContainerMetrics{}, err
	}
	if s.metrics == nil {
		return containerapi.ContainerMetrics{}, containerapi.ErrNotSupported
	}
	podMetrics, err := s.metrics.MetricsV1beta1().PodMetricses(s.namespace()).Get(ctx, id, metav1.GetOptions{})
	if err != nil {
		return containerapi.ContainerMetrics{}, mapK8sMetricsErr(err)
	}
	for _, item := range podMetrics.Containers {
		if item.Name != workspaceContainer {
			continue
		}
		cpuQuantity := item.Usage.Cpu()
		memoryQuantity := item.Usage.Memory()
		cpuNanocores := cpuQuantity.MilliValue() * 1_000_000
		memoryBytes := memoryQuantity.Value()
		result := containerapi.ContainerMetrics{
			SampledAt: podMetrics.Timestamp.Time,
			CPU:       &containerapi.CPUMetrics{},
			Memory:    &containerapi.MemoryMetrics{},
		}
		if cpuNanocores > 0 {
			result.CPU.UsageNanocores = uint64(cpuNanocores) //nolint:gosec // negative quantities are ignored above
		}
		if memoryBytes > 0 {
			result.Memory.UsageBytes = uint64(memoryBytes) //nolint:gosec // negative quantities are ignored above
		}
		return result, nil
	}
	return containerapi.ContainerMetrics{}, containerapi.ErrNotFound
}

func (s *Service) ListTasks(ctx context.Context, _ *containerapi.ListTasksOptions) ([]containerapi.TaskInfo, error) {
	if err := s.ensureClients(); err != nil {
		return nil, err
	}
	pods, err := s.client.CoreV1().Pods(s.namespace()).List(ctx, metav1.ListOptions{
		LabelSelector: "memoh.workspace",
	})
	if err != nil {
		return nil, mapK8sErr(err)
	}
	out := make([]containerapi.TaskInfo, 0, len(pods.Items))
	for i := range pods.Items {
		out = append(out, podToTaskInfo(&pods.Items[i]))
	}
	return out, nil
}

func (s *Service) SetupNetwork(ctx context.Context, req containerapi.NetworkRequest) (containerapi.NetworkResult, error) {
	if strings.TrimSpace(req.ContainerID) == "" {
		return containerapi.NetworkResult{}, containerapi.ErrInvalidArgument
	}
	if err := s.ensureClients(); err != nil {
		return containerapi.NetworkResult{}, err
	}
	pod, err := s.client.CoreV1().Pods(s.namespace()).Get(ctx, req.ContainerID, metav1.GetOptions{})
	if err != nil {
		return containerapi.NetworkResult{}, mapK8sErr(err)
	}
	return containerapi.NetworkResult{IP: strings.TrimSpace(pod.Status.PodIP)}, nil
}

func (*Service) RemoveNetwork(context.Context, containerapi.NetworkRequest) error {
	return nil
}

func (s *Service) CheckNetwork(ctx context.Context, req containerapi.NetworkRequest) error {
	result, err := s.SetupNetwork(ctx, req)
	if err != nil {
		return err
	}
	if strings.TrimSpace(result.IP) == "" {
		return containerapi.ErrNotSupported
	}
	return nil
}

func (s *Service) CommitSnapshot(ctx context.Context, req containerapi.CommitSnapshotRequest) error {
	snapshotter := strings.TrimSpace(req.Source.Driver)
	name := strings.TrimSpace(req.Target.Key)
	key := strings.TrimSpace(req.Source.Key)
	if !isK8sSnapshotter(snapshotter) {
		return containerapi.ErrNotSupported
	}
	if strings.TrimSpace(name) == "" || strings.TrimSpace(key) == "" {
		return containerapi.ErrInvalidArgument
	}
	if err := s.ensureClients(); err != nil {
		return err
	}
	className, err := s.volumeSnapshotClass(ctx)
	if err != nil {
		return err
	}
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "snapshot.storage.k8s.io/v1",
		"kind":       "VolumeSnapshot",
		"metadata": map[string]any{
			"name":      name,
			"namespace": s.namespace(),
			"labels": map[string]any{
				containerapi.StorageKeyLabel: name,
				snapshotParentLabel:          key,
			},
		},
		"spec": map[string]any{
			"source": map[string]any{
				"persistentVolumeClaimName": key,
			},
			"volumeSnapshotClassName": className,
		},
	}}
	if _, err := s.dynamic.Resource(volumeSnapshotsGVR).Namespace(s.namespace()).Create(ctx, obj, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return mapK8sErr(err)
		}
	}
	return s.waitVolumeSnapshotReady(ctx, name)
}

func (s *Service) ListSnapshots(ctx context.Context, req containerapi.ListSnapshotsRequest) ([]containerapi.SnapshotInfo, error) {
	snapshotter := strings.TrimSpace(req.Driver)
	if !isK8sSnapshotter(snapshotter) {
		return nil, containerapi.ErrNotSupported
	}
	if err := s.ensureClients(); err != nil {
		return nil, err
	}
	out := []containerapi.SnapshotInfo{}
	claims, err := s.client.CoreV1().PersistentVolumeClaims(s.namespace()).List(ctx, metav1.ListOptions{
		LabelSelector: containerapi.StorageKeyLabel,
	})
	if err != nil {
		return nil, mapK8sErr(err)
	}
	for i := range claims.Items {
		pvc := &claims.Items[i]
		out = append(out, containerapi.SnapshotInfo{
			Name:    pvc.Name,
			Parent:  strings.TrimSpace(pvc.Labels[snapshotParentLabel]),
			Kind:    "active",
			Created: pvc.CreationTimestamp.Time,
			Updated: pvc.CreationTimestamp.Time,
			Labels:  cloneLabels(pvc.Labels),
		})
	}
	snaps, err := s.dynamic.Resource(volumeSnapshotsGVR).Namespace(s.namespace()).List(ctx, metav1.ListOptions{
		LabelSelector: containerapi.StorageKeyLabel,
	})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return out, nil
		}
		return nil, mapK8sErr(err)
	}
	for _, item := range snaps.Items {
		out = append(out, snapshotInfoFromUnstructured(item))
	}
	return out, nil
}

func (s *Service) PrepareSnapshot(ctx context.Context, req containerapi.PrepareSnapshotRequest) error {
	snapshotter := strings.TrimSpace(req.Target.Driver)
	key := strings.TrimSpace(req.Target.Key)
	parent := strings.TrimSpace(req.Parent.Key)
	if !isK8sSnapshotter(snapshotter) {
		return containerapi.ErrNotSupported
	}
	if strings.TrimSpace(key) == "" || strings.TrimSpace(parent) == "" {
		return containerapi.ErrInvalidArgument
	}
	if err := s.ensureClients(); err != nil {
		return err
	}
	size, err := resource.ParseQuantity(s.cfg.Kubernetes.EffectivePVCSize())
	if err != nil {
		return err
	}
	labels := map[string]string{
		containerapi.StorageKeyLabel: key,
		snapshotParentLabel:          parent,
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key,
			Namespace: s.namespace(),
			Labels:    labels,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: size},
			},
			DataSource: &corev1.TypedLocalObjectReference{
				APIGroup: stringPtr("snapshot.storage.k8s.io"),
				Kind:     "VolumeSnapshot",
				Name:     parent,
			},
		},
	}
	if storageClass := strings.TrimSpace(s.cfg.Kubernetes.PVCStorageClass); storageClass != "" {
		pvc.Spec.StorageClassName = &storageClass
	}
	if _, err := s.client.CoreV1().PersistentVolumeClaims(s.namespace()).Create(ctx, pvc, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return mapK8sErr(err)
		}
	}
	return s.waitPVCBound(ctx, key)
}

func (s *Service) BridgeTarget(botID string) string {
	if strings.TrimSpace(botID) == "" {
		return ""
	}
	if err := s.ensureClients(); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	pod, err := s.client.CoreV1().Pods(s.namespace()).Get(ctx, defaultWorkspacePref+botID, metav1.GetOptions{})
	if err != nil || strings.TrimSpace(pod.Status.PodIP) == "" {
		return ""
	}
	if s.inCluster || s.restCfg == nil {
		return netJoinHostPort(pod.Status.PodIP, s.bridgePort())
	}
	return s.portForwardTarget(ctx, pod.Name)
}

func (s *Service) SnapshotSupported(ctx context.Context) bool {
	if err := s.ensureClients(); err != nil {
		return false
	}
	_, err := s.volumeSnapshotClass(ctx)
	return err == nil
}

func (s *Service) ensureClients() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil && s.dynamic != nil {
		return nil
	}
	if s.clientErr != nil {
		return s.clientErr
	}
	restCfg, err := s.buildRESTConfig()
	if err != nil {
		s.clientErr = err
		return err
	}
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		s.clientErr = err
		return err
	}
	dynamicClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		s.clientErr = err
		return err
	}
	metricsClient, err := metricsclient.NewForConfig(restCfg)
	if err != nil {
		metricsClient = nil
	}
	s.restCfg = restCfg
	s.client = clientset
	s.dynamic = dynamicClient
	s.metrics = metricsClient
	return nil
}

func (s *Service) buildRESTConfig() (*rest.Config, error) {
	kubeconfig := strings.TrimSpace(s.cfg.Kubernetes.Kubeconfig)
	if s.cfg.Kubernetes.InCluster && kubeconfig == "" {
		if cfg, err := rest.InClusterConfig(); err == nil {
			s.inCluster = true
			return cfg, nil
		}
	}
	s.inCluster = false
	if kubeconfig == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
	}
	if kubeconfig == "" {
		return nil, errors.New("kubernetes kubeconfig is required outside cluster")
	}
	return clientcmd.BuildConfigFromFlags("", expandHome(kubeconfig))
}

func (s *Service) portForwardTarget(ctx context.Context, podName string) string {
	stopCh := make(chan struct{})
	readyCh := make(chan struct{})
	transport, upgrader, err := spdy.RoundTripperFor(s.restCfg)
	if err != nil {
		return ""
	}
	serverURL, err := url.Parse(s.restCfg.Host)
	if err != nil {
		return ""
	}
	serverURL.Path = fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", s.namespace(), podName)
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, serverURL)
	forwarder, err := portforward.NewOnAddresses(
		dialer,
		[]string{"127.0.0.1"},
		[]string{fmt.Sprintf("0:%d", s.bridgePort())},
		stopCh,
		readyCh,
		io.Discard,
		io.Discard,
	)
	if err != nil {
		close(stopCh)
		return ""
	}

	s.replacePortForward(podName, stopCh)
	go func() {
		if err := forwarder.ForwardPorts(); err != nil {
			s.clearPortForward(podName, stopCh)
		}
	}()

	select {
	case <-readyCh:
	case <-ctx.Done():
		s.clearPortForward(podName, stopCh)
		return ""
	case <-time.After(5 * time.Second):
		s.clearPortForward(podName, stopCh)
		return ""
	}
	ports, err := forwarder.GetPorts()
	if err != nil || len(ports) == 0 {
		s.clearPortForward(podName, stopCh)
		return ""
	}
	return netJoinHostPort("127.0.0.1", int(ports[0].Local))
}

func (s *Service) replacePortForward(podName string, stopCh chan struct{}) {
	s.pfMu.Lock()
	defer s.pfMu.Unlock()
	if s.pfStops == nil {
		s.pfStops = map[string]chan struct{}{}
	}
	if previous := s.pfStops[podName]; previous != nil {
		close(previous)
	}
	s.pfStops[podName] = stopCh
}

func (s *Service) clearPortForward(podName string, stopCh chan struct{}) {
	s.pfMu.Lock()
	defer s.pfMu.Unlock()
	if s.pfStops[podName] == stopCh {
		delete(s.pfStops, podName)
		close(stopCh)
	}
}

func (s *Service) namespace() string { return s.cfg.Kubernetes.EffectiveNamespace() }

func (s *Service) bridgePort() int { return s.cfg.Kubernetes.EffectiveBridgePort() }

func (s *Service) ensurePVC(ctx context.Context, namespace, name string, req containerapi.CreateContainerRequest) error {
	if existing, err := s.client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{}); err == nil {
		if existing.Annotations == nil {
			existing.Annotations = map[string]string{}
		}
		if existing.Annotations[imageAnnotation] != req.ImageRef {
			existing.Annotations[imageAnnotation] = req.ImageRef
			_, _ = s.client.CoreV1().PersistentVolumeClaims(namespace).Update(ctx, existing, metav1.UpdateOptions{})
		}
		return nil
	} else if !apierrors.IsNotFound(err) {
		return mapK8sErr(err)
	}
	size, err := resource.ParseQuantity(s.cfg.Kubernetes.EffectivePVCSize())
	if err != nil {
		return err
	}
	labels := cloneLabels(req.Labels)
	labels[containerapi.StorageKeyLabel] = name
	annotations := map[string]string{imageAnnotation: req.ImageRef}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: size},
			},
		},
	}
	if storageClass := strings.TrimSpace(s.cfg.Kubernetes.PVCStorageClass); storageClass != "" {
		pvc.Spec.StorageClassName = &storageClass
	}
	_, err = s.client.CoreV1().PersistentVolumeClaims(namespace).Create(ctx, pvc, metav1.CreateOptions{})
	return mapK8sErr(err)
}

func (s *Service) ensurePod(ctx context.Context, namespace, pvcName string, req containerapi.CreateContainerRequest) error {
	if pvc, err := s.client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, pvcName, metav1.GetOptions{}); err == nil {
		if pvc.Annotations == nil {
			pvc.Annotations = map[string]string{}
		}
		if req.ImageRef != "" && pvc.Annotations[imageAnnotation] != req.ImageRef {
			pvc.Annotations[imageAnnotation] = req.ImageRef
			_, _ = s.client.CoreV1().PersistentVolumeClaims(namespace).Update(ctx, pvc, metav1.UpdateOptions{})
		}
	}
	if _, err := s.client.CoreV1().Pods(namespace).Get(ctx, req.ID, metav1.GetOptions{}); err == nil {
		return nil
	} else if !apierrors.IsNotFound(err) {
		return mapK8sErr(err)
	}
	labels := cloneLabels(req.Labels)
	labels[containerapi.StorageKeyLabel] = pvcName
	env := envVars(req.Spec.Env)
	env = upsertEnv(env, "BRIDGE_TCP_ADDR", fmt.Sprintf(":%d", s.bridgePort()))
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.ID,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			RestartPolicy:                 corev1.RestartPolicyAlways,
			ServiceAccountName:            strings.TrimSpace(s.cfg.Kubernetes.ServiceAccountName),
			ImagePullSecrets:              imagePullSecrets(s.cfg.Kubernetes.ImagePullSecret),
			DNSPolicy:                     corev1.DNSClusterFirst,
			Volumes:                       []corev1.Volume{{Name: dataVolumeName, VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcName}}}},
			Containers:                    []corev1.Container{{Name: workspaceContainer, Image: req.ImageRef, ImagePullPolicy: imagePullPolicy(req.ImagePullPolicy), Command: req.Spec.Cmd, Env: env, WorkingDir: req.Spec.WorkDir, TTY: req.Spec.TTY, VolumeMounts: []corev1.VolumeMount{{Name: dataVolumeName, MountPath: dataMountPath}}, Ports: []corev1.ContainerPort{{Name: "bridge", ContainerPort: int32(s.bridgePort())}}}}, //nolint:gosec // bridge_port is validated as an operator-controlled small TCP port.
			TerminationGracePeriodSeconds: int64Ptr(10),
		},
	}
	if len(req.Spec.DNS) > 0 {
		pod.Spec.DNSPolicy = corev1.DNSNone
		pod.Spec.DNSConfig = &corev1.PodDNSConfig{Nameservers: req.Spec.DNS}
	}
	addSpecMounts(pod, req.Spec.Mounts)
	_, err := s.client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return mapK8sErr(err)
	}
	return s.waitPodReady(ctx, req.ID)
}

func (s *Service) waitPodReady(ctx context.Context, name string) error {
	deadline, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		pod, err := s.client.CoreV1().Pods(s.namespace()).Get(deadline, name, metav1.GetOptions{})
		if err != nil {
			return mapK8sErr(err)
		}
		if workspaceContainerReady(pod) {
			return nil
		}
		select {
		case <-deadline.Done():
			return deadline.Err()
		case <-ticker.C:
		}
	}
}

func (s *Service) waitPodDeleted(ctx context.Context, name string) error {
	deadline, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		_, err := s.client.CoreV1().Pods(s.namespace()).Get(deadline, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return mapK8sErr(err)
		}
		select {
		case <-deadline.Done():
			return deadline.Err()
		case <-ticker.C:
		}
	}
}

func workspaceContainerReady(pod *corev1.Pod) bool {
	if pod == nil || pod.Status.Phase != corev1.PodRunning || strings.TrimSpace(pod.Status.PodIP) == "" {
		return false
	}
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == workspaceContainer {
			return status.Ready
		}
	}
	return false
}

func (s *Service) waitPVCBound(ctx context.Context, name string) error {
	deadline, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		pvc, err := s.client.CoreV1().PersistentVolumeClaims(s.namespace()).Get(deadline, name, metav1.GetOptions{})
		if err != nil {
			return mapK8sErr(err)
		}
		if pvc.Status.Phase == corev1.ClaimBound {
			return nil
		}
		select {
		case <-deadline.Done():
			return deadline.Err()
		case <-ticker.C:
		}
	}
}

func (s *Service) volumeSnapshotClass(ctx context.Context) (string, error) {
	items, err := s.dynamic.Resource(volumeSnapshotClassesGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", containerapi.ErrNotSupported
		}
		return "", mapK8sErr(err)
	}
	if len(items.Items) == 0 {
		return "", containerapi.ErrNotSupported
	}
	for _, item := range items.Items {
		if item.GetAnnotations()["snapshot.storage.kubernetes.io/is-default-class"] == "true" {
			return item.GetName(), nil
		}
	}
	if len(items.Items) == 1 {
		return items.Items[0].GetName(), nil
	}
	return "", containerapi.ErrNotSupported
}

func (s *Service) waitVolumeSnapshotReady(ctx context.Context, name string) error {
	deadline, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		obj, err := s.dynamic.Resource(volumeSnapshotsGVR).Namespace(s.namespace()).Get(deadline, name, metav1.GetOptions{})
		if err != nil {
			return mapK8sErr(err)
		}
		ready, _, _ := unstructured.NestedBool(obj.Object, "status", "readyToUse")
		if ready {
			return nil
		}
		select {
		case <-deadline.Done():
			return deadline.Err()
		case <-ticker.C:
		}
	}
}

func podToContainerInfo(pod *corev1.Pod) containerapi.ContainerInfo {
	imageRef := ""
	if len(pod.Spec.Containers) > 0 {
		imageRef = pod.Spec.Containers[0].Image
	}
	snapshotKey := strings.TrimSpace(pod.Labels[containerapi.StorageKeyLabel])
	if snapshotKey == "" {
		snapshotKey = dataPVCName(pod.Name)
	}
	return containerapi.ContainerInfo{
		ID:         pod.Name,
		Image:      imageRef,
		Labels:     cloneLabels(pod.Labels),
		StorageRef: containerapi.StorageRef{Driver: runtimeName, Key: snapshotKey, Kind: "pvc"},
		Runtime:    containerapi.RuntimeInfo{Name: runtimeName},
		CreatedAt:  pod.CreationTimestamp.Time,
		UpdatedAt:  pod.CreationTimestamp.Time,
	}
}

func podToTaskInfo(pod *corev1.Pod) containerapi.TaskInfo {
	return containerapi.TaskInfo{
		ContainerID: pod.Name,
		ID:          pod.Name,
		Status:      taskStatusFromPodPhase(pod.Status.Phase),
	}
}

func taskStatusFromPodPhase(phase corev1.PodPhase) containerapi.TaskStatus {
	switch phase {
	case corev1.PodRunning:
		return containerapi.TaskStatusRunning
	case corev1.PodPending:
		return containerapi.TaskStatusCreated
	case corev1.PodSucceeded, corev1.PodFailed:
		return containerapi.TaskStatusStopped
	default:
		return containerapi.TaskStatusUnknown
	}
}

func snapshotInfoFromUnstructured(item unstructured.Unstructured) containerapi.SnapshotInfo {
	labels := item.GetLabels()
	return containerapi.SnapshotInfo{
		Name:    firstNonEmpty(labels[containerapi.StorageKeyLabel], item.GetName()),
		Parent:  strings.TrimSpace(labels[snapshotParentLabel]),
		Kind:    "committed",
		Created: item.GetCreationTimestamp().Time,
		Updated: item.GetCreationTimestamp().Time,
		Labels:  cloneLabels(labels),
	}
}

func isK8sSnapshotter(snapshotter string) bool {
	snapshotter = strings.TrimSpace(snapshotter)
	return snapshotter == "" || snapshotter == runtimeName || snapshotter == "csi"
}

func dataPVCName(containerID string) string {
	name := strings.TrimSpace(containerID) + "-data"
	if len(name) <= 63 {
		return name
	}
	return name[:63]
}

func (s *Service) Namespace() string {
	return s.namespace()
}

func (s *Service) GetService(ctx context.Context, name string) (*corev1.Service, error) {
	if err := s.ensureClients(); err != nil {
		return nil, err
	}
	return s.client.CoreV1().Services(s.namespace()).Get(ctx, name, metav1.GetOptions{})
}

func (s *Service) ApplyService(ctx context.Context, svc *corev1.Service) (*corev1.Service, error) {
	if err := s.ensureClients(); err != nil {
		return nil, err
	}
	namespace := strings.TrimSpace(svc.Namespace)
	if namespace == "" {
		namespace = s.namespace()
	}
	svc.Namespace = namespace
	existing, err := s.client.CoreV1().Services(namespace).Get(ctx, svc.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return s.client.CoreV1().Services(namespace).Create(ctx, svc, metav1.CreateOptions{})
	}
	if err != nil {
		return nil, err
	}
	svc.ResourceVersion = existing.ResourceVersion
	return s.client.CoreV1().Services(namespace).Update(ctx, svc, metav1.UpdateOptions{})
}

func (s *Service) UpdateService(ctx context.Context, svc *corev1.Service) (*corev1.Service, error) {
	if err := s.ensureClients(); err != nil {
		return nil, err
	}
	namespace := strings.TrimSpace(svc.Namespace)
	if namespace == "" {
		namespace = s.namespace()
	}
	return s.client.CoreV1().Services(namespace).Update(ctx, svc, metav1.UpdateOptions{})
}

func (s *Service) GetResource(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
	if err := s.ensureClients(); err != nil {
		return nil, err
	}
	return s.dynamic.Resource(gvr).Namespace(effectiveNamespace(namespace, s.namespace())).Get(ctx, name, metav1.GetOptions{})
}

func (s *Service) ApplyResource(ctx context.Context, gvr schema.GroupVersionResource, namespace string, obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	if err := s.ensureClients(); err != nil {
		return nil, err
	}
	ns := effectiveNamespace(namespace, s.namespace())
	obj.SetNamespace(ns)
	resource := s.dynamic.Resource(gvr).Namespace(ns)
	existing, err := resource.Get(ctx, obj.GetName(), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return resource.Create(ctx, obj, metav1.CreateOptions{})
	}
	if err != nil {
		return nil, err
	}
	obj.SetResourceVersion(existing.GetResourceVersion())
	return resource.Update(ctx, obj, metav1.UpdateOptions{})
}

func (s *Service) DeleteResource(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, opts metav1.DeleteOptions) error {
	if err := s.ensureClients(); err != nil {
		return err
	}
	return s.dynamic.Resource(gvr).Namespace(effectiveNamespace(namespace, s.namespace())).Delete(ctx, name, opts)
}

func (s *Service) ListResources(ctx context.Context, gvr schema.GroupVersionResource, namespace string, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	if err := s.ensureClients(); err != nil {
		return nil, err
	}
	return s.dynamic.Resource(gvr).Namespace(effectiveNamespace(namespace, s.namespace())).List(ctx, opts)
}

func effectiveNamespace(namespace, fallback string) string {
	if trimmed := strings.TrimSpace(namespace); trimmed != "" {
		return trimmed
	}
	return fallback
}

func envVars(values []string) []corev1.EnvVar {
	out := make([]corev1.EnvVar, 0, len(values))
	for _, raw := range values {
		k, v, ok := strings.Cut(raw, "=")
		if !ok || strings.TrimSpace(k) == "" {
			continue
		}
		out = append(out, corev1.EnvVar{Name: strings.TrimSpace(k), Value: v})
	}
	return out
}

func imagePullPolicy(policy string) corev1.PullPolicy {
	switch strings.TrimSpace(strings.ToLower(policy)) {
	case config.ImagePullPolicyAlways:
		return corev1.PullAlways
	case config.ImagePullPolicyNever:
		return corev1.PullNever
	default:
		return corev1.PullIfNotPresent
	}
}

func upsertEnv(env []corev1.EnvVar, name, value string) []corev1.EnvVar {
	for i := range env {
		if env[i].Name == name {
			env[i].Value = value
			return env
		}
	}
	return append(env, corev1.EnvVar{Name: name, Value: value})
}

func addSpecMounts(pod *corev1.Pod, mounts []containerapi.MountSpec) {
	for i, mount := range mounts {
		if strings.TrimSpace(mount.Source) == "" || strings.TrimSpace(mount.Destination) == "" {
			continue
		}
		if mount.Destination == "/run/memoh" || mount.Destination == "/etc/resolv.conf" {
			continue
		}
		if mount.Type != "bind" {
			continue
		}
		name := "host-mount-" + strconv.Itoa(i)
		hostPathType := corev1.HostPathDirectoryOrCreate
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: name,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{Path: mount.Source, Type: &hostPathType},
			},
		})
		pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      name,
			MountPath: mount.Destination,
			ReadOnly:  hasReadonlyOption(mount.Options),
		})
	}
}

func imagePullSecrets(name string) []corev1.LocalObjectReference {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	return []corev1.LocalObjectReference{{Name: name}}
}

func hasReadonlyOption(options []string) bool {
	for _, opt := range options {
		switch strings.TrimSpace(opt) {
		case "ro", "readonly":
			return true
		}
	}
	return false
}

func cloneLabels(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mapK8sErr(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case apierrors.IsNotFound(err):
		return errors.Join(containerapi.ErrNotFound, err)
	case apierrors.IsAlreadyExists(err):
		return errors.Join(containerapi.ErrAlreadyExists, err)
	default:
		return errors.Join(containerapi.ErrRuntime, err)
	}
}

func mapK8sMetricsErr(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case apierrors.IsNotFound(err):
		return errors.Join(containerapi.ErrNotFound, err)
	case apierrors.IsForbidden(err), apierrors.IsServiceUnavailable(err):
		return errors.Join(containerapi.ErrNotSupported, err)
	default:
		return mapK8sErr(err)
	}
}

func stringPtr(v string) *string { return &v }

func int64Ptr(v int64) *int64 { return &v }

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func netJoinHostPort(host string, port int) string {
	return strings.TrimSpace(host) + ":" + strconv.Itoa(port)
}

func expandHome(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}
