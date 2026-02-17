package channel

import (
	"context"
	"errors"
	"log/slog"
	"strings"
)

type connectionEntry struct {
	config     Config
	connection Connection
}

func (m *Manager) refresh(ctx context.Context) {
	// Serialize refresh calls to prevent concurrent reconcile from starting
	// duplicate adapter connections.
	if !m.refreshMu.TryLock() {
		return
	}
	defer m.refreshMu.Unlock()

	if m.service == nil {
		return
	}
	configs := make([]Config, 0)
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

func (m *Manager) reconcile(ctx context.Context, configs []Config) {
	active := map[string]Config{}
	for _, cfg := range configs {
		if cfg.ID == "" {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(cfg.Status))
		if status != "" && status != "active" && status != "verified" {
			continue
		}
		active[cfg.ID] = cfg
		if err := m.ensureConnection(ctx, cfg); err != nil {
			if m.logger != nil {
				m.logger.Error("adapter start failed", slog.String("channel", cfg.Type.String()), slog.String("config_id", cfg.ID), slog.Any("error", err))
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
				m.logger.Info("adapter stop", slog.String("channel", entry.config.Type.String()), slog.String("config_id", id))
			}
			if err := entry.connection.Stop(ctx); err != nil && !errors.Is(err, ErrStopNotSupported) && m.logger != nil {
				m.logger.Warn("adapter stop failed", slog.String("config_id", id), slog.Any("error", err))
			}
		}
		delete(m.connections, id)
	}
}

func (m *Manager) ensureConnection(ctx context.Context, cfg Config) error {
	_, ok := m.registry.GetReceiver(cfg.Type)
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
			m.logger.Info("adapter restart", slog.String("channel", cfg.Type.String()), slog.String("config_id", cfg.ID))
		}
		if err := oldConn.Stop(ctx); err != nil {
			if errors.Is(err, ErrStopNotSupported) {
				if m.logger != nil {
					m.logger.Warn("adapter restart skipped", slog.String("channel", cfg.Type.String()), slog.String("config_id", cfg.ID))
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

	receiver, ok := m.registry.GetReceiver(cfg.Type)
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
		m.logger.Info("adapter start", slog.String("channel", cfg.Type.String()), slog.String("config_id", cfg.ID))
	}
	handler := m.handleInbound
	for i := len(m.middlewares) - 1; i >= 0; i-- {
		handler = m.middlewares[i](handler)
	}
	conn, err := receiver.Connect(ctx, cfg, handler)
	if err != nil {
		return err
	}

	m.mu.Lock()
	// Final check: if another goroutine raced and inserted first, stop our new
	// connection and keep the existing one.
	if existing, ok := m.connections[cfg.ID]; ok && existing != nil {
		m.mu.Unlock()
		_ = conn.Stop(ctx)
		return nil
	}
	m.connections[cfg.ID] = &connectionEntry{
		config:     cfg,
		connection: conn,
	}
	m.mu.Unlock()
	return nil
}

func (m *Manager) stopAll(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, entry := range m.connections {
		if entry != nil && entry.connection != nil {
			if m.logger != nil {
				m.logger.Info("adapter stop", slog.String("channel", entry.config.Type.String()), slog.String("config_id", id))
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
		return errors.New("config id is required")
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
		return errors.New("bot id is required")
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
