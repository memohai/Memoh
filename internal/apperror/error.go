package apperror

import (
	"errors"
	"net/http"
	"strings"
)

// Code is the stable machine identity shared by every transport. Client logic
// may branch on it; it must never branch on Detail or an underlying cause.
type Code string

const (
	CodeBotNameTaken                  Code = "bot.name_taken"
	CodeWorkspaceUnreachable          Code = "workspace.unreachable"
	CodeWorkspaceDisplayPrepareFailed Code = "workspace.display_prepare_failed"
)

// Definition is the single catalog entry for a public error contract.
// Type URIs and frontend i18n keys are derived mechanically from Code.
type Definition struct {
	HTTPStatus  int
	Detail      string
	AllowedArgs []string
}

// codesync(error-catalog): Detail strings double as the no-locale fallback for
// clients; the localized copies live under errors.* in
// apps/web/src/i18n/locales/{en,zh,ja}.json. Keep both sides in sync.
var catalog = map[Code]Definition{
	CodeBotNameTaken: {
		HTTPStatus:  http.StatusConflict,
		Detail:      "This name is already taken.",
		AllowedArgs: []string{"field"},
	},
	CodeWorkspaceUnreachable: {
		HTTPStatus: http.StatusServiceUnavailable,
		Detail:     "The workspace could not be reached.",
	},
	// Distinct from workspace.unreachable: preparation started but broke
	// mid-flight, so "could not be reached" would mislead the user.
	CodeWorkspaceDisplayPrepareFailed: {
		HTTPStatus: http.StatusInternalServerError,
		Detail:     "Display preparation failed.",
	},
}

// Error keeps the public contract separate from private diagnostics. The cause
// is intentionally not exposed through Unwrap; transport boundaries may log it
// through CauseOf without making infrastructure details part of the API.
type Error struct {
	code  Code
	args  map[string]string
	cause error
}

// New creates a public application error without an infrastructure cause.
func New(code Code, args map[string]string) *Error {
	return &Error{code: code, args: sanitizeArgs(code, args)}
}

// Wrap retains a private cause for boundary logging. Only catalog-allowed args
// are kept for serialization.
func Wrap(code Code, cause error, args map[string]string) *Error {
	return &Error{code: code, args: sanitizeArgs(code, args), cause: cause}
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return string(e.code)
}

func As(err error) (*Error, bool) {
	var appErr *Error
	if !errors.As(err, &appErr) {
		return nil, false
	}
	return appErr, true
}

func CodeOf(err error) Code {
	appErr, ok := As(err)
	if !ok {
		return ""
	}
	return appErr.code
}

func ArgsOf(err error) map[string]string {
	appErr, ok := As(err)
	if !ok {
		return map[string]string{}
	}
	return cloneArgs(appErr.args)
}

// CauseOf is intentionally separate from errors.Unwrap: infrastructure errors
// are retained for boundary logging without becoming a domain-level contract.
func CauseOf(err error) error {
	appErr, ok := As(err)
	if !ok {
		return nil
	}
	return appErr.cause
}

func Lookup(code Code) (Definition, bool) {
	definition, ok := catalog[code]
	definition.AllowedArgs = append([]string(nil), definition.AllowedArgs...)
	return definition, ok
}

func TypeURI(code Code) string {
	return "urn:memoh:error:" + string(code)
}

func cloneArgs(args map[string]string) map[string]string {
	cloned := make(map[string]string, len(args))
	for key, value := range args {
		key = strings.TrimSpace(key)
		if key != "" {
			cloned[key] = value
		}
	}
	return cloned
}

// sanitizeArgs is the public-data boundary for error metadata. Callers may
// provide useful internal context, but only keys declared by the catalog are
// allowed onto the wire.
func sanitizeArgs(code Code, args map[string]string) map[string]string {
	definition, ok := catalog[code]
	if !ok || len(definition.AllowedArgs) == 0 {
		return map[string]string{}
	}

	allowed := make(map[string]struct{}, len(definition.AllowedArgs))
	for _, key := range definition.AllowedArgs {
		allowed[key] = struct{}{}
	}
	sanitized := make(map[string]string, len(args))
	for key, value := range args {
		key = strings.TrimSpace(key)
		if _, ok := allowed[key]; ok {
			sanitized[key] = value
		}
	}
	return sanitized
}
