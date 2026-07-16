package workspace

import "testing"

func TestResourceLimitCapabilitiesForContainerRuncRuntime(t *testing.T) {
	t.Parallel()

	caps := ResourceLimitCapabilitiesFor("container", "io.containerd.runc.v2")
	if !caps.CPU.HardLimitSupported {
		t.Fatal("runc runtime should support CPU hard limits")
	}
	if !caps.Memory.HardLimitSupported {
		t.Fatal("runc runtime should support memory hard limits")
	}
	if caps.Storage.HardLimitSupported {
		t.Fatal("runc runtime should not report storage hard limits without a disk quota implementation")
	}
	if !caps.Storage.SoftLimitSupported {
		t.Fatal("runc runtime should keep storage soft limits")
	}
}
