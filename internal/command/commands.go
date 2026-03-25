package command

// buildRegistry constructs the full command registry with all resource groups.
func (h *Handler) buildRegistry() *Registry {
	r := newRegistry()
	r.RegisterGroup(h.buildScheduleGroup())
	r.RegisterGroup(h.buildMCPGroup())
	r.RegisterGroup(h.buildSettingsGroup())
	r.RegisterGroup(h.buildModelGroup())
	r.RegisterGroup(h.buildMemoryGroup())
	r.RegisterGroup(h.buildSearchGroup())
	r.RegisterGroup(h.buildBrowserGroup())
	r.RegisterGroup(h.buildUsageGroup())
	r.RegisterGroup(h.buildEmailGroup())
	r.RegisterGroup(h.buildHeartbeatGroup())
	r.RegisterGroup(h.buildSkillGroup())
	r.RegisterGroup(h.buildFSGroup())
	return r
}
