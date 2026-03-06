package tts

import "context"

type TtsType string

type TtsMeta struct {
	Provider    string
	Description string
}

type TtsAdapter interface {
	Type() TtsType
	Meta() TtsMeta
	Capabilities() Capabilities
	Synthesize(ctx context.Context, text string, config AudioConfig) ([]byte, error)
	Stream(ctx context.Context, text string, config AudioConfig) (chan []byte, chan error)
}
