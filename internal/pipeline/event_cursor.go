package pipeline

import (
	"errors"
	"fmt"
)

const maxJSONSafeEventCursor int64 = 1<<53 - 1

func assignEventCursor(event CanonicalEvent, cursor int64) (CanonicalEvent, error) {
	if event == nil {
		return nil, errors.New("canonical event is nil")
	}
	if cursor <= 0 || cursor > maxJSONSafeEventCursor {
		return nil, fmt.Errorf("event cursor %d is outside the JSON-safe range", cursor)
	}
	switch typed := event.(type) {
	case MessageEvent:
		typed.EventCursor = cursor
		return typed, nil
	case EditEvent:
		typed.EventCursor = cursor
		return typed, nil
	case DeleteEvent:
		typed.EventCursor = cursor
		return typed, nil
	case ServiceEvent:
		typed.EventCursor = cursor
		return typed, nil
	case *MessageEvent:
		if typed == nil {
			return nil, errors.New("canonical message event is nil")
		}
		cloned := *typed
		cloned.EventCursor = cursor
		return cloned, nil
	case *EditEvent:
		if typed == nil {
			return nil, errors.New("canonical edit event is nil")
		}
		cloned := *typed
		cloned.EventCursor = cursor
		return cloned, nil
	case *DeleteEvent:
		if typed == nil {
			return nil, errors.New("canonical delete event is nil")
		}
		cloned := *typed
		cloned.EventCursor = cursor
		return cloned, nil
	case *ServiceEvent:
		if typed == nil {
			return nil, errors.New("canonical service event is nil")
		}
		cloned := *typed
		cloned.EventCursor = cursor
		return cloned, nil
	default:
		return nil, fmt.Errorf("unsupported canonical event type %T", event)
	}
}
