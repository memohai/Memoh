package channel

// ChannelCapabilities 描述通道在功能层面的能力矩阵。
// 该结构用于上层自适应逻辑，不依赖具体适配器实现。
type ChannelCapabilities struct {
	Text           bool     `json:"text"`
	Markdown       bool     `json:"markdown"`
	RichText       bool     `json:"rich_text"`
	Attachments    bool     `json:"attachments"`
	Media          bool     `json:"media"`
	Reactions      bool     `json:"reactions"`
	Buttons        bool     `json:"buttons"`
	Reply          bool     `json:"reply"`
	Threads        bool     `json:"threads"`
	Streaming      bool     `json:"streaming"`
	Polls          bool     `json:"polls"`
	Edit           bool     `json:"edit"`
	Unsend         bool     `json:"unsend"`
	NativeCommands bool     `json:"native_commands"`
	BlockStreaming bool     `json:"block_streaming"`
	ChatTypes      []string `json:"chat_types,omitempty"`
}
