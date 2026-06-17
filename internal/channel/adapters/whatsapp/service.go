package whatsapp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/memohai/memoh/internal/channel"
)

const pendingTTL = 5 * time.Minute

const (
	pendingModeQR    = "qr"
	pendingModePhone = "phone"
)

var (
	ErrLoginNotFound      = errors.New("whatsapp login not found")
	ErrPhonePairingFailed = errors.New("whatsapp phone pairing failed")
	replaceWhatsAppStore  = replaceStore
)

type channelStore interface {
	ResolveEffectiveConfig(ctx context.Context, botID string, channelType channel.ChannelType) (channel.ChannelConfig, error)
	UpsertConfig(ctx context.Context, botID string, channelType channel.ChannelType, req channel.UpsertConfigRequest) (channel.ChannelConfig, error)
	DeleteConfig(ctx context.Context, botID string, channelType channel.ChannelType) error
}

type channelLifecycle interface {
	SetBotChannelStatus(ctx context.Context, botID string, channelType channel.ChannelType, disabled bool) (channel.ChannelConfig, error)
}

type connectionStopper interface {
	Stop(ctx context.Context, configID string) error
}

type Service struct {
	logger    *slog.Logger
	dataRoot  string
	store     channelStore
	lifecycle channelLifecycle
	manager   connectionStopper
	adapter   *Adapter

	mu       sync.Mutex
	configMu sync.Mutex
	pending  map[string]*pendingLogin
	idSource func() string
	now      func() time.Time
}

type pendingLogin struct {
	mu          sync.Mutex
	finishOnce  sync.Once
	expiryTimer *time.Timer
	ID          string
	BotID       string
	Mode        string
	ExpiresAt   time.Time
	Status      string
	QRCode      string
	PairCode    string
	Phone       string
	Timeout     time.Duration
	Error       string
	paired      bool
	cfgID       string

	ctx       context.Context
	cancel    context.CancelFunc
	container interface{ Close() error }
	client    *whatsmeow.Client
	events    <-chan whatsmeow.QRChannelItem
}

type QRStartResponse struct {
	LoginID   string    `json:"login_id"`
	Status    string    `json:"status"`
	QRCode    string    `json:"qr_code,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
	Message   string    `json:"message,omitempty"`
}

type QRPollResponse struct {
	LoginID   string    `json:"login_id"`
	Status    string    `json:"status"`
	QRCode    string    `json:"qr_code,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Message   string    `json:"message,omitempty"`
	ConfigID  string    `json:"config_id,omitempty"`
}

type PhoneStartResponse struct {
	LoginID     string    `json:"login_id"`
	Status      string    `json:"status"`
	PairingCode string    `json:"pairing_code,omitempty"`
	ExpiresAt   time.Time `json:"expires_at"`
	Message     string    `json:"message,omitempty"`
}

