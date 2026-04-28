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
