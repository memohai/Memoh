// Package container ships the orchestrationenv.Backend implementation
// for the KindContainer resource family. It allocates a container per
// env session via the project's existing internal/container runtime
// abstraction, returning a runtime handle the kernel persists into
// orchestration_env_sessions.runtime_handle so subsequent
// snapshot / release calls can reattach to the same workload.
//
// The backend depends on the small Runtime interface defined here
// rather than on internal/container.Service directly. That keeps the
// adapter testable with a fake Runtime and lets the cmd/agent wiring
// layer pick whichever container backend (containerd / docker /
// apple) is appropriate for the deployment target without changing
// this package.
//
// Image management: the backend honours an explicit image_pull_policy
// in the resource config but does not yet implement caching or
// retries — Stage 3-E and 3-I will tighten the production path once
// real workloads exercise it. Snapshot capture maps onto
// container.SnapshotService.CommitSnapshot when the underlying
// runtime supports it; backends that do not (apple virtualization
// today) get a stable bookkeeping reference and an empty digest.
package container