type PhonePollResponse struct {
	LoginID     string    `json:"login_id"`
	Status      string    `json:"status"`
	PairingCode string    `json:"pairing_code,omitempty"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
	Message     string    `json:"message,omitempty"`
	ConfigID    string    `json:"config_id,omitempty"`
}

type StatusResponse struct {
	Configured bool                   `json:"configured"`
	ConfigID   string                 `json:"config_id,omitempty"`
	Status     string                 `json:"status"`
	Running    bool                   `json:"running"`
	LastError  string                 `json:"last_error,omitempty"`
	DeviceJID  string                 `json:"device_jid,omitempty"`
	Phone      string                 `json:"phone,omitempty"`
	PushName   string                 `json:"push_name,omitempty"`
	UpdatedAt  time.Time              `json:"updated_at,omitempty"`
	Config     *channel.ChannelConfig `json:"config,omitempty"`
}

type LogoutResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type CancelLoginResponse struct {
	Status string `json:"status"`
}

func NewService(log *slog.Logger, dataRoot string, store channelStore, lifecycle channelLifecycle, manager connectionStopper, adapter *Adapter) *Service {
	if log == nil {
		log = slog.Default()
	}
	svc := &Service{
		logger:    log.With(slog.String("service", "whatsapp")),
		dataRoot:  dataRoot,
		store:     store,
		lifecycle: lifecycle,
		manager:   manager,
		adapter:   adapter,
		pending:   map[string]*pendingLogin{},
		idSource:  newLoginID,
		now:       func() time.Time { return time.Now().UTC() },
	}
	if adapter != nil {
		adapter.SetTerminalHandler(svc.markNeedsRelink)
	}
	return svc
}

func (s *Service) StartQR(ctx context.Context, botID string) (QRStartResponse, error) {
	if s.store == nil {
		return QRStartResponse{}, errors.New("channel store not configured")
	}
	p, err := s.startPending(ctx, botID, pendingModeQR)
	if err != nil {
		return QRStartResponse{}, err
	}
	return QRStartResponse{
		LoginID:   p.ID,
		Status:    p.Status,
		QRCode:    p.QRCode,
		ExpiresAt: p.ExpiresAt,
		Message:   statusMessage(p.Status),
	}, nil
}

func (s *Service) StartPhone(ctx context.Context, botID, phone string) (PhoneStartResponse, error) {
	if s.store == nil {
		return PhoneStartResponse{}, errors.New("channel store not configured")
	}
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return PhoneStartResponse{}, errors.New("phone is required")
	}
	p, err := s.startPending(ctx, botID, pendingModePhone)
	if err != nil {
		return PhoneStartResponse{}, err
	}
	p.mu.Lock()
	p.Phone = phone
	p.mu.Unlock()
	code, err := p.client.PairPhone(ctx, phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	if err != nil {
		s.finishPending(p.ID, true)
		return PhoneStartResponse{}, errors.Join(ErrPhonePairingFailed, err)
	}
	p.mu.Lock()
	p.PairCode = strings.TrimSpace(code)
	p.Status = "pair_code"
	resp := PhoneStartResponse{
		LoginID:     p.ID,
		Status:      p.Status,
		PairingCode: p.PairCode,
		ExpiresAt:   p.ExpiresAt,
		Message:     statusMessage(p.Status),
	}
	p.mu.Unlock()
	return resp, nil
}

func (s *Service) PollQR(ctx context.Context, botID, loginID string) (QRPollResponse, error) {
	p := s.getPending(loginID)
	if p == nil || p.BotID != strings.TrimSpace(botID) || p.Mode != pendingModeQR {
		return QRPollResponse{}, ErrLoginNotFound
	}
	removePending := false
	p.mu.Lock()
	defer func() {
		p.mu.Unlock()
		if removePending {
			s.finishPending(p.ID, true)
		}
	}()
	if s.now().After(p.ExpiresAt) {
		p.Status = "expired"
		removePending = true
		return qrPollResponseFromPending(p), nil
	}
	s.drainPending(p, 1500*time.Millisecond)
	if p.Status == "success" && !p.paired {
		cfg, err := s.finalizePair(ctx, p)
		if err != nil {
			p.Status = StatusTerminal
			p.Error = err.Error()
			removePending = true
			return QRPollResponse{}, err
		}
		p.paired = true
		p.cfgID = cfg.ID
	}
	resp := qrPollResponseFromPending(p)
	if p.paired || p.Status == "expired" || p.Status == StatusTerminal {
		removePending = true
	}
	return resp, nil
}

func (s *Service) PollPhone(ctx context.Context, botID, loginID string) (PhonePollResponse, error) {
	p := s.getPending(loginID)
	if p == nil || p.BotID != strings.TrimSpace(botID) || p.Mode != pendingModePhone {
		return PhonePollResponse{}, ErrLoginNotFound
	}
	removePending := false
	p.mu.Lock()
	defer func() {
		p.mu.Unlock()
		if removePending {
			s.finishPending(p.ID, true)
		}
	}()
	if s.now().After(p.ExpiresAt) {
		p.Status = "expired"
		removePending = true
		return phonePollResponseFromPending(p), nil
	}
	s.drainPending(p, 1500*time.Millisecond)
	if p.Status == "success" && !p.paired {
		cfg, err := s.finalizePair(ctx, p)
		if err != nil {
			p.Status = StatusTerminal
			p.Error = err.Error()
			removePending = true
			return PhonePollResponse{}, err
		}
		p.paired = true
		p.cfgID = cfg.ID
	}
	resp := phonePollResponseFromPending(p)
	if p.paired || p.Status == "expired" || p.Status == StatusTerminal {
		removePending = true
	}
	return resp, nil
}

func (s *Service) CancelLogin(_ context.Context, botID, loginID string) (CancelLoginResponse, error) {
	p := s.getPending(loginID)
	if p == nil || p.BotID != strings.TrimSpace(botID) {
		return CancelLoginResponse{}, ErrLoginNotFound
	}
	p.mu.Lock()
	if p.paired || p.Status == "success" {
		status := p.Status
		if p.paired {
			status = "paired"
		}
		p.mu.Unlock()
		return CancelLoginResponse{Status: status}, nil
	}
	p.Status = "cancelled"
	p.mu.Unlock()
	s.finishPending(p.ID, true)
	return CancelLoginResponse{Status: "cancelled"}, nil
}

func qrPollResponseFromPending(p *pendingLogin) QRPollResponse {
	resp := QRPollResponse{
		LoginID:   p.ID,
		Status:    p.Status,
		QRCode:    p.QRCode,
		ExpiresAt: p.ExpiresAt,
		Message:   statusMessage(p.Status),
		ConfigID:  p.cfgID,
	}
	if p.Error != "" {
		resp.Message = p.Error
	}
	return resp
}

func phonePollResponseFromPending(p *pendingLogin) PhonePollResponse {
	resp := PhonePollResponse{
		LoginID:     p.ID,
		Status:      p.Status,
		PairingCode: p.PairCode,
		ExpiresAt:   p.ExpiresAt,
		Message:     statusMessage(p.Status),
		ConfigID:    p.cfgID,
	}
	if p.Error != "" {
		resp.Message = p.Error
	}
	return resp
}

func (s *Service) Status(ctx context.Context, botID string) (StatusResponse, error) {
	if s.store == nil {
		return StatusResponse{}, errors.New("channel store not configured")
	}
	cfg, err := s.store.ResolveEffectiveConfig(ctx, botID, Type)
	if err != nil {
		if errors.Is(err, channel.ErrChannelConfigNotFound) || strings.Contains(err.Error(), "not found") {
			return StatusResponse{Configured: false, Status: StatusDisconnected}, nil
		}
		return StatusResponse{}, err
	}
	resp := StatusResponse{
		Configured: true,
		ConfigID:   cfg.ID,
		Status:     StatusDisconnected,
		Config:     &cfg,
	}
	if cfg.Disabled {
		resp.Status = StatusDisconnected
	}
	if s.adapter != nil {
		if st, ok := s.adapter.Status(cfg.ID); ok {
			resp.Status = st.Status
			resp.Running = st.Running
			resp.LastError = st.LastError
			resp.UpdatedAt = st.UpdatedAt
		}
	}
	resp.DeviceJID = strings.TrimSpace(channel.ReadString(cfg.SelfIdentity, "device_jid"))
	resp.Phone = strings.TrimSpace(channel.ReadString(cfg.SelfIdentity, "phone"))
	resp.PushName = strings.TrimSpace(channel.ReadString(cfg.SelfIdentity, "push_name"))
	if cfg.Credentials != nil && readBool(cfg.Credentials, "needsRelink", "needs_relink") {
		resp.Status = StatusLoggedOut
	}
	return resp, nil
}

func (s *Service) Logout(ctx context.Context, botID string) (LogoutResponse, error) {
	if s.store == nil {
		return LogoutResponse{}, errors.New("channel store not configured")
	}
	cfg, err := s.store.ResolveEffectiveConfig(ctx, botID, Type)
	if err != nil {
		if errors.Is(err, channel.ErrChannelConfigNotFound) || strings.Contains(err.Error(), "not found") {
			return LogoutResponse{Status: "not_configured"}, nil
		}
		return LogoutResponse{}, err
	}
	var remoteLogoutErr error
	if s.adapter != nil {
		remoteLogoutErr = s.adapter.Logout(ctx, cfg)
		if remoteLogoutErr != nil && s.logger != nil {
			s.logger.Warn("remote whatsapp logout failed; continuing local cleanup",
				slog.String("bot_id", botID),
				slog.String("config_id", cfg.ID),
				slog.Any("error", remoteLogoutErr),
			)
		}
	}
	if s.manager != nil {
		_ = s.manager.Stop(ctx, cfg.ID)
	}
	waCfg, _ := parseConfig(cfg.Credentials)
	if waCfg.StoreID != "" {
		if err := removeStore(finalStorePaths(s.dataRoot, waCfg.StoreID)); err != nil {
			return LogoutResponse{Status: "cleanup_error", Message: err.Error()}, err
		}
	}
	if err := s.store.DeleteConfig(ctx, botID, Type); err != nil {
		return LogoutResponse{Status: "cleanup_error", Message: err.Error()}, err
	}
	resp := LogoutResponse{Status: "logged_out"}
	if remoteLogoutErr != nil {
		resp.Message = remoteLogoutErr.Error()
	}
	return resp, nil
}

func (s *Service) markNeedsRelink(ctx context.Context, cfg channel.ChannelConfig, status string, cause error) {
	if s.store == nil || strings.TrimSpace(cfg.BotID) == "" {
		return
	}
	s.configMu.Lock()
	defer s.configMu.Unlock()
	current, err := s.store.ResolveEffectiveConfig(ctx, cfg.BotID, Type)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("load whatsapp config before marking relink failed",
				slog.String("config_id", cfg.ID),
				slog.String("status", status),
				slog.Any("cause", cause),
				slog.Any("error", err),
			)
		}
		return
	}
	if strings.TrimSpace(cfg.ID) != "" && current.ID != cfg.ID {
		return
	}
	if !cfg.UpdatedAt.IsZero() && current.UpdatedAt.After(cfg.UpdatedAt) {
		return
	}
	creds := map[string]any{"needsRelink": true}
	if waCfg, err := parseConfig(cfg.Credentials); err == nil && waCfg.StoreID != "" {
		creds["storeId"] = waCfg.StoreID
	}
	disabled := true
	updateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if _, err := s.store.UpsertConfig(updateCtx, cfg.BotID, Type, channel.UpsertConfigRequest{
		Credentials: creds,
		Disabled:    &disabled,
	}); err != nil && s.logger != nil {
		s.logger.Warn("mark whatsapp channel needs relink failed",
			slog.String("config_id", cfg.ID),
			slog.String("status", status),
			slog.Any("cause", cause),
			slog.Any("error", err),
		)
	}
	if s.manager != nil {
		_ = s.manager.Stop(updateCtx, cfg.ID)
	}
}

func (s *Service) startPending(ctx context.Context, botID, mode string) (*pendingLogin, error) {
	s.cleanupExpired()
	loginID := s.idSource()
	if strings.TrimSpace(loginID) == "" {
		return nil, errors.New("failed to allocate login id")
	}
	if err := validateStoreID(loginID); err != nil {
		return nil, err
	}
	loginCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	paths := pendingStorePaths(s.dataRoot, loginID)
	container, client, err := openClientStore(loginCtx, paths, waLog.Noop)
	if err != nil {
		cancel()
		return nil, err
	}
	qrChan, err := client.GetQRChannel(loginCtx)
	if err != nil {
		_ = container.Close()
		cancel()
		_ = removeStore(paths)
		return nil, err
	}
	p := &pendingLogin{
		ID:        loginID,
		BotID:     strings.TrimSpace(botID),
		Mode:      mode,
		Status:    StatusQRPending,
		ExpiresAt: s.now().Add(pendingTTL),
		ctx:       loginCtx,
		cancel:    cancel,
		container: container,
		client:    client,
		events:    qrChan,
	}
	s.mu.Lock()
	s.pending[loginID] = p
	s.mu.Unlock()
	s.schedulePendingExpiry(p)
	if err := client.Connect(); err != nil {
		s.finishPending(loginID, true)
		return nil, err
	}
	p.mu.Lock()
	s.drainPending(p, 5*time.Second)
	p.mu.Unlock()
	return p, nil
}

func (s *Service) schedulePendingExpiry(p *pendingLogin) {
	if p == nil {
		return
	}
	delay := p.ExpiresAt.Sub(s.now())
	if delay <= 0 {
		go s.expirePending(p.ID)
		return
	}
	p.expiryTimer = time.AfterFunc(delay, func() {
		s.expirePending(p.ID)
	})
}

func (s *Service) finalizePair(ctx context.Context, p *pendingLogin) (channel.ChannelConfig, error) {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	if p.client != nil {
		p.client.Disconnect()
	}
	if p.container != nil {
		_ = p.container.Close()
		p.container = nil
	}
	previous, hadPrevious, err := s.currentConfig(ctx, p.BotID)
	if err != nil {
		return channel.ChannelConfig{}, err
	}
	placeholderWritten := false
	committed := false
	defer func() {
		if committed || !placeholderWritten {
			return
		}
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := s.restoreConfig(rollbackCtx, p.BotID, previous, hadPrevious); err != nil && s.logger != nil {
			s.logger.Warn("rollback whatsapp channel config after pairing failure failed",
				slog.String("bot_id", p.BotID),
				slog.Bool("had_previous", hadPrevious),
				slog.Any("error", err),
			)
		}
	}()
	placeholderDisabled := true
	cfg, err := s.store.UpsertConfig(ctx, p.BotID, Type, channel.UpsertConfigRequest{
		Credentials: map[string]any{"needsRelink": true},
		Disabled:    &placeholderDisabled,
	})
	if err != nil {
		return channel.ChannelConfig{}, err
	}
	placeholderWritten = true
	if s.manager != nil {
		_ = s.manager.Stop(ctx, cfg.ID)
	}
	pendingPaths := pendingStorePaths(s.dataRoot, p.ID)
	self, externalID, err := loadStoreIdentity(ctx, pendingPaths)
	if err != nil {
		return channel.ChannelConfig{}, err
	}
	verifiedAt := s.now()
	cfg, err = s.store.UpsertConfig(ctx, p.BotID, Type, channel.UpsertConfigRequest{
		Credentials:      map[string]any{"storeId": cfg.ID},
		ExternalIdentity: externalID,
		SelfIdentity:     self,
		Disabled:         &placeholderDisabled,
		VerifiedAt:       &verifiedAt,
	})
	if err != nil {
		return channel.ChannelConfig{}, err
	}
	replacement, err := replaceWhatsAppStore(pendingPaths, finalStorePaths(s.dataRoot, cfg.ID), p.ID)
	if err != nil {
		return channel.ChannelConfig{}, err
	}
	replacement.Commit()
	committed = true
	if s.lifecycle != nil {
		cfg, err = s.lifecycle.SetBotChannelStatus(ctx, p.BotID, Type, false)
		if err != nil {
			return channel.ChannelConfig{}, err
		}
	}
	return cfg, nil
}

func (s *Service) currentConfig(ctx context.Context, botID string) (channel.ChannelConfig, bool, error) {
	cfg, err := s.store.ResolveEffectiveConfig(ctx, botID, Type)
	if err == nil {
		return cfg, true, nil
	}
	if errors.Is(err, channel.ErrChannelConfigNotFound) || strings.Contains(err.Error(), "not found") {
		return channel.ChannelConfig{}, false, nil
	}
	return channel.ChannelConfig{}, false, err
}

func (s *Service) restoreConfig(ctx context.Context, botID string, previous channel.ChannelConfig, hadPrevious bool) error {
	if !hadPrevious {
		return s.store.DeleteConfig(ctx, botID, Type)
	}
	_, err := s.store.UpsertConfig(ctx, botID, Type, upsertRequestFromWhatsAppConfig(previous))
	return err
}

func upsertRequestFromWhatsAppConfig(cfg channel.ChannelConfig) channel.UpsertConfigRequest {
	disabled := cfg.Disabled
	req := channel.UpsertConfigRequest{
		Credentials:      cloneAnyMap(cfg.Credentials),
		ExternalIdentity: strings.TrimSpace(cfg.ExternalIdentity),
		SelfIdentity:     cloneAnyMap(cfg.SelfIdentity),
		Routing:          cloneAnyMap(cfg.Routing),
		Disabled:         &disabled,
	}
	if !cfg.VerifiedAt.IsZero() {
		verifiedAt := cfg.VerifiedAt.UTC()
		req.VerifiedAt = &verifiedAt
	}
	return req
}

func cloneAnyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func loadStoreIdentity(ctx context.Context, paths storePaths) (map[string]any, string, error) {
	container, client, err := openClientStore(ctx, paths, waLog.Noop)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = container.Close() }()
	if client.Store == nil || client.Store.ID == nil {
		return nil, "", errors.New("paired whatsapp store has no device id")
	}
	jid := client.Store.GetJID()
	self := map[string]any{
		"device_jid": jid.String(),
		"phone":      jid.User,
	}
	if pushName := strings.TrimSpace(client.Store.PushName); pushName != "" {
		self["push_name"] = pushName
	}
	if business := strings.TrimSpace(client.Store.BusinessName); business != "" {
		self["business_name"] = business
	}
	return self, jid.String(), nil
}

func (s *Service) getPending(loginID string) *pendingLogin {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pending[strings.TrimSpace(loginID)]
}

func (s *Service) finishPending(loginID string, remove bool) {
	s.mu.Lock()
	p := s.pending[loginID]
	if remove {
		delete(s.pending, loginID)
	}
	s.mu.Unlock()
	if p == nil {
		return
	}
	p.finishOnce.Do(func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.expiryTimer != nil {
			p.expiryTimer.Stop()
		}
		if p.cancel != nil {
			p.cancel()
		}
		if p.client != nil {
			p.client.Disconnect()
		}
		if p.container != nil {
			_ = p.container.Close()
		}
		if remove {
			_ = removeStore(pendingStorePaths(s.dataRoot, p.ID))
		}
	})
}

func (s *Service) expirePending(loginID string) {
	p := s.getPending(loginID)
	if p == nil {
		return
	}
	p.mu.Lock()
	if p.paired || s.now().Before(p.ExpiresAt) {
		p.mu.Unlock()
		return
	}
	p.Status = "expired"
	p.mu.Unlock()
	s.finishPending(p.ID, true)
}

func (s *Service) cleanupExpired() {
	now := s.now()
	var expired []*pendingLogin
	s.mu.Lock()
	for _, p := range s.pending {
		if now.After(p.ExpiresAt) {
			expired = append(expired, p)
		}
	}
	s.mu.Unlock()
	for _, p := range expired {
		p.mu.Lock()
		if now.After(p.ExpiresAt) && !p.paired {
			p.Status = "expired"
		}
		p.mu.Unlock()
		s.finishPending(p.ID, true)
	}
}

func (s *Service) drainPending(p *pendingLogin, maxWait time.Duration) {
	deadline := time.NewTimer(maxWait)
	defer deadline.Stop()
	for {
		select {
		case item, ok := <-p.events:
			if !ok {
				if p.Status == StatusQRPending {
					p.Status = "expired"
				}
				return
			}
			s.applyQRItem(p, item)
			if p.QRCode != "" || p.Status != StatusQRPending {
				return
			}
		case <-deadline.C:
			return
		}
	}
}

func (*Service) applyQRItem(p *pendingLogin, item whatsmeow.QRChannelItem) {
	switch item.Event {
	case whatsmeow.QRChannelEventCode:
		if p.Mode == pendingModePhone && p.PairCode != "" {
			p.Status = "pair_code"
		} else {
			p.Status = StatusQRPending
		}
		p.QRCode = strings.TrimSpace(item.Code)
		p.Timeout = item.Timeout
	case whatsmeow.QRChannelEventError:
		p.Status = StatusTerminal
		if item.Error != nil {
			p.Error = item.Error.Error()
		}
	case "success":
		p.Status = "success"
	case "timeout":
		p.Status = "expired"
	case "err-client-outdated":
		p.Status = StatusTerminal
		p.Error = "client outdated"
	case "err-scanned-without-multidevice":
		p.Status = StatusTerminal
		p.Error = "scan failed: multi-device is required"
	default:
		if strings.HasPrefix(item.Event, "err-") {
			p.Status = StatusTerminal
			p.Error = item.Event
		}
	}
}

func statusMessage(status string) string {
	switch status {
	case StatusQRPending:
		return "Scan the QR code with WhatsApp"
	case "pair_code":
		return "Enter the pairing code in WhatsApp"
	case "success":
		return "Login successful"
	case "expired":
		return "QR code expired"
	case StatusConnected:
		return "Connected"
	case StatusLoggedOut:
		return "Logged out"
	default:
		return status
	}
}

func newLoginID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return hex.EncodeToString(buf[:])
	}
	return hex.EncodeToString([]byte(time.Now().UTC().Format("20060102150405.000000000")))
}
