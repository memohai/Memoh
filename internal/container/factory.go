package container

const (
	DefaultSocketPath = "/run/containerd/containerd.sock"
	DefaultNamespace  = "default"

	BackendContainerd = "containerd"
	BackendApple      = "apple"
	BackendDocker     = "docker"
)
