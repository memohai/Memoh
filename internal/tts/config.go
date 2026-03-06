package tts

import "github.com/go-playground/validator/v10"

var validate = validator.New()

// AudioConfig is the user-facing configuration for a TTS request.
type AudioConfig struct {
	Format     string  `json:"format"      validate:"omitempty"`
	SampleRate int     `json:"sample_rate"  validate:"omitempty,oneof=16000 24000 48000"`
	Speed      float64 `json:"speed"        validate:"omitempty"`
	Pitch      float64 `json:"pitch"        validate:"omitempty"`
	Voice      string  `json:"voice"        validate:"required"`
}

func (c AudioConfig) Validate() error {
	return validate.Struct(c)
}

// ParamConstraint describes valid values for a numeric parameter.
// If Options is non-empty, only those discrete values are allowed (frontend renders a select).
// Otherwise Min/Max define a continuous range (frontend renders a slider).
type ParamConstraint struct {
	Options []float64 `json:"options,omitempty"`
	Min     float64   `json:"min,omitempty"`
	Max     float64   `json:"max,omitempty"`
	Default float64   `json:"default"`
}

// Capabilities describes what a TTS backend supports.
// nil pointer means the parameter is not supported; frontend should hide it.
type Capabilities struct {
	Voices  []VoiceInfo      `json:"voices"`
	Formats []string         `json:"formats"`
	Speed   *ParamConstraint `json:"speed,omitempty"`
	Pitch   *ParamConstraint `json:"pitch,omitempty"`
}

type VoiceInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Lang string `json:"lang"`
}
