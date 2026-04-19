package tts

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	alibabaspeech "github.com/memohai/twilight-ai/provider/alibabacloud/speech"
	deepgramspeech "github.com/memohai/twilight-ai/provider/deepgram/speech"
	edgespeech "github.com/memohai/twilight-ai/provider/edge/speech"
	elevenlabsspeech "github.com/memohai/twilight-ai/provider/elevenlabs/speech"
	microsoftspeech "github.com/memohai/twilight-ai/provider/microsoft/speech"
	minimaxspeech "github.com/memohai/twilight-ai/provider/minimax/speech"
	openaispeech "github.com/memohai/twilight-ai/provider/openai/speech"
	openrouterspeech "github.com/memohai/twilight-ai/provider/openrouter/speech"
	volcenginespeech "github.com/memohai/twilight-ai/provider/volcengine/speech"
	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/models"
)

type ProviderFactory func(config map[string]any) (sdk.SpeechProvider, error)

type ProviderDefinition struct {
	ClientType   models.ClientType
	DisplayName  string
	Icon         string
	Description  string
	ConfigSchema ConfigSchema
	DefaultModel string
	SupportsList bool
	Models       []ModelInfo
	Factory      ProviderFactory
	Order        int
}

type Registry struct {
	mu        sync.RWMutex
	providers map[models.ClientType]ProviderDefinition
	ordered   []models.ClientType
}

func NewRegistry() *Registry {
	r := &Registry{
		providers: make(map[models.ClientType]ProviderDefinition),
	}
	for _, def := range defaultProviderDefinitions() {
		r.Register(def)
	}
	return r
}

func (r *Registry) Register(def ProviderDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[def.ClientType]; !exists {
		r.ordered = append(r.ordered, def.ClientType)
	}
	r.providers[def.ClientType] = def
	sort.SliceStable(r.ordered, func(i, j int) bool {
		left := r.providers[r.ordered[i]]
		right := r.providers[r.ordered[j]]
		if left.Order != right.Order {
			return left.Order < right.Order
		}
		return left.DisplayName < right.DisplayName
	})
}

func (r *Registry) Get(clientType models.ClientType) (ProviderDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.providers[clientType]
	if !ok {
		return ProviderDefinition{}, fmt.Errorf("speech provider not found: %s", clientType)
	}
	return def, nil
}

func (r *Registry) List() []ProviderDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ProviderDefinition, 0, len(r.ordered))
	for _, key := range r.ordered {
		out = append(out, r.providers[key])
	}
	return out
}

func (r *Registry) ListMeta() []ProviderMetaResponse {
	defs := r.List()
	metas := make([]ProviderMetaResponse, 0, len(defs))
	for _, def := range defs {
		metas = append(metas, ProviderMetaResponse{
			Provider:     string(def.ClientType),
			DisplayName:  def.DisplayName,
			Description:  def.Description,
			ConfigSchema: def.ConfigSchema,
			DefaultModel: def.DefaultModel,
			Models:       def.Models,
		})
	}
	return metas
}

