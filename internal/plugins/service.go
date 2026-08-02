package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/mcp"
	skillset "github.com/memohai/memoh/internal/skills"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

type BridgeProvider struct {
	Provider bridge.Provider
}

type Service struct {
	queries      dbstore.Queries
	mcpService   *mcp.ConnectionService
	oauthService *mcp.OAuthService
	oauthClients *OAuthClientRegistry
	bridges      bridge.Provider
	logger       *slog.Logger
}

func NewService(log *slog.Logger, queries dbstore.Queries, mcpService *mcp.ConnectionService, oauthService *mcp.OAuthService, oauthClients *OAuthClientRegistry, bridges BridgeProvider) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		queries:      queries,
		mcpService:   mcpService,
		oauthService: oauthService,
		oauthClients: oauthClients,
		bridges:      bridges.Provider,
		logger:       log.With(slog.String("service", "plugins")),
	}
}

func (s *Service) List(ctx context.Context, botID string) ([]Installation, error) {
	if scoped := s.scopedService(ctx, botID); scoped != nil && scoped != s {
		return scoped.List(ctx, botID)
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListBotPluginInstallations(ctx, botUUID)
	if err != nil {
		return nil, err
	}
	items := make([]Installation, 0, len(rows))
	for _, row := range rows {
		item, err := s.normalizeInstallation(ctx, row)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) Get(ctx context.Context, botID, installationID string) (Installation, error) {
	if scoped := s.scopedService(ctx, botID); scoped != nil && scoped != s {
		return scoped.Get(ctx, botID, installationID)
	}
	row, err := s.getRow(ctx, botID, installationID)
	if err != nil {
		return Installation{}, err
	}
	return s.normalizeInstallation(ctx, row)
}

// InstalledPluginRelease returns the immutable release currently owned by a
// bot/plugin identity. An installed Plugin without release metadata is still
// reported as installed so callers cannot mistake it for a new installation.
func (s *Service) InstalledPluginRelease(ctx context.Context, botID, pluginID string) (string, bool, error) {
	state, installed, err := s.InstalledPluginState(ctx, botID, pluginID)
	return state.ReleaseRevision, installed, err
}

// InstalledPluginState returns both the immutable release revision and the
// mutable installation generation represented by updated_at.
func (s *Service) InstalledPluginState(
	ctx context.Context,
	botID, pluginID string,
) (InstalledPluginState, bool, error) {
	if scoped := s.scopedService(ctx, botID); scoped != nil && scoped != s {
		return scoped.InstalledPluginState(ctx, botID, pluginID)
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return InstalledPluginState{}, false, err
	}
	rows, err := s.queries.ListBotPluginInstallations(ctx, botUUID)
	if err != nil {
		return InstalledPluginState{}, false, err
	}
	for _, row := range rows {
		if row.PluginID != pluginID {
			continue
		}
		metadata, err := decodeJSONMap(row.Metadata)
		if err != nil {
			return InstalledPluginState{}, false, err
		}
		revision, _ := metadata["release_revision"].(string)
		return InstalledPluginState{
			ReleaseRevision: strings.TrimSpace(revision),
			UpdatedAt:       timeFromPg(row.UpdatedAt),
		}, true, nil
	}
	return InstalledPluginState{}, false, nil
}

func (s *Service) Install(ctx context.Context, botID string, req InstallRequest) (Installation, error) {
	var result Installation
	err := s.withBotMutation(ctx, botID, func(scopedCtx context.Context, scoped *Service) error {
		return scoped.inTransaction(scopedCtx, func(txService *Service) error {
			var installErr error
			result, installErr = txService.install(scopedCtx, botID, req)
			return installErr
		})
	})
	return result, err
}

func (s *Service) install(
	ctx context.Context,
	botID string,
	req InstallRequest,
) (Installation, error) {
	if s.queries == nil || s.mcpService == nil {
		return Installation{}, errors.New("plugin service is not configured")
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return Installation{}, err
	}
	manifest := normalizeManifest(req.Manifest)
	if manifest.ID == "" {
		return Installation{}, errors.New("plugin id is required")
	}
	if err := ValidateSkillReferences(manifest.Skills); err != nil {
		return Installation{}, err
	}
	if err := validateReleaseMetadata(req.Release); err != nil {
		return Installation{}, err
	}
	if err := validateSkillArtifacts(manifest.Skills, req.SkillArtifacts); err != nil {
		return Installation{}, err
	}
	if manifest.Name == "" {
		manifest.Name = manifest.ID
	}
	status := s.evaluateInitialStatus(manifest, req.Variables)
	enabled := status == StatusReady

	configPayload, err := encodeJSON(map[string]any{
		"variables": req.Variables,
	})
	if err != nil {
		return Installation{}, err
	}
	metadata := manifestMetadata(manifest)
	if req.Release.Revision != "" {
		metadata["release_revision"] = req.Release.Revision
		metadata["plugin_artifact_digest"] = req.Release.ArtifactDigest
	}
	if workspaceTargetID := strings.TrimSpace(req.WorkspaceTargetID); workspaceTargetID != "" {
		metadata["workspace_target_id"] = workspaceTargetID
	}
	metadataPayload, err := encodeJSON(metadata)
	if err != nil {
		return Installation{}, err
	}
	manifestPayload, err := encodeJSON(manifest)
	if err != nil {
		return Installation{}, err
	}

	row, err := s.queries.CreateBotPluginInstallation(ctx, sqlc.CreateBotPluginInstallationParams{
		BotID:      botUUID,
		PluginID:   manifest.ID,
		PluginName: manifest.Name,
		Version:    manifest.Version,
		Status:     status,
		Enabled:    enabled,
		Config:     configPayload,
		Metadata:   metadataPayload,
		Manifest:   manifestPayload,
	})
	if err != nil {
		return Installation{}, err
	}

	installationID := row.ID.String()
	if err := s.mcpService.DeleteByPlugin(ctx, botID, installationID); err != nil {
		return Installation{}, err
	}
	if err := s.queries.DeleteBotPluginResources(ctx, row.ID); err != nil {
		return Installation{}, err
	}

	for _, resource := range manifest.MCPs {
		authReq := manifestAuthForResource(manifest, resource)
		connReq := buildMCPConnectionRequest(manifest, resource, authReq, req.Variables)
		active := enabled && strings.TrimSpace(strings.ToLower(authReq.Type)) != "managed_oauth"
		connReq.Active = &active
		conn, err := s.mcpService.CreateManaged(ctx, botID, connReq, mcp.ManagedConnectionRequest{
			InstallationID: installationID,
			ResourceKey:    resource.Key,
			Visible:        strings.TrimSpace(strings.ToLower(resource.Visibility)) == "visible",
			Metadata:       mcpResourceMetadata(manifest, resource, authReq),
		})
		if err != nil {
			return Installation{}, fmt.Errorf("create plugin MCP resource %q: %w", resource.Key, err)
		}
		if _, err := s.queries.UpsertBotPluginResource(ctx, sqlc.UpsertBotPluginResourceParams{
			InstallationID: row.ID,
			ResourceType:   "mcp",
			ResourceKey:    resource.Key,
			ResourceID:     conn.ID,
			Status:         resourceStatus(status, authReq),
			Metadata:       mustJSON(mcpResourceMetadata(manifest, resource, authReq)),
		}); err != nil {
			return Installation{}, err
		}
	}

	for _, resource := range manifest.Skills {
		dirPath, err := skillset.SkillDirForIDs(resource.RegistryID, resource.PackageID, resource.SkillID)
		if err != nil {
			return Installation{}, fmt.Errorf("plugin Skill reference %q is invalid", SkillReferenceIdentity(resource))
		}
		identity := SkillReferenceIdentity(resource)
		metadata := map[string]any{
			"registry_id": resource.RegistryID,
			"package_id":  resource.PackageID,
			"skill_id":    resource.SkillID,
		}
		if workspaceTargetID := strings.TrimSpace(req.WorkspaceTargetID); workspaceTargetID != "" {
			metadata["workspace_target_id"] = workspaceTargetID
		}
		if artifact, ok := req.SkillArtifacts[identity]; ok {
			metadata["registry_revision"] = artifact.RegistryRevision
			metadata["install_id"] = artifact.InstallID
			metadata["artifact_digest"] = artifact.ArtifactDigest
			metadata["files_written"] = artifact.FilesWritten
		}
		if _, err := s.queries.UpsertBotPluginResource(ctx, sqlc.UpsertBotPluginResourceParams{
			InstallationID: row.ID,
			ResourceType:   "skill",
			ResourceKey:    identity,
			ResourceID:     path.Join(dirPath, "SKILL.md"),
			Status:         "installed",
			Metadata:       mustJSON(metadata),
		}); err != nil {
			return Installation{}, err
		}
	}
	return s.normalizeInstallation(ctx, row)
}

func (s *Service) SetEnabled(ctx context.Context, botID, installationID string, enabled bool) (Installation, error) {
	var result Installation
	err := s.withBotMutation(ctx, botID, func(scopedCtx context.Context, scoped *Service) error {
		return scoped.inTransaction(scopedCtx, func(txService *Service) error {
			var updateErr error
			result, updateErr = txService.setEnabled(scopedCtx, botID, installationID, enabled)
			return updateErr
		})
	})
	return result, err
}

func (s *Service) setEnabled(ctx context.Context, botID, installationID string, enabled bool) (Installation, error) {
	row, err := s.getRow(ctx, botID, installationID)
	if err != nil {
		return Installation{}, err
	}
	if !enabled {
		if err := s.mcpService.SetPluginConnectionsActive(ctx, botID, installationID, false); err != nil {
			return Installation{}, err
		}
		updated, err := s.updateStatus(ctx, botID, installationID, StatusDisabled, false)
		if err != nil {
			return Installation{}, err
		}
		return s.normalizeInstallation(ctx, updated)
	}

	manifest, err := decodeManifest(row.Manifest)
	if err != nil {
		return Installation{}, err
	}
	if row.Status == StatusUninstalled {
		return Installation{}, errors.New("plugin is uninstalled")
	}
	variables, configErr := variablesFromConfig(row.Config)
	if configErr != nil {
		return Installation{}, configErr
	}
	status := s.evaluateInitialStatus(manifest, variables)
	if status == StatusNeedsAuth {
		status, err = s.refreshOAuthStatus(ctx, botID, row, manifest)
		if err != nil {
			return Installation{}, err
		}
	}
	if status != StatusReady {
		return Installation{}, fmt.Errorf("plugin is not ready: %s", status)
	}
	if err := s.mcpService.SetPluginConnectionsActive(ctx, botID, installationID, true); err != nil {
		return Installation{}, err
	}
	updated, err := s.updateStatus(ctx, botID, installationID, StatusReady, true)
	if err != nil {
		return Installation{}, err
	}
	return s.normalizeInstallation(ctx, updated)
}

func (s *Service) Uninstall(ctx context.Context, botID, installationID string) (Installation, error) {
	var result Installation
	err := s.withBotMutation(ctx, botID, func(scopedCtx context.Context, scoped *Service) error {
		row, err := scoped.getRow(scopedCtx, botID, installationID)
		if err != nil {
			return err
		}
		var bundleRemoval *pluginBundleRemoval
		if row.Status != StatusUninstalled {
			bundleRemoval, err = scoped.preparePluginBundleRemoval(scopedCtx, botID, row)
			if err != nil {
				return err
			}
		}
		if err := scoped.inTransaction(scopedCtx, func(txService *Service) error {
			var uninstallErr error
			result, uninstallErr = txService.uninstall(scopedCtx, botID, installationID)
			return uninstallErr
		}); err != nil {
			if rollbackErr := bundleRemoval.rollback(scopedCtx); rollbackErr != nil {
				return errors.Join(err, fmt.Errorf("roll back Plugin bundle removal: %w", rollbackErr))
			}
			return err
		}
		if err := bundleRemoval.commit(scopedCtx); err != nil {
			scoped.logger.Warn(
				"cleanup removed Plugin bundle failed",
				slog.String("bot_id", botID),
				slog.String("plugin_id", row.PluginID),
				slog.Any("error", err),
			)
		}
		return nil
	})
	return result, err
}

func (s *Service) uninstall(
	ctx context.Context,
	botID, installationID string,
) (Installation, error) {
	row, err := s.getRow(ctx, botID, installationID)
	if err != nil {
		return Installation{}, err
	}
	if err := s.mcpService.DeleteByPlugin(ctx, botID, installationID); err != nil {
		return Installation{}, err
	}
	if err := s.queries.DeleteBotPluginResources(ctx, row.ID); err != nil {
		return Installation{}, err
	}
	updated, err := s.updateStatus(ctx, botID, installationID, StatusUninstalled, false)
	if err != nil {
		return Installation{}, err
	}
	return s.normalizeInstallation(ctx, updated)
}

func (s *Service) Purge(ctx context.Context, botID, installationID string) error {
	return s.withBotMutation(ctx, botID, func(scopedCtx context.Context, scoped *Service) error {
		row, err := scoped.getRow(scopedCtx, botID, installationID)
		if err != nil {
			return err
		}
		var bundleRemoval *pluginBundleRemoval
		if row.Status != StatusUninstalled {
			bundleRemoval, err = scoped.preparePluginBundleRemoval(scopedCtx, botID, row)
			if err != nil {
				return err
			}
		}
		if err := scoped.inTransaction(scopedCtx, func(txService *Service) error {
			return txService.purge(scopedCtx, botID, installationID)
		}); err != nil {
			if rollbackErr := bundleRemoval.rollback(scopedCtx); rollbackErr != nil {
				return errors.Join(err, fmt.Errorf("roll back Plugin bundle removal: %w", rollbackErr))
			}
			return err
		}
		if err := bundleRemoval.commit(scopedCtx); err != nil {
			scoped.logger.Warn(
				"cleanup purged Plugin bundle failed",
				slog.String("bot_id", botID),
				slog.String("plugin_id", row.PluginID),
				slog.Any("error", err),
			)
		}
		return nil
	})
}

func (s *Service) purge(
	ctx context.Context,
	botID, installationID string,
) error {
	row, err := s.getRow(ctx, botID, installationID)
	if err != nil {
		return err
	}
	if err := s.mcpService.DeleteByPlugin(ctx, botID, installationID); err != nil {
		return err
	}
	if err := s.queries.DeleteBotPluginResources(ctx, row.ID); err != nil {
		return err
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	installationUUID, err := db.ParseUUID(installationID)
	if err != nil {
		return err
	}
	if err := s.queries.DeleteBotPluginInstallation(ctx, sqlc.DeleteBotPluginInstallationParams{
		BotID: botUUID,
		ID:    installationUUID,
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) StartOAuth(ctx context.Context, botID, installationID, callbackURL string) (*mcp.AuthorizeResult, error) {
	row, err := s.getRow(ctx, botID, installationID)
	if err != nil {
		return nil, err
	}
	manifest, err := decodeManifest(row.Manifest)
	if err != nil {
		return nil, err
	}
	resources, err := s.queries.ListBotPluginResources(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	resourceByKey := map[string]string{}
	for _, resource := range resources {
		if strings.TrimSpace(resource.ResourceType) == "mcp" {
			resourceByKey[resource.ResourceKey] = resource.ResourceID
		}
	}
	for _, resource := range manifest.MCPs {
		authReq := manifestAuthForResource(manifest, resource)
		if strings.TrimSpace(strings.ToLower(authReq.Type)) != "managed_oauth" {
			continue
		}
		var client OAuthClient
		var ok bool
		if strings.TrimSpace(authReq.ClientRef) != "" {
			client, ok = s.oauthClients.Get(authReq.ClientRef)
		}
		if strings.TrimSpace(authReq.ClientRef) != "" && (!ok || strings.TrimSpace(client.ClientID) == "") {
			return nil, fmt.Errorf("OAuth client %q is not configured", authReq.ClientRef)
		}
		connID := strings.TrimSpace(resourceByKey[resource.Key])
		if connID == "" {
			return nil, fmt.Errorf("OAuth MCP resource %q is not installed", resource.Key)
		}
		if strings.TrimSpace(callbackURL) == "" {
			callbackURL = client.RedirectURI
		}
		if strings.TrimSpace(client.AuthorizationEndpoint) != "" && strings.TrimSpace(client.TokenEndpoint) != "" {
			if err := s.oauthService.SaveDiscovery(ctx, connID, &mcp.DiscoveryResult{
				AuthorizationServerURL: authorizationServerFromEndpoint(client.AuthorizationEndpoint),
				AuthorizationEndpoint:  client.AuthorizationEndpoint,
				TokenEndpoint:          client.TokenEndpoint,
				ScopesSupported:        authReq.Scopes,
				ResourceURI:            strings.TrimSpace(resource.URL),
			}); err != nil {
				return nil, err
			}
		} else {
			result, err := s.oauthService.Discover(ctx, resource.URL)
			if err != nil {
				return nil, err
			}
			applyRequestedScopes(result, authReq.Scopes)
			if err := s.oauthService.SaveDiscovery(ctx, connID, result); err != nil {
				return nil, err
			}
		}
		return s.oauthService.StartAuthorization(ctx, connID, client.ClientID, client.ClientSecret, callbackURL)
	}
	return nil, errors.New("plugin does not declare a managed OAuth MCP resource")
}

func (s *Service) RefreshOAuthStatus(ctx context.Context, botID, installationID string) (Installation, error) {
	var result Installation
	err := s.withBotMutation(ctx, botID, func(scopedCtx context.Context, scoped *Service) error {
		return scoped.inTransaction(scopedCtx, func(txService *Service) error {
			var refreshErr error
			result, refreshErr = txService.refreshOAuthStatusAndInstallation(scopedCtx, botID, installationID)
			return refreshErr
		})
	})
	return result, err
}

func (s *Service) refreshOAuthStatusAndInstallation(ctx context.Context, botID, installationID string) (Installation, error) {
	row, err := s.getRow(ctx, botID, installationID)
	if err != nil {
		return Installation{}, err
	}
	manifest, err := decodeManifest(row.Manifest)
	if err != nil {
		return Installation{}, err
	}
	status, err := s.refreshOAuthStatus(ctx, botID, row, manifest)
	if err != nil {
		return Installation{}, err
	}
	enabled := status == StatusReady
	if err := s.mcpService.SetPluginConnectionsActive(ctx, botID, installationID, enabled); err != nil {
		return Installation{}, err
	}
	updated, err := s.updateStatus(ctx, botID, installationID, status, enabled)
	if err != nil {
		return Installation{}, err
	}
	return s.normalizeInstallation(ctx, updated)
}

func (s *Service) getRow(ctx context.Context, botID, installationID string) (sqlc.BotPluginInstallation, error) {
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return sqlc.BotPluginInstallation{}, err
	}
	installationUUID, err := db.ParseUUID(installationID)
	if err != nil {
		return sqlc.BotPluginInstallation{}, err
	}
	return s.queries.GetBotPluginInstallationByID(ctx, sqlc.GetBotPluginInstallationByIDParams{
		BotID: botUUID,
		ID:    installationUUID,
	})
}

func (s *Service) updateStatus(ctx context.Context, botID, installationID, status string, enabled bool) (sqlc.BotPluginInstallation, error) {
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return sqlc.BotPluginInstallation{}, err
	}
	installationUUID, err := db.ParseUUID(installationID)
	if err != nil {
		return sqlc.BotPluginInstallation{}, err
	}
	return s.queries.UpdateBotPluginInstallationStatus(ctx, sqlc.UpdateBotPluginInstallationStatusParams{
		BotID:   botUUID,
		ID:      installationUUID,
		Status:  status,
		Enabled: enabled,
	})
}

func (s *Service) normalizeInstallation(ctx context.Context, row sqlc.BotPluginInstallation) (Installation, error) {
	manifest, err := decodeManifest(row.Manifest)
	if err != nil {
		return Installation{}, err
	}
	metadata, err := decodeJSONMap(row.Metadata)
	if err != nil {
		return Installation{}, err
	}
	config, err := decodeJSONMap(row.Config)
	if err != nil {
		return Installation{}, err
	}
	resources, err := s.queries.ListBotPluginResources(ctx, row.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Installation{}, err
	}
	outResources := make([]Resource, 0, len(resources))
	for _, resource := range resources {
		item, err := normalizeResource(resource)
		if err != nil {
			return Installation{}, err
		}
		outResources = append(outResources, item)
	}
	return Installation{
		ID:          row.ID.String(),
		BotID:       row.BotID.String(),
		PluginID:    row.PluginID,
		PluginName:  row.PluginName,
		Version:     row.Version,
		Status:      row.Status,
		Enabled:     row.Enabled,
		Config:      redactConfig(manifest, config),
		Metadata:    metadata,
		Manifest:    manifest,
		Resources:   outResources,
		InstalledAt: timeFromPg(row.InstalledAt),
		UpdatedAt:   timeFromPg(row.UpdatedAt),
	}, nil
}

// OwnsRegistrySkill reports whether an installed Plugin still references one
// canonical Registry Skill path. Disabled and not-yet-ready Plugins retain
// ownership so the Skill cannot be deleted directly.
func OwnsRegistrySkill(installations []Installation, sourcePath string) bool {
	return ownsRegistrySkill(installations, sourcePath, "", false)
}

func OwnsRegistrySkillAtTarget(
	installations []Installation,
	sourcePath, workspaceTargetID string,
) bool {
	return ownsRegistrySkill(installations, sourcePath, strings.TrimSpace(workspaceTargetID), true)
}

func ownsRegistrySkill(
	installations []Installation,
	sourcePath, workspaceTargetID string,
	matchTarget bool,
) bool {
	sourcePath = path.Clean(strings.TrimSpace(sourcePath))
	if _, _, _, ok := skillset.RegistrySkillIDs(sourcePath); !ok {
		return false
	}
	for _, installation := range installations {
		if installation.Status == StatusUninstalled {
			continue
		}
		if matchTarget {
			installedTarget, _ := installation.Metadata["workspace_target_id"].(string)
			if strings.TrimSpace(installedTarget) != workspaceTargetID {
				continue
			}
		}
		for _, resource := range installation.Resources {
			if resource.Type == "skill" && path.Clean(strings.TrimSpace(resource.ResourceID)) == sourcePath {
				return true
			}
		}
	}
	return false
}

// CheckSkillArtifactConflicts rejects an update when another installed Plugin
// owns the same Registry Skill identity at a different immutable Artifact.
func (s *Service) CheckSkillArtifactConflicts(
	ctx context.Context,
	botID, targetPluginID, workspaceTargetID string,
	expected map[string]string,
) error {
	if len(expected) == 0 && strings.TrimSpace(targetPluginID) == "" {
		return nil
	}
	installations, err := s.List(ctx, botID)
	if err != nil {
		return fmt.Errorf("list Plugin Skill owners: %w", err)
	}
	return checkSkillArtifactConflicts(installations, targetPluginID, workspaceTargetID, expected)
}

func checkSkillArtifactConflicts(
	installations []Installation,
	targetPluginID, workspaceTargetID string,
	expected map[string]string,
) error {
	for identity, digest := range expected {
		if !artifactDigestPattern.MatchString(digest) {
			return fmt.Errorf("plugin Skill %q expected Artifact digest is invalid", identity)
		}
	}
	for _, installation := range installations {
		if installation.Status == StatusUninstalled {
			continue
		}
		installedTarget, _ := installation.Metadata["workspace_target_id"].(string)
		installedTarget = strings.TrimSpace(installedTarget)
		expectedTarget := strings.TrimSpace(workspaceTargetID)
		if targetPluginID != "" && installation.PluginID == targetPluginID {
			if installedTarget != expectedTarget {
				return fmt.Errorf(
					"plugin %q is installed on workspace target %q; uninstall it before installing on %q",
					targetPluginID, installedTarget, expectedTarget,
				)
			}
			continue
		}
		if installedTarget != expectedTarget {
			continue
		}
		for _, resource := range installation.Resources {
			if resource.Type != "skill" {
				continue
			}
			expectedDigest, shared := expected[resource.Key]
			if !shared {
				continue
			}
			actualDigest, ok := resource.Metadata["artifact_digest"].(string)
			if !ok || !artifactDigestPattern.MatchString(actualDigest) {
				return fmt.Errorf("plugin Skill %q owner %q has unverifiable Artifact metadata", resource.Key, installation.PluginID)
			}
			if actualDigest != expectedDigest {
				return fmt.Errorf("plugin Skill %q owner %q requires a different Artifact", resource.Key, installation.PluginID)
			}
		}
	}
	return nil
}

func (s *Service) evaluateInitialStatus(manifest Manifest, variables map[string]string) string {
	status := StatusReady
	for _, resource := range manifest.MCPs {
		authReq := manifestAuthForResource(manifest, resource)
		switch strings.TrimSpace(strings.ToLower(authReq.Type)) {
		case "managed_oauth":
			if strings.TrimSpace(authReq.ClientRef) != "" && !s.oauthClients.HasUsableClient(authReq.ClientRef) {
				return StatusAdminRequired
			}
			status = StatusNeedsAuth
		case "user_secret":
			if missingRequiredVariables(manifest, resource, authReq, variables) {
				return StatusNeedsConfig
			}
		}
		if missingResourceConfig(manifest, resource, variables) {
			return StatusNeedsConfig
		}
	}
	return status
}

func (s *Service) refreshOAuthStatus(ctx context.Context, botID string, row sqlc.BotPluginInstallation, manifest Manifest) (string, error) {
	resources, err := s.queries.ListBotPluginResources(ctx, row.ID)
	if err != nil {
		return "", err
	}
	resourceByKey := map[string]string{}
	for _, resource := range resources {
		if strings.TrimSpace(resource.ResourceType) == "mcp" {
			resourceByKey[resource.ResourceKey] = resource.ResourceID
		}
	}
	hasManagedOAuth := false
	for _, resource := range manifest.MCPs {
		authReq := manifestAuthForResource(manifest, resource)
		if strings.TrimSpace(strings.ToLower(authReq.Type)) != "managed_oauth" {
			continue
		}
		hasManagedOAuth = true
		if strings.TrimSpace(authReq.ClientRef) != "" && !s.oauthClients.HasUsableClient(authReq.ClientRef) {
			return StatusAdminRequired, nil
		}
		connID := strings.TrimSpace(resourceByKey[resource.Key])
		if connID == "" {
			return StatusNeedsAuth, nil
		}
		status, err := s.oauthService.GetStatus(ctx, connID)
		if err != nil {
			s.logger.Warn("failed to get plugin OAuth status", slog.String("bot_id", botID), slog.String("installation_id", row.ID.String()), slog.Any("error", err))
			return StatusNeedsAuth, nil
		}
		if !status.HasToken || status.Expired {
			return StatusNeedsAuth, nil
		}
	}
	if hasManagedOAuth {
		return StatusReady, nil
	}
	return row.Status, nil
}

func normalizeResource(row sqlc.BotPluginResource) (Resource, error) {
	metadata, err := decodeJSONMap(row.Metadata)
	if err != nil {
		return Resource{}, err
	}
	return Resource{
		ID:         row.ID.String(),
		Type:       row.ResourceType,
		Key:        row.ResourceKey,
		ResourceID: row.ResourceID,
		Status:     row.Status,
		Metadata:   metadata,
		CreatedAt:  timeFromPg(row.CreatedAt),
		UpdatedAt:  timeFromPg(row.UpdatedAt),
	}, nil
}

func normalizeManifest(manifest Manifest) Manifest {
	manifest.ID = sanitizeID(manifest.ID)
	manifest.Name = strings.TrimSpace(manifest.Name)
	manifest.Version = strings.TrimSpace(manifest.Version)
	if manifest.Version == "" {
		manifest.Version = "0.1.0"
	}
	if manifest.SchemaVersion == "" {
		manifest.SchemaVersion = "1"
	}
	manifest.Install = normalizeInstallCommands(manifest.Install)
	for i := range manifest.MCPs {
		manifest.MCPs[i].Key = sanitizeID(manifest.MCPs[i].Key)
		if manifest.MCPs[i].Key == "" {
			manifest.MCPs[i].Key = "mcp"
		}
		if manifest.MCPs[i].Name == "" {
			manifest.MCPs[i].Name = manifest.MCPs[i].DisplayName
		}
	}
	for i := range manifest.Skills {
		manifest.Skills[i].RegistryID = strings.TrimSpace(manifest.Skills[i].RegistryID)
		manifest.Skills[i].PackageID = strings.TrimSpace(manifest.Skills[i].PackageID)
		manifest.Skills[i].SkillID = strings.TrimSpace(manifest.Skills[i].SkillID)
	}
	return manifest
}

func NormalizeManifest(manifest Manifest) Manifest {
	return normalizeManifest(manifest)
}

func normalizeInstallCommands(commands []string) InstallCommands {
	out := make([]string, 0, len(commands))
	for _, command := range commands {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		out = append(out, command)
	}
	return InstallCommands(out)
}

func manifestAuthForResource(manifest Manifest, resource MCPResource) AuthRequirement {
	authRef := strings.TrimSpace(resource.AuthRef)
	if authRef != "" {
		for _, req := range manifest.AuthRequirements {
			if strings.TrimSpace(req.Key) == authRef {
				return req
			}
		}
	}
	if len(manifest.AuthRequirements) == 1 {
		return manifest.AuthRequirements[0]
	}
	return AuthRequirement{Key: "anonymous", Type: "none"}
}

func buildMCPConnectionRequest(manifest Manifest, resource MCPResource, authReq AuthRequirement, variables map[string]string) mcp.UpsertRequest {
	resolved := resolveVariables(manifest, resource, variables)
	headers := map[string]string{}
	for _, header := range resource.Headers {
		value := resolveConfigValue(header, resolved)
		if value != "" {
			headers[header.Key] = expandTemplateVars(value, resolved)
		}
	}
	env := map[string]string{}
	for _, item := range resource.Env {
		value := resolveConfigValue(item, resolved)
		if value != "" {
			env[item.Key] = expandTemplateVars(value, resolved)
		}
	}
	args := make([]string, 0, len(resource.Args))
	for _, arg := range resource.Args {
		args = append(args, expandTemplateVars(arg, resolved))
	}
	authType := "none"
	if strings.TrimSpace(strings.ToLower(authReq.Type)) == "managed_oauth" {
		authType = "oauth"
	}
	return mcp.UpsertRequest{
		Name:      stableResourceName(manifest.ID, resource.Key),
		Command:   expandTemplateVars(resource.Command, resolved),
		Args:      args,
		Env:       env,
		Cwd:       expandTemplateVars(resource.Cwd, resolved),
		URL:       expandTemplateVars(resource.URL, resolved),
		Headers:   headers,
		Transport: resource.Transport,
		AuthType:  authType,
	}
}

func resolveVariables(manifest Manifest, resource MCPResource, variables map[string]string) map[string]string {
	resolved := map[string]string{}
	for key, value := range variables {
		key = strings.TrimSpace(key)
		if key != "" {
			resolved[key] = value
		}
	}
	for _, item := range manifest.Variables {
		seedDefaultVariable(resolved, item)
	}
	for _, item := range resource.Env {
		seedDefaultVariable(resolved, item)
	}
	for _, item := range resource.Headers {
		seedDefaultVariable(resolved, item)
	}
	return resolved
}

func seedDefaultVariable(resolved map[string]string, item ConfigVar) {
	key := strings.TrimSpace(item.Key)
	if key == "" {
		return
	}
	if _, ok := resolved[key]; ok {
		return
	}
	value := strings.TrimSpace(item.DefaultValue)
	if value == "" {
		return
	}
	value = expandTemplateVars(value, resolved)
	if hasUnresolvedTemplateVars(value) {
		return
	}
	resolved[key] = value
}

func resolveConfigValue(item ConfigVar, variables map[string]string) string {
	key := strings.TrimSpace(item.Key)
	if key == "" {
		return ""
	}
	if value, ok := variables[key]; ok {
		return value
	}
	value := strings.TrimSpace(item.DefaultValue)
	if value == "" {
		return ""
	}
	value = expandTemplateVars(value, variables)
	if hasUnresolvedTemplateVars(value) {
		return ""
	}
	return value
}

func missingRequiredVariables(manifest Manifest, resource MCPResource, authReq AuthRequirement, variables map[string]string) bool {
	resolved := resolveVariables(manifest, resource, variables)
	for _, key := range authReq.Variables {
		if strings.TrimSpace(resolved[strings.TrimSpace(key)]) == "" {
			return true
		}
	}
	return false
}

func missingResourceConfig(manifest Manifest, resource MCPResource, variables map[string]string) bool {
	resolved := resolveVariables(manifest, resource, variables)
	for _, item := range append(resource.Env, resource.Headers...) {
		if !item.Required {
			continue
		}
		if strings.TrimSpace(resolveConfigValue(item, resolved)) == "" {
			return true
		}
	}
	return false
}

func resourceStatus(installationStatus string, authReq AuthRequirement) string {
	if strings.TrimSpace(strings.ToLower(authReq.Type)) == "managed_oauth" && installationStatus == StatusNeedsAuth {
		return StatusNeedsAuth
	}
	return installationStatus
}

func applyRequestedScopes(result *mcp.DiscoveryResult, scopes []string) {
	if result == nil || len(scopes) == 0 {
		return
	}
	result.ScopesSupported = scopes
}

func manifestMetadata(manifest Manifest) map[string]any {
	return map[string]any{
		"icon":         manifest.Icon,
		"tags":         manifest.Tags,
		"capabilities": manifest.Capabilities,
		"homepage":     manifest.Homepage,
	}
}

func mcpResourceMetadata(manifest Manifest, resource MCPResource, authReq AuthRequirement) map[string]any {
	return map[string]any{
		"plugin_id":    manifest.ID,
		"plugin_name":  manifest.Name,
		"plugin_icon":  manifest.Icon,
		"resource_key": resource.Key,
		"display_name": resource.DisplayName,
		"capabilities": resource.Capabilities,
		"auth_type":    authReq.Type,
		"client_ref":   authReq.ClientRef,
		"tool_prefix":  stableResourceName(manifest.ID, resource.Key),
		"visibility":   resource.Visibility,
	}
}

func redactConfig(manifest Manifest, config map[string]any) map[string]any {
	rawVariables, _ := config["variables"].(map[string]any)
	variableStatus := map[string]any{}
	for _, item := range manifest.Variables {
		if item.Key == "" {
			continue
		}
		_, configured := rawVariables[item.Key]
		variableStatus[item.Key] = map[string]bool{"configured": configured}
	}
	return map[string]any{"variables": variableStatus}
}

func variablesFromConfig(raw []byte) (map[string]string, error) {
	config, err := decodeJSONMap(raw)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	switch variables := config["variables"].(type) {
	case map[string]any:
		for key, value := range variables {
			key = strings.TrimSpace(key)
			if key == "" || value == nil {
				continue
			}
			switch typed := value.(type) {
			case string:
				out[key] = typed
			default:
				out[key] = fmt.Sprint(typed)
			}
		}
	case map[string]string:
		for key, value := range variables {
			key = strings.TrimSpace(key)
			if key != "" {
				out[key] = value
			}
		}
	}
	return out, nil
}

func encodeJSON(value any) ([]byte, error) {
	if value == nil {
		value = map[string]any{}
	}
	return json.Marshal(value)
}

func mustJSON(value any) []byte {
	payload, _ := encodeJSON(value)
	return payload
}

func decodeJSONMap(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func decodeManifest(raw []byte) (Manifest, error) {
	if len(raw) == 0 {
		return Manifest{}, nil
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return Manifest{}, err
	}
	return normalizeManifest(manifest), nil
}

func timeFromPg(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return db.TimeFromPg(value)
}

func sanitizeID(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	return idPattern.ReplaceAllString(value, "_")
}

func stableResourceName(pluginID, resourceKey string) string {
	name := sanitizeID(pluginID + "_" + resourceKey)
	if name == "" {
		return "plugin_mcp"
	}
	return name
}

func expandTemplateVars(value string, vars map[string]string) string {
	if value == "" || len(vars) == 0 {
		return value
	}
	return templateVarPattern.ReplaceAllStringFunc(value, func(match string) string {
		key := match[2 : len(match)-1]
		if val, ok := vars[key]; ok {
			return val
		}
		return match
	})
}

func hasUnresolvedTemplateVars(value string) bool {
	return templateVarPattern.MatchString(value)
}

func authorizationServerFromEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	idx := strings.Index(endpoint, "/oauth")
	if idx > len("https://") {
		return endpoint[:idx]
	}
	return endpoint
}

var (
	idPattern             = regexp.MustCompile(`[^a-z0-9_-]+`)
	artifactDigestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
	templateVarPattern    = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
)

func SkillReferenceIdentity(reference SkillReference) string {
	return reference.RegistryID + "/" + reference.PackageID + "/" + reference.SkillID
}

func ValidateSkillReferences(references []SkillReference) error {
	seen := make(map[string]struct{}, len(references))
	for _, reference := range references {
		identity := SkillReferenceIdentity(reference)
		if !skillset.IsValidRegistryID(reference.RegistryID) ||
			!skillset.IsValidRegistryComponent(reference.PackageID) ||
			!skillset.IsValidRegistryComponent(reference.SkillID) {
			return fmt.Errorf("plugin Skill reference %q is invalid", identity)
		}
		if _, ok := seen[identity]; ok {
			return fmt.Errorf("plugin Skill reference %q is duplicated", identity)
		}
		seen[identity] = struct{}{}
	}
	return nil
}

func validateReleaseMetadata(release ReleaseMetadata) error {
	if release.Revision == "" && release.ArtifactDigest == "" {
		return nil
	}
	if !artifactDigestPattern.MatchString(release.Revision) ||
		!artifactDigestPattern.MatchString(release.ArtifactDigest) {
		return errors.New("plugin release metadata is invalid")
	}
	return nil
}

func validateSkillArtifacts(references []SkillReference, artifacts map[string]SkillArtifactMetadata) error {
	for _, reference := range references {
		identity := SkillReferenceIdentity(reference)
		artifact, ok := artifacts[identity]
		if !ok {
			return fmt.Errorf("plugin Skill %q was not installed", identity)
		}
		expectedInstallID := strings.Join([]string{reference.RegistryID, reference.PackageID, reference.SkillID}, "+")
		if !artifactDigestPattern.MatchString(artifact.RegistryRevision) ||
			artifact.InstallID != expectedInstallID ||
			!artifactDigestPattern.MatchString(artifact.ArtifactDigest) ||
			artifact.FilesWritten < 1 {
			return fmt.Errorf("plugin Skill %q installation metadata is invalid", identity)
		}
	}
	return nil
}
