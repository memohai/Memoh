package network

import "testing"

func TestDescriptorForBackendCapabilities(t *testing.T) {
	tests := []struct {
		name              string
		backend           string
		wantKind          string
		wantJoinNamespace bool
		wantJoinContainer bool
		wantCNI           bool
		wantKubernetes    bool
		wantSidecar       bool
		wantRuntimeSetup  bool
		wantClusterNative bool
	}{
		{
			name:              "containerd",
			backend:           "containerd",
			wantKind:          "containerd",
			wantJoinNamespace: true,
			wantCNI:           true,
			wantSidecar:       true,
			wantRuntimeSetup:  true,
		},
		{
			name:              "docker",
			backend:           "docker",
			wantKind:          "docker",
			wantJoinContainer: true,
		},
		{
			name:              "kubernetes",
			backend:           "kubernetes",
			wantKind:          "kubernetes",
			wantKubernetes:    true,
			wantClusterNative: true,
		},
		{
			name:     "apple",
			backend:  "apple",
			wantKind: "apple",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc := descriptorForBackend(tt.backend)
			if desc.Kind != tt.wantKind {
				t.Fatalf("Kind = %q, want %q", desc.Kind, tt.wantKind)
			}
			caps := desc.Capabilities
			if caps.JoinNamespacePath != tt.wantJoinNamespace {
				t.Fatalf("JoinNamespacePath = %v, want %v", caps.JoinNamespacePath, tt.wantJoinNamespace)
			}
			if caps.JoinContainerNetwork != tt.wantJoinContainer {
				t.Fatalf("JoinContainerNetwork = %v, want %v", caps.JoinContainerNetwork, tt.wantJoinContainer)
			}
			if caps.CNI != tt.wantCNI {
				t.Fatalf("CNI = %v, want %v", caps.CNI, tt.wantCNI)
			}
			if caps.KubernetesNative != tt.wantKubernetes {
				t.Fatalf("KubernetesNative = %v, want %v", caps.KubernetesNative, tt.wantKubernetes)
			}
			if caps.SidecarWorker != tt.wantSidecar {
				t.Fatalf("SidecarWorker = %v, want %v", caps.SidecarWorker, tt.wantSidecar)
			}
			if caps.RuntimeNetworkSetup != tt.wantRuntimeSetup {
				t.Fatalf("RuntimeNetworkSetup = %v, want %v", caps.RuntimeNetworkSetup, tt.wantRuntimeSetup)
			}
			if caps.ClusterNative != tt.wantClusterNative {
				t.Fatalf("ClusterNative = %v, want %v", caps.ClusterNative, tt.wantClusterNative)
			}
		})
	}
}
