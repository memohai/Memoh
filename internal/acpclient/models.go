package acpclient

import (
	"context"
	"errors"
	"fmt"
	"strings"

	acp "github.com/coder/acp-go-sdk"
)

var (
	ErrModelSelectionUnsupported = errors.New("ACP agent does not expose selectable models")
	ErrModelUnavailable          = errors.New("ACP model is not available for this session")
	ErrModelIDRequired           = errors.New("model_id is required")
	ErrSessionNotInitialized     = errors.New("ACP session is not initialized")
	ErrSessionClosed             = errors.New("ACP session is closed")
	ErrPromptRequired            = errors.New("prompt is required")
	ErrImagePromptUnsupported    = errors.New("ACP agent does not support image prompts")
	ErrInvalidPromptImage        = errors.New("invalid ACP prompt image")
)

type ModelInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type ModelState struct {
	Supported      bool        `json:"supported"`
	CurrentModelID string      `json:"current_model_id,omitempty"`
	Available      []ModelInfo `json:"available_models,omitempty"`
}

func modelStateFromACP(state *acp.SessionModelState) ModelState {
	if state == nil {
		return ModelState{Supported: false}
	}
	out := ModelState{
		Supported:      true,
		CurrentModelID: strings.TrimSpace(string(state.CurrentModelId)),
		Available:      make([]ModelInfo, 0, len(state.AvailableModels)),
	}
	for _, model := range state.AvailableModels {
		id := strings.TrimSpace(string(model.ModelId))
		name := strings.TrimSpace(model.Name)
		if id == "" && name == "" {
			continue
		}
		if name == "" {
			name = id
		}
		item := ModelInfo{ID: id, Name: name}
		if model.Description != nil {
			item.Description = strings.TrimSpace(*model.Description)
		}
		out.Available = append(out.Available, item)
	}
	return out
}

func cloneModelState(state ModelState) ModelState {
	state.Available = append([]ModelInfo(nil), state.Available...)
	return state
}

func (s *Session) ModelState() ModelState {
	if s == nil {
		return ModelState{Supported: false}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneModelState(s.modelState)
}

func (s *Session) SetModel(ctx context.Context, modelID string) (ModelState, error) {
	if s == nil {
		return ModelState{Supported: false}, ErrSessionNotInitialized
	}
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return s.ModelState(), ErrModelIDRequired
	}

	s.mu.Lock()
	if s.conn == nil {
		s.mu.Unlock()
		return ModelState{Supported: false}, ErrSessionNotInitialized
	}
	if s.closed {
		state := cloneModelState(s.modelState)
		s.mu.Unlock()
		return state, ErrSessionClosed
	}
	if !s.modelState.Supported {
		state := cloneModelState(s.modelState)
		s.mu.Unlock()
		return state, ErrModelSelectionUnsupported
	}
	available := false
	for _, model := range s.modelState.Available {
		if model.ID == modelID {
			available = true
			break
		}
	}
	if !available {
		state := cloneModelState(s.modelState)
		s.mu.Unlock()
		return state, fmt.Errorf("%w: %s", ErrModelUnavailable, modelID)
	}
	conn := s.conn
	sessionID := s.sessionID
	s.mu.Unlock()

	if _, err := conn.UnstableSetSessionModel(ctx, acp.UnstableSetSessionModelRequest{
		SessionId: sessionID,
		ModelId:   acp.UnstableModelId(modelID),
	}); err != nil {
		return s.ModelState(), err
	}

	s.mu.Lock()
	s.modelState.CurrentModelID = modelID
	state := cloneModelState(s.modelState)
	s.mu.Unlock()
	return state, nil
}