func defaultProviderDefinitions() []ProviderDefinition {
	edgeVoices := make([]VoiceInfo, 0)
	for lang, ids := range edgespeech.EdgeTTSVoices {
		for _, id := range ids {
			name := strings.TrimPrefix(id, lang+"-")
			name = strings.TrimSuffix(name, "Neural")
			edgeVoices = append(edgeVoices, VoiceInfo{ID: id, Lang: lang, Name: name})
		}
	}
	sort.Slice(edgeVoices, func(i, j int) bool {
		if edgeVoices[i].Lang != edgeVoices[j].Lang {
			return edgeVoices[i].Lang < edgeVoices[j].Lang
		}
		return edgeVoices[i].ID < edgeVoices[j].ID
	})

	return []ProviderDefinition{
		{
			ClientType:   models.ClientTypeEdgeSpeech,
			DisplayName:  "Microsoft Edge",
			Icon:         "microsoft",
			Description:  "Free Edge Read Aloud TTS",
			ConfigSchema: ConfigSchema{Fields: []FieldSchema{stringField("base_url", "Base URL", "Override the Edge WebSocket endpoint", false, "", 10)}},
			DefaultModel: "edge-read-aloud",
			SupportsList: false,
			Models: []ModelInfo{{
				ID:          "edge-read-aloud",
				Name:        "Edge Read Aloud",
				Description: "Built-in Edge Read Aloud speech model",
				ConfigSchema: ConfigSchema{Fields: []FieldSchema{
					enumField("voice", "Voice", "Edge voice ID", false, voiceIDs(edgeVoices), 10),
					stringField("language", "Language", "Optional BCP-47 language tag", false, "en-US", 20),
					enumField("format", "Format", "Output audio format", false, []string{"audio-24khz-48kbitrate-mono-mp3", "audio-24khz-96kbitrate-mono-mp3", "webm-24khz-16bit-mono-opus"}, 30),
					numberField("speed", "Speed", "Speech rate, 1.0 = normal", false, 1.0, 40),
					numberField("pitch", "Pitch", "Pitch adjustment in Hz", false, 0, 50),
				}},
				Capabilities: ModelCapabilities{
					ConfigSchema: ConfigSchema{Fields: []FieldSchema{
						enumField("voice", "Voice", "Edge voice ID", false, voiceIDs(edgeVoices), 10),
						stringField("language", "Language", "Optional BCP-47 language tag", false, "en-US", 20),
						enumField("format", "Format", "Output audio format", false, []string{"audio-24khz-48kbitrate-mono-mp3", "audio-24khz-96kbitrate-mono-mp3", "webm-24khz-16bit-mono-opus"}, 30),
						numberField("speed", "Speed", "Speech rate, 1.0 = normal", false, 1.0, 40),
						numberField("pitch", "Pitch", "Pitch adjustment in Hz", false, 0, 50),
					}},
					Voices:  edgeVoices,
					Formats: []string{"audio-24khz-48kbitrate-mono-mp3", "audio-24khz-96kbitrate-mono-mp3", "webm-24khz-16bit-mono-opus"},
					Speed:   &ParamConstraint{Options: []float64{0.5, 1.0, 2.0, 3.0}, Default: 1.0},
					Pitch:   &ParamConstraint{Min: -100, Max: 100, Default: 0},
				},
			}},
			Factory: func(config map[string]any) (sdk.SpeechProvider, error) {
				opts := []edgespeech.Option{}
				if v := configString(config, "base_url"); v != "" {
					opts = append(opts, edgespeech.WithBaseURL(v))
				}
				return edgespeech.New(opts...), nil
			},
			Order: 10,
		},
		{
			ClientType:  models.ClientTypeOpenAISpeech,
			DisplayName: "OpenAI Speech",
			Icon:        "openai",
			Description: "OpenAI /audio/speech compatible TTS",
			ConfigSchema: ConfigSchema{Fields: []FieldSchema{
				secretField("api_key", "API Key", "Bearer API key", true, 10),
				stringField("base_url", "Base URL", "Override the API base URL", false, "https://api.openai.com/v1", 20),
			}},
			DefaultModel: "gpt-4o-mini-tts",
			SupportsList: true,
			Models: []ModelInfo{{
				ID:          "gpt-4o-mini-tts",
				Name:        "gpt-4o-mini-tts",
				Description: "Default OpenAI speech model",
				ConfigSchema: ConfigSchema{Fields: []FieldSchema{
					stringField("voice", "Voice", "Voice ID", false, "coral", 10),
					enumField("response_format", "Response Format", "Audio format", false, []string{"mp3", "opus", "pcm", "wav"}, 20),
					numberField("speed", "Speed", "Speech rate", false, 1.0, 30),
					stringField("instructions", "Instructions", "Style instructions for supported models", false, "", 40),
				}},
				Capabilities: ModelCapabilities{
					ConfigSchema: ConfigSchema{Fields: []FieldSchema{
						stringField("voice", "Voice", "Voice ID", false, "coral", 10),
						enumField("response_format", "Response Format", "Audio format", false, []string{"mp3", "opus", "pcm", "wav"}, 20),
						numberField("speed", "Speed", "Speech rate", false, 1.0, 30),
						stringField("instructions", "Instructions", "Style instructions for supported models", false, "", 40),
					}},
					Formats: []string{"mp3", "opus", "pcm", "wav"},
				},
			}},
			Factory: func(config map[string]any) (sdk.SpeechProvider, error) {
				opts := []openaispeech.Option{}
				if v := configString(config, "api_key"); v != "" {
					opts = append(opts, openaispeech.WithAPIKey(v))
				}
				if v := configString(config, "base_url"); v != "" {
					opts = append(opts, openaispeech.WithBaseURL(v))
				}
				return openaispeech.New(opts...), nil
			},
			Order: 20,
		},
		{
			ClientType:  models.ClientTypeOpenRouterSpeech,
			DisplayName: "OpenRouter Speech",
			Icon:        "openrouter",
			Description: "OpenRouter audio modality TTS",
			ConfigSchema: ConfigSchema{Fields: []FieldSchema{
				secretField("api_key", "API Key", "OpenRouter API key", true, 10),
				stringField("base_url", "Base URL", "Override the API base URL", false, "https://openrouter.ai/api/v1", 20),
			}},
			DefaultModel: "openrouter-tts",
			SupportsList: true,
			Models: []ModelInfo{{
				ID:           "openrouter-tts",
				Name:         "openrouter-tts",
				Description:  "Default OpenRouter speech wrapper model",
				TemplateOnly: true,
				ConfigSchema: ConfigSchema{Fields: []FieldSchema{
					advancedStringField("model", "Model", "Underlying OpenRouter model ID", false, "openai/gpt-audio-mini", 10),
					stringField("voice", "Voice", "Voice name", false, "coral", 20),
					numberField("speed", "Speed", "Speech rate", false, 1.0, 30),
				}},
				Capabilities: ModelCapabilities{ConfigSchema: ConfigSchema{Fields: []FieldSchema{
					advancedStringField("model", "Model", "Underlying OpenRouter model ID", false, "openai/gpt-audio-mini", 10),
					stringField("voice", "Voice", "Voice name", false, "coral", 20),
					numberField("speed", "Speed", "Speech rate", false, 1.0, 30),
				}}},
			}},
			Factory: func(config map[string]any) (sdk.SpeechProvider, error) {
				opts := []openrouterspeech.Option{}
				if v := configString(config, "api_key"); v != "" {
					opts = append(opts, openrouterspeech.WithAPIKey(v))
				}
				if v := configString(config, "base_url"); v != "" {
					opts = append(opts, openrouterspeech.WithBaseURL(v))
				}
				return openrouterspeech.New(opts...), nil
			},
			Order: 30,
		},
		{
			ClientType:  models.ClientTypeElevenLabsSpeech,
			DisplayName: "ElevenLabs Speech",
			Icon:        "elevenlabs",
			Description: "ElevenLabs text-to-speech",
			ConfigSchema: ConfigSchema{Fields: []FieldSchema{
				secretField("api_key", "API Key", "ElevenLabs API key", true, 10),
				stringField("base_url", "Base URL", "Override the API base URL", false, "https://api.elevenlabs.io", 20),
			}},
			DefaultModel: "elevenlabs-tts",
			SupportsList: true,
			Models: []ModelInfo{{
				ID:           "elevenlabs-tts",
				Name:         "elevenlabs-tts",
				Description:  "Default ElevenLabs speech wrapper model",
				TemplateOnly: true,
				ConfigSchema: ConfigSchema{Fields: []FieldSchema{
					stringField("voice_id", "Voice ID", "ElevenLabs voice ID", true, "", 10),
					advancedStringField("model_id", "Model ID", "ElevenLabs model ID", false, "eleven_multilingual_v2", 20),
					numberField("stability", "Stability", "Voice stability 0-1", false, 0.5, 30),
					numberField("similarity_boost", "Similarity Boost", "Voice similarity boost 0-1", false, 0.75, 40),
					numberField("style", "Style", "Speaking style intensity 0-1", false, 0, 50),
					boolField("use_speaker_boost", "Speaker Boost", "Enable speaker boost", false, 60),
					numberField("speed", "Speed", "Speech rate 0.5-2.0", false, 1.0, 70),
					stringField("output_format", "Output Format", "Output format", false, "mp3_44100_128", 80),
					numberField("seed", "Seed", "Deterministic seed", false, 0, 90),
					enumField("apply_text_normalization", "Text Normalization", "Text normalization mode", false, []string{"auto", "on", "off"}, 100),
					stringField("language_code", "Language Code", "Optional BCP-47 language code", false, "en-US", 110),
				}},
				Capabilities: ModelCapabilities{ConfigSchema: ConfigSchema{Fields: []FieldSchema{
					stringField("voice_id", "Voice ID", "ElevenLabs voice ID", true, "", 10),
					advancedStringField("model_id", "Model ID", "ElevenLabs model ID", false, "eleven_multilingual_v2", 20),
					numberField("stability", "Stability", "Voice stability 0-1", false, 0.5, 30),
					numberField("similarity_boost", "Similarity Boost", "Voice similarity boost 0-1", false, 0.75, 40),
					numberField("style", "Style", "Speaking style intensity 0-1", false, 0, 50),
					boolField("use_speaker_boost", "Speaker Boost", "Enable speaker boost", false, 60),
					numberField("speed", "Speed", "Speech rate 0.5-2.0", false, 1.0, 70),
					stringField("output_format", "Output Format", "Output format", false, "mp3_44100_128", 80),
					numberField("seed", "Seed", "Deterministic seed", false, 0, 90),
					enumField("apply_text_normalization", "Text Normalization", "Text normalization mode", false, []string{"auto", "on", "off"}, 100),
					stringField("language_code", "Language Code", "Optional BCP-47 language code", false, "en-US", 110),
				}}},
			}},
			Factory: func(config map[string]any) (sdk.SpeechProvider, error) {
				opts := []elevenlabsspeech.Option{}
				if v := configString(config, "api_key"); v != "" {
					opts = append(opts, elevenlabsspeech.WithAPIKey(v))
				}
				if v := configString(config, "base_url"); v != "" {
					opts = append(opts, elevenlabsspeech.WithBaseURL(v))
				}
				return elevenlabsspeech.New(opts...), nil
			},
			Order: 40,
		},
		{
			ClientType:  models.ClientTypeDeepgramSpeech,
			DisplayName: "Deepgram Speech",
			Icon:        "deepgram",
			Description: "Deepgram TTS",
			ConfigSchema: ConfigSchema{Fields: []FieldSchema{
				secretField("api_key", "API Key", "Deepgram API key", true, 10),
				stringField("base_url", "Base URL", "Override the API base URL", false, "https://api.deepgram.com", 20),
			}},
			DefaultModel: "deepgram-tts",
			SupportsList: false,
			Models: []ModelInfo{{
				ID:          "deepgram-tts",
				Name:        "deepgram-tts",
				Description: "Default Deepgram speech wrapper model",
				ConfigSchema: ConfigSchema{Fields: []FieldSchema{
					advancedStringField("model", "Model", "Deepgram voice model", false, "aura-2-asteria-en", 10),
					enumField("encoding", "Encoding", "Audio encoding", false, []string{"linear16", "mulaw", "alaw"}, 20),
					numberField("sample_rate", "Sample Rate", "Audio sample rate in Hz", false, 24000, 30),
					enumField("container", "Container", "Audio container", false, []string{"wav", "none"}, 40),
				}},
				Capabilities: ModelCapabilities{
					ConfigSchema: ConfigSchema{Fields: []FieldSchema{
						advancedStringField("model", "Model", "Deepgram voice model", false, "aura-2-asteria-en", 10),
						enumField("encoding", "Encoding", "Audio encoding", false, []string{"linear16", "mulaw", "alaw"}, 20),
						numberField("sample_rate", "Sample Rate", "Audio sample rate in Hz", false, 24000, 30),
						enumField("container", "Container", "Audio container", false, []string{"wav", "none"}, 40),
					}},
					Formats: []string{"wav", "none"},
				},
			}},
			Factory: func(config map[string]any) (sdk.SpeechProvider, error) {
				opts := []deepgramspeech.Option{}
				if v := configString(config, "api_key"); v != "" {
					opts = append(opts, deepgramspeech.WithAPIKey(v))
				}
				if v := configString(config, "base_url"); v != "" {
					opts = append(opts, deepgramspeech.WithBaseURL(v))
				}
				return deepgramspeech.New(opts...), nil
			},
			Order: 50,
		},
		{
			ClientType:  models.ClientTypeMiniMaxSpeech,
			DisplayName: "MiniMax Speech",
			Icon:        "minimax-color",
			Description: "MiniMax TTS",
			ConfigSchema: ConfigSchema{Fields: []FieldSchema{
				secretField("api_key", "API Key", "MiniMax API key", true, 10),
				stringField("base_url", "Base URL", "Override the API base URL", false, "https://api.minimax.io", 20),
			}},
			DefaultModel: "minimax-tts",
			SupportsList: false,
			Models: []ModelInfo{{
				ID:          "minimax-tts",
				Name:        "minimax-tts",
				Description: "Default MiniMax speech wrapper model",
				ConfigSchema: ConfigSchema{Fields: []FieldSchema{
					stringField("voice_id", "Voice ID", "MiniMax voice ID", false, "English_expressive_narrator", 10),
					advancedStringField("model", "Model", "MiniMax model", false, "speech-2.8-hd", 20),
					numberField("speed", "Speed", "Speech rate", false, 1.0, 30),
					numberField("vol", "Volume", "Volume", false, 1.0, 40),
					numberField("pitch", "Pitch", "Pitch adjustment", false, 0, 50),
					enumField("output_format", "Output Format", "Audio format", false, []string{"mp3", "pcm", "flac", "wav"}, 60),
					numberField("sample_rate", "Sample Rate", "Audio sample rate", false, 32000, 70),
				}},
				Capabilities: ModelCapabilities{
					ConfigSchema: ConfigSchema{Fields: []FieldSchema{
						stringField("voice_id", "Voice ID", "MiniMax voice ID", false, "English_expressive_narrator", 10),
						advancedStringField("model", "Model", "MiniMax model", false, "speech-2.8-hd", 20),
						numberField("speed", "Speed", "Speech rate", false, 1.0, 30),
						numberField("vol", "Volume", "Volume", false, 1.0, 40),
						numberField("pitch", "Pitch", "Pitch adjustment", false, 0, 50),
						enumField("output_format", "Output Format", "Audio format", false, []string{"mp3", "pcm", "flac", "wav"}, 60),
						numberField("sample_rate", "Sample Rate", "Audio sample rate", false, 32000, 70),
					}},
					Formats: []string{"mp3", "pcm", "flac", "wav"},
				},
			}},
			Factory: func(config map[string]any) (sdk.SpeechProvider, error) {
				opts := []minimaxspeech.Option{}
				if v := configString(config, "api_key"); v != "" {
					opts = append(opts, minimaxspeech.WithAPIKey(v))
				}
				if v := configString(config, "base_url"); v != "" {
					opts = append(opts, minimaxspeech.WithBaseURL(v))
				}
				return minimaxspeech.New(opts...), nil
			},
			Order: 60,
		},
		{
			ClientType:  models.ClientTypeVolcengineSpeech,
			DisplayName: "Volcengine Speech",
			Icon:        "volcengine-color",
			Description: "Volcengine SAMI TTS",
			ConfigSchema: ConfigSchema{Fields: []FieldSchema{
				secretField("access_key", "Access Key", "Volcengine access key", true, 10),
				secretField("secret_key", "Secret Key", "Volcengine secret key", true, 20),
				secretField("app_key", "App Key", "SAMI app key", true, 30),
				stringField("base_url", "Base URL", "Override the API base URL", false, "https://sami.bytedance.com", 40),
			}},
			DefaultModel: "sami-tts",
			SupportsList: false,
			Models: []ModelInfo{{
				ID:          "sami-tts",
				Name:        "sami-tts",
				Description: "Default Volcengine SAMI speech wrapper model",
				ConfigSchema: ConfigSchema{Fields: []FieldSchema{
					stringField("speaker", "Speaker", "Speaker ID", true, "", 10),
					enumField("encoding", "Encoding", "Output encoding", false, []string{"mp3", "wav", "aac"}, 20),
					numberField("sample_rate", "Sample Rate", "Audio sample rate", false, 24000, 30),
					numberField("speech_rate", "Speech Rate", "Speech rate [-50,100]", false, 0, 40),
					numberField("pitch_rate", "Pitch Rate", "Pitch rate [-12,12]", false, 0, 50),
				}},
				Capabilities: ModelCapabilities{
					ConfigSchema: ConfigSchema{Fields: []FieldSchema{
						stringField("speaker", "Speaker", "Speaker ID", true, "", 10),
						enumField("encoding", "Encoding", "Output encoding", false, []string{"mp3", "wav", "aac"}, 20),
						numberField("sample_rate", "Sample Rate", "Audio sample rate", false, 24000, 30),
						numberField("speech_rate", "Speech Rate", "Speech rate [-50,100]", false, 0, 40),
						numberField("pitch_rate", "Pitch Rate", "Pitch rate [-12,12]", false, 0, 50),
					}},
					Formats: []string{"mp3", "wav", "aac"},
				},
			}},
			Factory: func(config map[string]any) (sdk.SpeechProvider, error) {
				opts := []volcenginespeech.Option{}
				if v := configString(config, "access_key"); v != "" {
					opts = append(opts, volcenginespeech.WithAccessKey(v))
				}
				if v := configString(config, "secret_key"); v != "" {
					opts = append(opts, volcenginespeech.WithSecretKey(v))
				}
				if v := configString(config, "app_key"); v != "" {
					opts = append(opts, volcenginespeech.WithAppKey(v))
				}
				if v := configString(config, "base_url"); v != "" {
					opts = append(opts, volcenginespeech.WithBaseURL(v))
				}
				return volcenginespeech.New(opts...), nil
			},
			Order: 70,
		},
		{
			ClientType:  models.ClientTypeAlibabaSpeech,
			DisplayName: "Alibaba Cloud Speech",
			Icon:        "bailian-color",
			Description: "DashScope CosyVoice TTS",
			ConfigSchema: ConfigSchema{Fields: []FieldSchema{
				secretField("api_key", "API Key", "DashScope API key", true, 10),
				stringField("base_url", "Base URL", "Override the WebSocket endpoint", false, "wss://dashscope.aliyuncs.com/api-ws/v1/inference/", 20),
			}},
			DefaultModel: "cosyvoice-tts",
			SupportsList: false,
			Models: []ModelInfo{{
				ID:          "cosyvoice-tts",
				Name:        "cosyvoice-tts",
				Description: "Default DashScope CosyVoice wrapper model",
				ConfigSchema: ConfigSchema{Fields: []FieldSchema{
					advancedStringField("model", "Model", "DashScope model ID", false, "cosyvoice-v1", 10),
					stringField("voice", "Voice", "Voice or custom clone ID", true, "", 20),
					enumField("format", "Format", "Audio format", false, []string{"mp3", "wav", "pcm", "opus"}, 30),
					numberField("sample_rate", "Sample Rate", "Audio sample rate", false, 22050, 40),
					numberField("volume", "Volume", "Volume 0-100", false, 50, 50),
					numberField("rate", "Rate", "Speech rate 0.5-2.0", false, 1.0, 60),
					numberField("pitch", "Pitch", "Pitch multiplier 0.5-2.0", false, 1.0, 70),
				}},
				Capabilities: ModelCapabilities{
					ConfigSchema: ConfigSchema{Fields: []FieldSchema{
						advancedStringField("model", "Model", "DashScope model ID", false, "cosyvoice-v1", 10),
						stringField("voice", "Voice", "Voice or custom clone ID", true, "", 20),
						enumField("format", "Format", "Audio format", false, []string{"mp3", "wav", "pcm", "opus"}, 30),
						numberField("sample_rate", "Sample Rate", "Audio sample rate", false, 22050, 40),
						numberField("volume", "Volume", "Volume 0-100", false, 50, 50),
						numberField("rate", "Rate", "Speech rate 0.5-2.0", false, 1.0, 60),
						numberField("pitch", "Pitch", "Pitch multiplier 0.5-2.0", false, 1.0, 70),
					}},
					Formats: []string{"mp3", "wav", "pcm", "opus"},
				},
			}},
			Factory: func(config map[string]any) (sdk.SpeechProvider, error) {
				opts := []alibabaspeech.Option{}
				if v := configString(config, "api_key"); v != "" {
					opts = append(opts, alibabaspeech.WithAPIKey(v))
				}
				if v := configString(config, "base_url"); v != "" {
					opts = append(opts, alibabaspeech.WithBaseURL(v))
				}
				return alibabaspeech.New(opts...), nil
			},
			Order: 80,
		},
		{
			ClientType:  models.ClientTypeMicrosoftSpeech,
			DisplayName: "Microsoft Speech",
			Icon:        "azure-color",
			Description: "Azure Cognitive Services TTS",
			ConfigSchema: ConfigSchema{Fields: []FieldSchema{
				secretField("api_key", "API Key", "Azure speech subscription key", true, 10),
				stringField("base_url", "Base URL", "Optional full TTS endpoint override", false, "", 20),
			}},
			DefaultModel: "microsoft-tts",
			SupportsList: false,
			Models: []ModelInfo{{
				ID:          "microsoft-tts",
				Name:        "microsoft-tts",
				Description: "Default Azure speech wrapper model",
				ConfigSchema: ConfigSchema{Fields: []FieldSchema{
					stringField("region", "Region", "Azure region, e.g. eastus", false, "eastus", 10),
					stringField("voice", "Voice", "Azure voice name", false, "en-US-JennyNeural", 20),
					stringField("language", "Language", "Optional BCP-47 language tag", false, "en-US", 30),
					stringField("output_format", "Output Format", "Azure output format", false, "audio-16khz-128kbitrate-mono-mp3", 40),
					stringField("style", "Style", "Optional speaking style", false, "", 50),
					stringField("rate", "Rate", "Optional speaking rate", false, "", 60),
					stringField("pitch", "Pitch", "Optional pitch adjustment", false, "", 70),
				}},
				Capabilities: ModelCapabilities{ConfigSchema: ConfigSchema{Fields: []FieldSchema{
					stringField("region", "Region", "Azure region, e.g. eastus", false, "eastus", 10),
					stringField("voice", "Voice", "Azure voice name", false, "en-US-JennyNeural", 20),
					stringField("language", "Language", "Optional BCP-47 language tag", false, "en-US", 30),
					stringField("output_format", "Output Format", "Azure output format", false, "audio-16khz-128kbitrate-mono-mp3", 40),
					stringField("style", "Style", "Optional speaking style", false, "", 50),
					stringField("rate", "Rate", "Optional speaking rate", false, "", 60),
					stringField("pitch", "Pitch", "Optional pitch adjustment", false, "", 70),
				}}},
			}},
			Factory: func(config map[string]any) (sdk.SpeechProvider, error) {
				opts := []microsoftspeech.Option{}
				if v := configString(config, "api_key"); v != "" {
					opts = append(opts, microsoftspeech.WithAPIKey(v))
				}
				if v := configString(config, "base_url"); v != "" {
					opts = append(opts, microsoftspeech.WithBaseURL(v))
				}
				return microsoftspeech.New(opts...), nil
			},
			Order: 90,
		},
	}
}

