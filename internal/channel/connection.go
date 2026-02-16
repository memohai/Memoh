package channel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

type connectionEntry struct {
	config     ChannelConfig
	connection Connection
}

func (m *Manager) refresh(ctx context.Context) {
	// Serialize refresh calls so concurrent callers wait instead of silently skipping.
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()

	if m.service == nil {
		return
	}
	configs := make([]ChannelConfig, 0)
	for _, channelType := range m.registry.Types() {
		items, err := m.service.ListConfigsByType(ctx, channelType)
		if err != nil {
			if m.logger != nil {
				m.logger.Error("list configs failed", slog.String("channel", channelType.String()), slog.Any("error", err))
			}
			continue
		}
		configs = append(configs, items...)
	}
	m.reconcile(ctx, configs)
}

func (m *Manager) reconcile(ctx context.Context, configs []ChannelConfig) {
	active := map[string]ChannelConfig{}
	for _, cfg := range configs {
		if cfg.ID == "" || cfg.Disabled {
			continue
		}
		active[cfg.ID] = cfg
		if err := m.ensureConnection(ctx, cfg); err != nil {
			if m.logger != nil {
				m.logger.Error("adapter start failed", slog.String("channel", cfg.ChannelType.String()), slog.String("config_id", cfg.ID), slog.Any("error", err))
			}
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for id, entry := range m.connections {
		if _, ok := active[id]; ok {
			continue
		}
		if entry != nil && entry.connection != nil {
			if m.logger != nil {
				m.logger.Info("adapter stop", slog.String("channel", entry.config.ChannelType.String()), slog.String("config_id", id))
			}
			if err := entry.connection.Stop(ctx); err != nil && !errors.Is(err, ErrStopNotSupported) && m.logger != nil {
				m.logger.Warn("adapter stop failed", slog.String("config_id", id), slog.Any("error", err))
			}
		}
		delete(m.connections, id)
	}
}

func (m *Manager) ensureConnection(ctx context.Context, cfg ChannelConfig) error {
	_, ok := m.registry.GetReceiver(cfg.ChannelType)
	if !ok {
		return nil
	}

	m.mu.Lock()
	entry := m.connections[cfg.ID]

	// Config unchanged — nothing to do.
	if entry != nil && !entry.config.UpdatedAt.Before(cfg.UpdatedAt) {
		m.mu.Unlock()
		return nil
	}

	// Need to stop existing connection before starting a new one.
	// Keep the lock to prevent another goroutine from starting a duplicate.
	var oldConn Connection
	if entry != nil {
		oldConn = entry.connection
		delete(m.connections, cfg.ID)
	}
	m.mu.Unlock()

	if oldConn != nil {
		if m.logger != nil {
			m.logger.Info("adapter restart", slog.String("channel", cfg.ChannelType.String()), slog.String("config_id", cfg.ID))
		}
		if err := oldConn.Stop(ctx); err != nil {
			if errors.Is(err, ErrStopNotSupported) {
				if m.logger != nil {
					m.logger.Warn("adapter restart skipped", slog.String("channel", cfg.ChannelType.String()), slog.String("config_id", cfg.ID))
				}
				// Re-insert the entry since we can't restart it.
				m.mu.Lock()
				if _, exists := m.connections[cfg.ID]; !exists {
					m.connections[cfg.ID] = entry
				}
				m.mu.Unlock()
				return nil
			}
			return err
		}
	}

	receiver, ok := m.registry.GetReceiver(cfg.ChannelType)
	if !ok {
		return nil
	}

	// Double-check: another goroutine may have already started a connection
	// for this config while we were stopping the old one.
	m.mu.Lock()
	if existing, ok := m.connections[cfg.ID]; ok && existing != nil {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	if m.logger != nil {
		m.logger.Info("adapter start", slog.String("channel", cfg.ChannelType.String()), slog.String("config_id", cfg.ID))
	}
	handler := m.handleInbound
	for i := len(m.middlewares) - 1; i >= 0; i-- {
		handler = m.middlewares[i](handler)
	}
	connectCtx := context.Background()
	if ctx != nil {
		// Decouple long-lived adapter connections from short-lived request contexts.
		connectCtx = context.WithoutCancel(ctx)
	}
	conn, err := receiver.Connect(connectCtx, cfg, handler)
	if err != nil {
		return err
	}

	m.mu.Lock()
	// Final check: if another goroutine raced and inserted first, stop our new
	// connection and keep the existing one.
	if existing, ok := m.connections[cfg.ID]; ok && existing != nil {
		m.mu.Unlock()
		_ = conn.Stop(context.Background())
		return nil
	}
	m.connections[cfg.ID] = &connectionEntry{
		config:     cfg,
		connection: conn,
	}
	m.mu.Unlock()
	return nil
}

// EnsureConnection starts, restarts, or stops the connection for the given config.
// Disabled configs are stopped and removed; enabled configs are started or restarted.
func (m *Manager) EnsureConnection(ctx context.Context, cfg ChannelConfig) error {
	if cfg.ID == "" {
		return fmt.Errorf("config id is required")
	}
	if cfg.Disabled {
		return m.removeConnection(ctx, cfg.ID)
	}
	return m.ensureConnection(ctx, cfg)
}

// RemoveConnection stops and removes connections matching the given bot and channel type.
func (m *Manager) RemoveConnection(ctx context.Context, botID string, channelType ChannelType) {
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, entry := range m.connections {
		if entry == nil || entry.config.BotID != botID || entry.config.ChannelType != channelType {
			continue
		}
		if entry.connection != nil {
			if m.logger != nil {
				m.logger.Info("connection remove", slog.String("channel", channelType.String()), slog.String("config_id", id))
			}
			if err := entry.connection.Stop(ctx); err != nil && !errors.Is(err, ErrStopNotSupported) && m.logger != nil {
				m.logger.Warn("connection stop failed", slog.String("config_id", id), slog.Any("error", err))
			}
		}
		delete(m.connections, id)
	}
}

func (m *Manager) removeConnection(ctx context.Context, configID string) error {
	m.mu.Lock()
	entry := m.connections[configID]
	if entry == nil {
		m.mu.Unlock()
		return nil
	}
	delete(m.connections, configID)
	m.mu.Unlock()

	if entry.connection != nil {
		if m.logger != nil {
			m.logger.Info("connection remove", slog.String("channel", entry.config.ChannelType.String()), slog.String("config_id", configID))
		}
		if err := entry.connection.Stop(ctx); err != nil && !errors.Is(err, ErrStopNotSupported) {
			if m.logger != nil {
				m.logger.Warn("connection stop failed", slog.String("config_id", configID), slog.Any("error", err))
			}
			return err
		}
	}
	return nil
}

func (m *Manager) stopAll(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, entry := range m.connections {
		if entry != nil && entry.connection != nil {
			if m.logger != nil {
				m.logger.Info("adapter stop", slog.String("channel", entry.config.ChannelType.String()), slog.String("config_id", id))
			}
			if err := entry.connection.Stop(ctx); err != nil && !errors.Is(err, ErrStopNotSupported) && m.logger != nil {
				m.logger.Warn("adapter stop failed", slog.String("config_id", id), slog.Any("error", err))
			}
		}
		delete(m.connections, id)
	}
}

// Stop terminates the connection identified by the given config ID.
func (m *Manager) Stop(ctx context.Context, configID string) error {
	configID = strings.TrimSpace(configID)
	if configID == "" {
		return fmt.Errorf("config id is required")
	}
	m.mu.Lock()
	entry := m.connections[configID]
	m.mu.Unlock()
	if entry == nil || entry.connection == nil {
		return nil
	}
	return entry.connection.Stop(ctx)
}

// StopByBot terminates all connections belonging to the given bot.
func (m *Manager) StopByBot(ctx context.Context, botID string) error {
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return fmt.Errorf("bot id is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, entry := range m.connections {
		if entry != nil && entry.config.BotID == botID {
			if entry.connection != nil {
				_ = entry.connection.Stop(ctx)
			}
			delete(m.connections, id)
		}
	}
	return nil
}
