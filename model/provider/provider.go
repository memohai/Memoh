package provider

type Provider int

const (
    OpenAI Provider = iota
    Anthropic
    Google
)

func (p Provider) String() string {
    return []string{
        "openai",
        "anthropic",
        "google",
    }[p]
}