func stringField(key, title, description string, required bool, example any, order int) FieldSchema {
	return FieldSchema{Key: key, Type: "string", Title: title, Description: description, Required: required, Example: example, Order: order}
}

func advancedStringField(key, title, description string, required bool, example any, order int) FieldSchema {
	return FieldSchema{Key: key, Type: "string", Title: title, Description: description, Required: required, Advanced: true, Example: example, Order: order}
}

func secretField(key, title, description string, required bool, order int) FieldSchema {
	return FieldSchema{Key: key, Type: "secret", Title: title, Description: description, Required: required, Order: order}
}

func numberField(key, title, description string, required bool, example any, order int) FieldSchema {
	return FieldSchema{Key: key, Type: "number", Title: title, Description: description, Required: required, Example: example, Order: order}
}

func boolField(key, title, description string, required bool, order int) FieldSchema {
	return FieldSchema{Key: key, Type: "bool", Title: title, Description: description, Required: required, Order: order}
}

func enumField(key, title, description string, required bool, values []string, order int) FieldSchema {
	return FieldSchema{Key: key, Type: "enum", Title: title, Description: description, Required: required, Enum: values, Order: order}
}

func configString(cfg map[string]any, key string) string {
	if cfg == nil {
		return ""
	}
	if v, ok := cfg[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func voiceIDs(voices []VoiceInfo) []string {
	out := make([]string, 0, len(voices))
	for _, voice := range voices {
		out = append(out, voice.ID)
	}
	return out
}
