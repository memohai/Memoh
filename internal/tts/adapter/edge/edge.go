package edge

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/memohai/memoh/internal/tts"
)

const TtsTypeEdge tts.TtsType = "edge"

type EdgeAdapter struct {
	logger *slog.Logger
	client *EdgeWsClient
}

func NewEdgeAdapter(log *slog.Logger) *EdgeAdapter {
	return &EdgeAdapter{
		logger: log.With(slog.String("adapter", "edge")),
		client: NewEdgeWsClient(),
	}
}

// NewEdgeAdapterWithClient for testing: inject custom WebSocket client (e.g. mock server).
func NewEdgeAdapterWithClient(log *slog.Logger, client *EdgeWsClient) *EdgeAdapter {
	return &EdgeAdapter{
		logger: log.With(slog.String("adapter", "edge")),
		client: client,
	}
}

func (*EdgeAdapter) Type() tts.TtsType {
	return TtsTypeEdge
}

func (*EdgeAdapter) Meta() tts.TtsMeta {
	return tts.TtsMeta{
		Provider:    "Microsoft Edge",
		Description: "Microsoft Edge TTS",
	}
}

var edgeFormats = []string{
	"audio-24khz-48kbitrate-mono-mp3",
	"audio-24khz-96kbitrate-mono-mp3",
	"webm-24khz-16bit-mono-opus",
}

var edgeSpeedConstraint = &tts.ParamConstraint{
	Options: []float64{0.5, 1.0, 2.0, 3.0},
	Default: 1.0,
}

var edgePitchConstraint = &tts.ParamConstraint{
	Min:     -100,
	Max:     100,
	Default: 0,
}

func (*EdgeAdapter) Capabilities() tts.Capabilities {
	var voices []tts.VoiceInfo
	for lang, ids := range EdgeTTSVoices {
		for _, id := range ids {
			name := strings.TrimPrefix(id, lang+"-")
			name = strings.TrimSuffix(name, "Neural")
			voices = append(voices, tts.VoiceInfo{ID: id, Lang: lang, Name: name})
		}
	}
	return tts.Capabilities{
		Voices:  voices,
		Formats: edgeFormats,
		Speed:   edgeSpeedConstraint,
		Pitch:   edgePitchConstraint,
	}
}

func (a *EdgeAdapter) Synthesize(ctx context.Context, text string, config tts.AudioConfig) ([]byte, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("edge tts: invalid config: %w", err)
	}
	return a.client.Synthesize(ctx, text, config)
}

func (a *EdgeAdapter) Stream(ctx context.Context, text string, config tts.AudioConfig) (chan []byte, chan error) {
	if err := config.Validate(); err != nil {
		errCh := make(chan error, 1)
		errCh <- fmt.Errorf("edge tts: invalid config: %w", err)
		close(errCh)
		return nil, errCh
	}
	return a.client.Stream(ctx, text, config)
}
