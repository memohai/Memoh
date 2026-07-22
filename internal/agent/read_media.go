package agent

import (
	"fmt"
	"strings"
	"sync"

	sdk "github.com/memohai/twilight-ai/sdk"

	agenttools "github.com/memohai/memoh/internal/agent/tools"
	"github.com/memohai/memoh/internal/models"
)

func decorateReadMediaTools(model *sdk.Model, tools []sdk.Tool) ([]sdk.Tool, *readMediaDecorationState) {
	if len(tools) == 0 {
		return tools, nil
	}

	clientType := models.ResolveClientType(model)
	state := &readMediaDecorationState{
		pendingImages: make(map[string]sdk.ImagePart),
	}
	wrapped := make([]sdk.Tool, 0, len(tools))
	found := false

	for _, tool := range tools {
		if tool.Name != agenttools.ReadMediaToolName().String() || tool.Execute == nil {
			wrapped = append(wrapped, tool)
			continue
		}

		found = true
		originalExecute := tool.Execute
		toolCopy := tool
		toolCopy.Execute = func(ctx *sdk.ToolExecContext, input any) (any, error) {
			output, err := originalExecute(ctx, input)
			if err != nil {
				return output, err
			}

			publicResult, image, ok := normalizeReadMediaOutput(output, clientType)
			if !ok {
				return output, nil
			}
			if ctx != nil && strings.TrimSpace(ctx.ToolCallID) != "" && strings.TrimSpace(image.Image) != "" {
				state.mu.Lock()
				if _, exists := state.pendingImages[ctx.ToolCallID]; !exists {
					state.pendingOrder = append(state.pendingOrder, ctx.ToolCallID)
				}
				state.pendingImages[ctx.ToolCallID] = image
				state.mu.Unlock()
			}
			return publicResult, nil
		}
		wrapped = append(wrapped, toolCopy)
	}

	if !found {
		return tools, nil
	}

	return wrapped, state
}

type readMediaDecorationState struct {
	mu            sync.Mutex
	pendingOrder  []string
	pendingImages map[string]sdk.ImagePart
	prepareCalls  int
	injections    []readMediaInjection
	readyMessages []sdk.Message

	syntheticRowRecorder func(role string)
}

type readMediaInjection struct {
	afterStep int
	message   sdk.Message
}

func (s *readMediaDecorationState) prepareStep(params *sdk.GenerateParams) *sdk.GenerateParams {
	if s == nil || params == nil {
		return nil
	}

	afterStep := s.prepareCalls
	s.prepareCalls++

	s.mu.Lock()
	defer s.mu.Unlock()
	messages := append([]sdk.Message(nil), s.readyMessages...)
	s.readyMessages = s.readyMessages[:0]
	if message, ok := s.takePendingMessageLocked(); ok {
		s.injections = append(s.injections, readMediaInjection{
			afterStep: afterStep,
			message:   message,
		})
		messages = append(messages, message)
	}
	if len(messages) == 0 {
		return nil
	}

	next := *params
	next.Messages = append(append([]sdk.Message(nil), params.Messages...), messages...)
	return &next
}

func (s *readMediaDecorationState) completeStep(stepIndex int, step *sdk.StepResult) *sdk.StepResult {
	if s == nil || step == nil {
		return step
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	message, ok := s.takePendingMessageLocked()
	if !ok {
		return step
	}
	s.injections = append(s.injections, readMediaInjection{
		afterStep: stepIndex,
		message:   message,
	})
	s.readyMessages = append(s.readyMessages, message)

	next := *step
	next.Messages = append(append([]sdk.Message(nil), step.Messages...), message)
	return &next
}

func (s *readMediaDecorationState) takePendingMessageLocked() (sdk.Message, bool) {
	parts := make([]sdk.MessagePart, 0, len(s.pendingOrder))
	for _, toolCallID := range s.pendingOrder {
		image, ok := s.pendingImages[toolCallID]
		delete(s.pendingImages, toolCallID)
		if !ok || strings.TrimSpace(image.Image) == "" {
			continue
		}
		parts = append(parts, image)
	}
	s.pendingOrder = s.pendingOrder[:0]

	if len(parts) == 0 {
		return sdk.Message{}, false
	}

	message := sdk.Message{
		Role:    sdk.MessageRoleUser,
		Content: parts,
	}
	if s.syntheticRowRecorder != nil {
		s.syntheticRowRecorder(string(message.Role))
	}
	return message, true
}

func (s *readMediaDecorationState) mergeMessages(steps []sdk.StepResult, fallback []sdk.Message) []sdk.Message {
	if s == nil || len(s.injections) == 0 {
		return fallback
	}
	if len(steps) == 0 {
		merged := append([]sdk.Message(nil), fallback...)
		for _, injection := range s.injections {
			merged = append(merged, injection.message)
		}
		return merged
	}

	merged := make([]sdk.Message, 0, len(fallback)+len(s.injections))
	injectionIndex := 0
	for stepIndex, step := range steps {
		merged = append(merged, step.Messages...)
		for injectionIndex < len(s.injections) && s.injections[injectionIndex].afterStep == stepIndex {
			merged = append(merged, s.injections[injectionIndex].message)
			injectionIndex++
		}
	}
	for injectionIndex < len(s.injections) {
		merged = append(merged, s.injections[injectionIndex].message)
		injectionIndex++
	}
	return merged
}

func normalizeReadMediaOutput(output any, clientType string) (any, sdk.ImagePart, bool) {
	switch value := output.(type) {
	case agenttools.ReadMediaToolOutput:
		return value.Public, buildReadMediaImagePart(clientType, value.ImageBase64, value.ImageMediaType), true
	case *agenttools.ReadMediaToolOutput:
		if value == nil {
			return nil, sdk.ImagePart{}, false
		}
		return value.Public, buildReadMediaImagePart(clientType, value.ImageBase64, value.ImageMediaType), true
	default:
		return nil, sdk.ImagePart{}, false
	}
}

func publicReadMediaToolResult(output any) any {
	publicResult, _, ok := normalizeReadMediaOutput(output, "")
	if !ok {
		return output
	}
	return publicResult
}

func buildReadMediaImagePart(clientType, imageBase64, mediaType string) sdk.ImagePart {
	imageBase64 = strings.TrimSpace(imageBase64)
	mediaType = strings.TrimSpace(mediaType)
	if imageBase64 == "" {
		return sdk.ImagePart{}
	}
	if mediaType == "" {
		mediaType = "image/png"
	}

	image := imageBase64
	if clientType != string(models.ClientTypeAnthropicMessages) {
		image = fmt.Sprintf("data:%s;base64,%s", mediaType, imageBase64)
	}
	return sdk.ImagePart{
		Image:     image,
		MediaType: mediaType,
	}
}
