package channel

type FieldType string

const (
	FieldString FieldType = "string"
	FieldSecret FieldType = "secret"
	FieldBool   FieldType = "bool"
	FieldNumber FieldType = "number"
	FieldEnum   FieldType = "enum"
)

// FieldSchema 定义单个配置字段的结构化描述。
type FieldSchema struct {
	Type        FieldType `json:"type"`
	Required    bool      `json:"required"`
	Title       string    `json:"title,omitempty"`
	Description string    `json:"description,omitempty"`
	Enum        []string  `json:"enum,omitempty"`
	Example     any       `json:"example,omitempty"`
}

// ConfigSchema 描述通道配置或用户绑定的结构。
type ConfigSchema struct {
	Version int                    `json:"version"`
	Fields  map[string]FieldSchema `json:"fields"`
}
