package agent

import (
	"context"
	"log"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/googlegenai"

    "github.com/memohai/Memoh/model"
    "github.com/memohai/Memoh/agent/prompts"
)

type AgentParams struct {
    Model model.Model
}

type AgentInput struct {
    content string
}

type AgentOperations struct {
    Ask func(input AgentInput) (string, error)
}

func NewAgent(params AgentParams) AgentOperations {
    return AgentOperations{
        Ask: func(input AgentInput) (string, error) {
            return "", nil
        },
    }
}
