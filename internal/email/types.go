package email

import "time"

type ProviderName string


// FieldSchema describes a single configuration field for dynamic form generation.
type FieldSchema struct {
	Key         string   `json:"key"`
	Type        string   `json:"type"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Example     any      `json:"example,omitempty"`
	Order       int      `json:"order"`
}

type ConfigSchema struct {
	Fields []FieldSchema `json:"fields"`
}

type ProviderMeta struct {
	Provider     string       `json:"provider"`
	DisplayName  string       `json:"display_name"`
	ConfigSchema ConfigSchema `json:"config_schema"`
}

// ---- Provider CRUD DTOs ----

type CreateProviderRequest struct {
	Name     string         `json:"name"`
	Provider ProviderName   `json:"provider"`
	Config   map[string]any `json:"config,omitempty"`
}

type UpdateProviderRequest struct {
	Name     *string        `json:"name,omitempty"`
	Provider *ProviderName  `json:"provider,omitempty"`
	Config   map[string]any `json:"config,omitempty"`
}

type ProviderResponse struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Provider  string         `json:"provider"`
	Config    map[string]any `json:"config,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// ---- Binding DTOs ----

type CreateBindingRequest struct {
	EmailProviderID string         `json:"email_provider_id"`
	EmailAddress    string         `json:"email_address"`
	CanRead         *bool          `json:"can_read,omitempty"`
	CanWrite        *bool          `json:"can_write,omitempty"`
	CanDelete       *bool          `json:"can_delete,omitempty"`
	Config          map[string]any `json:"config,omitempty"`
}

type UpdateBindingRequest struct {
	EmailAddress *string        `json:"email_address,omitempty"`
	CanRead      *bool          `json:"can_read,omitempty"`
	CanWrite     *bool          `json:"can_write,omitempty"`
	CanDelete    *bool          `json:"can_delete,omitempty"`
	Config       map[string]any `json:"config,omitempty"`
}

type BindingResponse struct {
	ID              string         `json:"id"`
	BotID           string         `json:"bot_id"`
	EmailProviderID string         `json:"email_provider_id"`
	EmailAddress    string         `json:"email_address"`
	CanRead         bool           `json:"can_read"`
	CanWrite        bool           `json:"can_write"`
	CanDelete       bool           `json:"can_delete"`
	Config          map[string]any `json:"config,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// ---- Email message types ----

type OutboundEmail struct {
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Body    string   `json:"body"`
	HTML    bool     `json:"html,omitempty"`
}

type InboundEmail struct {
	MessageID   string         `json:"message_id"`
	From        string         `json:"from"`
	To          []string       `json:"to"`
	Subject     string         `json:"subject"`
	BodyText    string         `json:"body_text"`
	BodyHTML    string         `json:"body_html"`
	Attachments []any          `json:"attachments,omitempty"`
	Headers     map[string]any `json:"headers,omitempty"`
	ReceivedAt  time.Time      `json:"received_at"`
}

// ---- Inbox / Outbox response DTOs ----

type InboxItemResponse struct {
	ID          string    `json:"id"`
	ProviderID  string    `json:"provider_id"`
	BotID       string    `json:"bot_id,omitempty"`
	MessageID   string    `json:"message_id"`
	From        string    `json:"from"`
	To          []string  `json:"to"`
	Subject     string    `json:"subject"`
	BodyText    string    `json:"body_text,omitempty"`
	BodyHTML    string    `json:"body_html,omitempty"`
	Attachments []any     `json:"attachments,omitempty"`
	Headers     any       `json:"headers,omitempty"`
	ReceivedAt  time.Time `json:"received_at"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type OutboxItemResponse struct {
	ID          string    `json:"id"`
	ProviderID  string    `json:"provider_id"`
	BotID       string    `json:"bot_id"`
	MessageID   string    `json:"message_id,omitempty"`
	From        string    `json:"from"`
	To          []string  `json:"to"`
	Subject     string    `json:"subject"`
	BodyText    string    `json:"body_text,omitempty"`
	BodyHTML    string    `json:"body_html,omitempty"`
	Attachments []any     `json:"attachments,omitempty"`
	Status      string    `json:"status"`
	Error       string    `json:"error,omitempty"`
	SentAt      time.Time `json:"sent_at,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}
