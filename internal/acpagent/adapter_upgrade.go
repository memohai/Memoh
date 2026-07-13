package acpagent

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/memohai/memoh/internal/acpclient"
	"github.com/memohai/memoh/internal/acpprofile"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

const (
	dynamicAdapterStartTimeout = 90 * time.Second
	containerToolkitCABundle   = "/opt/memoh/toolkit/certs/ca-certificates.crt"
)

type adapterVersionResolver interface {
	ResolveACPAdapterVersion(ctx context.Context, botID, packageName string, env []string) (string, error)
}

type adapterUpgradeState struct {
	once     sync.Once
	version  string
	err      error
	disabled bool
}

// resolveDynamicAdapter performs at most one registry lookup per bot and
// package during the lifetime of this server process. The exact result is
// shared by all later cold starts; failures use the bundled adapter until the
// server restarts.
func (p *SessionPool) resolveDynamicAdapter(
	ctx context.Context,
	botID string,
	packageName string,
	env []string,
) (*adapterUpgradeState, string, error) {
	resolver, ok := p.runner.(adapterVersionResolver)
	if !ok {
		return nil, "", nil
	}
	key := botID + "|" + packageName
	p.adapterMu.Lock()
	if p.adapterStates == nil {
		p.adapterStates = make(map[string]*adapterUpgradeState)
	}
	state := p.adapterStates[key]
	if state == nil {
		state = &adapterUpgradeState{}
		p.adapterStates[key] = state
	}
	p.adapterMu.Unlock()
	state.once.Do(func() {
		state.version, state.err = resolver.ResolveACPAdapterVersion(ctx, botID, packageName, env)
	})
	p.adapterMu.Lock()
	disabled := state.disabled
	p.adapterMu.Unlock()
	if disabled {
		return state, "", nil
	}
	return state, state.version, state.err
}

func (p *SessionPool) disableDynamicAdapter(state *adapterUpgradeState) {
	if state == nil {
		return
	}
	p.adapterMu.Lock()
	state.disabled = true
	p.adapterMu.Unlock()
}

func (p *SessionPool) startDynamicAdapter(
	startCtx context.Context,
	profile acpprofile.Profile,
	workspaceInfo bridge.WorkspaceInfo,
	startReq acpclient.StartRequest,
	sink acpclient.EventSink,
) (*acpclient.Session, error) {
	packageName := strings.TrimSpace(profile.DynamicPackage)
	if strings.TrimSpace(profile.DynamicCommand) == "" || packageName == "" {
		return nil, nil
	}

	timeout := p.dynamicAdapterStartTimeout
	if timeout <= 0 {
		timeout = dynamicAdapterStartTimeout
	}
	dynamicCtx, cancel := context.WithTimeout(startCtx, timeout)
	defer cancel()
	useToolkitCA := p.containerToolkitCABundleAvailable(dynamicCtx, startReq.BotID, workspaceInfo)
	dynamicEnv := dynamicACPEnv(startReq.Env, workspaceInfo, useToolkitCA)
	state, version, resolveErr := p.resolveDynamicAdapter(dynamicCtx, startReq.BotID, packageName, adapterLookupEnv(dynamicEnv))
	if resolveErr != nil && startCtx.Err() != nil {
		return nil, startCtx.Err()
	}
	if resolveErr != nil {
		p.disableDynamicAdapter(state)
	}
	if version == "" || dynamicCtx.Err() != nil {
		if err := dynamicCtx.Err(); err != nil {
			return nil, err
		}
		return nil, resolveErr
	}

	dynamicReq := startReq
	dynamicReq.Command = profile.DynamicCommand
	dynamicReq.Args = append(append([]string(nil), profile.DynamicArgs...), packageName+"@"+version)
	dynamicReq.LocalCommand = profile.DynamicCommand
	dynamicReq.LocalArgs = append([]string(nil), dynamicReq.Args...)
	dynamicReq.Env = dynamicEnv

	sess, err := p.runner.StartSession(dynamicCtx, dynamicReq, sink)
	if err == nil && sess != nil {
		return sess, nil
	}
	if startCtx.Err() != nil {
		if err != nil {
			return nil, err
		}
		return nil, startCtx.Err()
	}
	p.disableDynamicAdapter(state)
	if err == nil {
		err = errors.New("dynamic ACP adapter returned no session")
	}
	return nil, err
}

func (p *SessionPool) containerToolkitCABundleAvailable(ctx context.Context, botID string, workspaceInfo bridge.WorkspaceInfo) bool {
	if !strings.EqualFold(strings.TrimSpace(workspaceInfo.Backend), bridge.WorkspaceBackendContainer) {
		return false
	}
	runner, ok := p.runner.(workspaceClientRunner)
	if !ok {
		return false
	}
	client, err := runner.MCPClient(ctx, botID)
	if err != nil {
		p.logger.Debug("could not inspect workspace CA bundle", slog.String("bot_id", botID), slog.Any("error", err))
		return false
	}
	if _, err := client.Stat(ctx, containerToolkitCABundle); err != nil {
		if !errors.Is(err, bridge.ErrNotFound) {
			p.logger.Debug("could not inspect workspace CA bundle", slog.String("bot_id", botID), slog.Any("error", err))
		}
		return false
	}
	return true
}

func dynamicACPEnv(env []string, workspaceInfo bridge.WorkspaceInfo, useToolkitCA bool) []string {
	cacheDir := "/data/.memoh/acp/npm-cache"
	if strings.EqualFold(strings.TrimSpace(workspaceInfo.Backend), bridge.WorkspaceBackendLocal) {
		localDataRoot := strings.TrimSpace(workspaceInfo.LocalDataRoot)
		if localDataRoot == "" {
			return append([]string(nil), env...)
		}
		cacheDir = filepath.Join(localDataRoot, "acp", "npm-cache")
	}

	result := replaceEnvValue(env, "NPM_CONFIG_CACHE", cacheDir)
	if useToolkitCA && !envHasKey(result, "SSL_CERT_FILE") {
		result = append(result, "SSL_CERT_FILE="+containerToolkitCABundle)
	}
	return result
}

func replaceEnvValue(env []string, targetKey, value string) []string {
	result := make([]string, 0, len(env)+1)
	for _, item := range env {
		key, _, ok := strings.Cut(item, "=")
		if ok && strings.EqualFold(strings.TrimSpace(key), targetKey) {
			continue
		}
		result = append(result, item)
	}
	return append(result, targetKey+"="+value)
}

func envHasKey(env []string, targetKey string) bool {
	for _, item := range env {
		key, _, ok := strings.Cut(item, "=")
		if ok && strings.EqualFold(strings.TrimSpace(key), targetKey) {
			return true
		}
	}
	return false
}

func adapterLookupEnv(env []string) []string {
	result := make([]string, 0, 2)
	for _, item := range env {
		key, _, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		switch {
		case strings.EqualFold(strings.TrimSpace(key), "NPM_CONFIG_CACHE"):
			result = append(result, item)
		case strings.EqualFold(strings.TrimSpace(key), "SSL_CERT_FILE"):
			result = append(result, item)
		}
	}
	return result
}
