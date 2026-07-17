package agent

// ResolveRunConfigResult holds a fully resolved run configuration for one
// agent request, together with the selected model and session runtime.
// Produced by flow.Resolver.ResolveRunConfig and consumed by the turn
// runtime adapters.
type ResolveRunConfigResult struct {
	RunConfig   RunConfig
	ModelID     string // database UUID of the selected model
	RuntimeType string
}
