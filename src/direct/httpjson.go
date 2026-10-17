package direct

import (
	"encoding/json"
	"fmt"
	"io"
)

func decodeBoundedJSON(reader io.Reader, maximumBytes int64, destination any) error {
	if maximumBytes <= 0 {
		return fmt.Errorf("JSON response limit must be positive")
	}
	body, err := io.ReadAll(io.LimitReader(reader, maximumBytes+1))
	if err != nil {
		return fmt.Errorf("read JSON response: %w", err)
	}
	if int64(len(body)) > maximumBytes {
		return fmt.Errorf("JSON response exceeds %d bytes", maximumBytes)
	}
	if err := json.Unmarshal(body, destination); err != nil {
		return fmt.Errorf("decode JSON response: %w", err)
	}
	return nil
}
