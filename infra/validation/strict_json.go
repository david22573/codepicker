package validation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const MaxArgLength = 5 * 1024 * 1024 // 5MB limit for arguments

// DecodeStrict validates the payload size, cleans markdown, and performs a strict JSON unmarshal.
func DecodeStrict(args string, v interface{}) error {
	if len(args) > MaxArgLength {
		return fmt.Errorf("validation failed: argument payload exceeds maximum allowed size")
	}

	cleanArgs := strings.TrimSpace(args)
	if strings.HasPrefix(cleanArgs, "```json") {
		cleanArgs = cleanArgs[7:]
	} else if strings.HasPrefix(cleanArgs, "```") {
		cleanArgs = cleanArgs[3:]
	}
	if strings.HasSuffix(cleanArgs, "```") {
		cleanArgs = cleanArgs[:len(cleanArgs)-3]
	}

	dec := json.NewDecoder(bytes.NewReader([]byte(strings.TrimSpace(cleanArgs))))
	dec.DisallowUnknownFields() // Reject any hallucinations of unknown parameters

	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("strict JSON decoding failed: %w", err)
	}

	return nil
}
