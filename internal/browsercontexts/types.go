package browsercontexts

import "encoding/json"

type BrowserContext struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Config    json.RawMessage `json:"config"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

type CreateRequest struct {
	Name   string          `json:"name"`
	Config json.RawMessage `json:"config"`
}

type UpdateRequest struct {
	Name   string          `json:"name"`
	Config json.RawMessage `json:"config"`
}
