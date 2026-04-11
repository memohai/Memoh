package network

// AttachmentRequest describes the network attachment work for a workspace
// container. The runtime network is always considered first; overlay fields are
// reserved for future SD-WAN provider attachment.
type AttachmentRequest struct {
	BotID       string
	ContainerID string
	Runtime     RuntimeNetworkRequest
	Overlay     OverlayRequest
}

// AttachmentStatus reports the outcome of runtime network and overlay
// attachment.
type AttachmentStatus struct {
	Runtime RuntimeNetworkStatus
	Overlay OverlayStatus
}

// RuntimeNetworkRequest describes the container network attachment target for a
// runtime adapter.
type RuntimeNetworkRequest struct {
	ContainerID string
	NetNSPath   string
	PID         uint32
	CNIBinDir   string
	CNIConfDir  string
}

// RuntimeNetworkStatus describes the current state of the container runtime
// network attachment.
type RuntimeNetworkStatus struct {
	Attached bool
	IP       string
}

// OverlayRequest is reserved for future provider-specific attachment options.
type OverlayRequest struct {
	Provider string
}

// OverlayStatus captures the observed overlay provider state.
type OverlayStatus struct {
	Provider string
	Attached bool
}
