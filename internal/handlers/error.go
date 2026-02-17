package handlers

// ErrorResponse is the standard API error body (message only).
type ErrorResponse struct {
	Message string `json:"message"`
}
