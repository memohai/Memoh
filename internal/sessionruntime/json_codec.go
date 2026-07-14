package sessionruntime

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

func marshalRuntimeJSON(value any) ([]byte, error) {
	return json.Marshal(value)
}

func unmarshalRuntimeJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("runtime JSON contains multiple values")
		}
		return fmt.Errorf("decode trailing runtime JSON: %w", err)
	}
	return nil
}

func cloneRuntimeJSON(source, target any) error {
	data, err := marshalRuntimeJSON(source)
	if err != nil {
		return err
	}
	return unmarshalRuntimeJSON(data, target)
}

func equalRuntimeJSON(left, right any) (bool, error) {
	leftData, err := marshalRuntimeJSON(left)
	if err != nil {
		return false, err
	}
	rightData, err := marshalRuntimeJSON(right)
	if err != nil {
		return false, err
	}
	return bytes.Equal(leftData, rightData), nil
}
