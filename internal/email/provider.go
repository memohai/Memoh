package email

import (
	"context"
	"fmt"
	"net/http"
	"sync"
)

// Adapter is the base interface every email adapter must implement.
type Adapter interface {
	Type() ProviderName
	Meta() ProviderMeta
	NormalizeConfig(raw map[string]any) (map[string]any, error)
}

// Sender sends outbound emails.
type Sender interface {
	Send(ctx context.Context, config map[string]any, msg OutboundEmail) (messageID string, err error)
}

// Receiver establishes a long-lived connection (IMAP IDLE / polling) to receive emails.
type Receiver interface {
	StartReceiving(ctx context.Context, config map[string]any, handler InboundHandler) (Stopper, error)
}

// WebhookReceiver handles inbound emails via HTTP webhook callbacks.
type WebhookReceiver interface {
	HandleWebhook(ctx context.Context, config map[string]any, r *http.Request) (*InboundEmail, error)
}

// MailboxReader lists and reads emails directly from the remote mailbox.
type MailboxReader interface {
	ListMailbox(ctx context.Context, config map[string]any, page, pageSize int) ([]InboundEmail, int, error)
	ReadMailbox(ctx context.Context, config map[string]any, uid uint32) (*InboundEmail, error)
}

// Deleter removes an email from the remote mailbox.
type Deleter interface {
	DeleteRemote(ctx context.Context, config map[string]any, messageID string) error
}

// InboundHandler is invoked when a new email arrives.
type InboundHandler func(ctx context.Context, providerID string, email InboundEmail) error

// Stopper represents a stoppable background process.
type Stopper interface {
	Stop(ctx context.Context) error
}

// Registry holds all registered email adapters.
type Registry struct {
	mu       sync.RWMutex
	adapters map[ProviderName]Adapter
}

func NewRegistry() *Registry {
	return &Registry{adapters: make(map[ProviderName]Adapter)}
}

func (r *Registry) Register(a Adapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[a.Type()] = a
}

func (r *Registry) Get(name ProviderName) (Adapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.adapters[name]
	if !ok {
		return nil, fmt.Errorf("email adapter not found: %s", name)
	}
	return a, nil
}

func (r *Registry) GetSender(name ProviderName) (Sender, error) {
	a, err := r.Get(name)
	if err != nil {
		return nil, err
	}
	s, ok := a.(Sender)
	if !ok {
		return nil, fmt.Errorf("email adapter %s does not support sending", name)
	}
	return s, nil
}

func (r *Registry) GetReceiver(name ProviderName) (Receiver, error) {
	a, err := r.Get(name)
	if err != nil {
		return nil, err
	}
	recv, ok := a.(Receiver)
	if !ok {
		return nil, fmt.Errorf("email adapter %s does not support receiving", name)
	}
	return recv, nil
}

func (r *Registry) GetMailboxReader(name ProviderName) (MailboxReader, error) {
	a, err := r.Get(name)
	if err != nil {
		return nil, err
	}
	reader, ok := a.(MailboxReader)
	if !ok {
		return nil, fmt.Errorf("email adapter %s does not support mailbox reading", name)
	}
	return reader, nil
}

func (r *Registry) ListMeta() []ProviderMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()
	metas := make([]ProviderMeta, 0, len(r.adapters))
	for _, a := range r.adapters {
		metas = append(metas, a.Meta())
	}
	return metas
}
