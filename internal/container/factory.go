package container

const (
	DefaultSocketPath = "/run/containerd/containerd.sock"
	DefaultNamespace  = "default"

	BackendContainerd = "containerd"
	BackendApple      = "apple"
	BackendKubernetes = "kubernetes"
	BackendK8s        = "k8s"
	BackendDocker     = "docker"
)

func NormalizeBackend(backend string) string {
	switch backend {
	case BackendK8s:
		return BackendKubernetes
	default:
		return backend
	}
}
