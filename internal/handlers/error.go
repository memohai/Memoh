package handlers

type ErrorResponse struct {
	Message    string            `json:"message"`
	Code       string            `json:"code,omitempty"`
	Reason     string            `json:"reason,omitempty"`
	HTTPStatus int               `json:"http_status,omitempty"`
	I18nKey    string            `json:"i18n_key,omitempty"`
	Args       map[string]string `json:"args,omitempty"`
}
