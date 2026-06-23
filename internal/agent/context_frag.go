package agent

import (
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/models"
)

// RefreshContextFrag rebuilds the typed context frag view from the legacy
// RunConfig fields. The SDK-facing fields remain the source of truth in phase 1.
func (cfg RunConfig) RefreshContextFrag() RunConfig {
	query := cfg.Query
	if cfg.ContextQueryMaterialized {
		query = ""
	}
	assembled := contextfrag.Compile(contextfrag.CompileInput{
		Source:          contextfrag.SourceRunConfig,
		Scope:           cfg.ContextScope,
		System:          cfg.System,
		Messages:        cfg.Messages,
		Query:           query,
		InlineImages:    cfg.InlineImages,
		ToolUsage:       cfg.ContextToolUsage,
		DynamicMutators: cfg.ContextDynamicMutators,
		Existing:        cfg.ContextFrags,
	})
	cfg.ContextFrags = assembled.Frags
	cfg.ContextManifest = assembled.Manifest
	return cfg
}

func (cfg RunConfig) RefreshContextFragWithDynamicMutators(readMedia bool, beforeModelCallHook bool, injectCh bool) RunConfig {
	cfg.ContextDynamicMutators = cfg.contextDynamicMutators(readMedia, beforeModelCallHook, injectCh)
	return cfg.RefreshContextFrag()
}

func (cfg RunConfig) contextDynamicMutators(readMedia bool, beforeModelCallHook bool, injectCh bool) []contextfrag.DynamicMutator {
	var mutators []contextfrag.DynamicMutator
	if cfg.Model != nil &&
		models.ResolveClientType(cfg.Model) == string(models.ClientTypeAnthropicMessages) &&
		models.NormalizePromptCacheTTL(cfg.PromptCacheTTL) != models.PromptCacheTTLOff {
		mutators = append(mutators, contextfrag.DynamicMutatorPromptCache)
	}
	if injectCh && cfg.InjectCh != nil {
		mutators = append(mutators, contextfrag.DynamicMutatorInjectCh)
	}
	if readMedia {
		mutators = append(mutators, contextfrag.DynamicMutatorReadMedia)
	}
	if beforeModelCallHook {
		mutators = append(mutators, contextfrag.DynamicMutatorBeforeModelCallHook)
	}
	if cfg.BackgroundManager != nil {
		mutators = append(mutators, contextfrag.DynamicMutatorBackgroundSummary)
	}
	mutators = append(mutators, contextfrag.DynamicMutatorMidTaskPrune)
	return mutators
}
