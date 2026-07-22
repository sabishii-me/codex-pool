package main

import (
	"bytes"
	"encoding/json"
)

// responseBodyCapture retains bounded prefix and suffix views while relaying a
// response unchanged. Prefix supports diagnostics and existing identifiers;
// suffix preserves terminal non-streaming usage metadata without buffering an
// arbitrarily large response body.
type responseBodyCapture struct {
	limit  int
	total  int64
	prefix bytes.Buffer
	tail   []byte
}

func newResponseBodyCapture(limit int64) *responseBodyCapture {
	if limit < 1 {
		limit = 1
	}
	return &responseBodyCapture{limit: int(limit)}
}

func (capture *responseBodyCapture) Write(p []byte) (int, error) {
	if capture == nil {
		return len(p), nil
	}
	capture.total += int64(len(p))
	if capture.prefix.Len() < capture.limit {
		remaining := capture.limit - capture.prefix.Len()
		if remaining > len(p) {
			remaining = len(p)
		}
		_, _ = capture.prefix.Write(p[:remaining])
	}
	if len(p) >= capture.limit {
		capture.tail = append(capture.tail[:0], p[len(p)-capture.limit:]...)
	} else {
		if overflow := len(capture.tail) + len(p) - capture.limit; overflow > 0 {
			copy(capture.tail, capture.tail[overflow:])
			capture.tail = capture.tail[:len(capture.tail)-overflow]
		}
		capture.tail = append(capture.tail, p...)
	}
	return len(p), nil
}

func (capture *responseBodyCapture) Bytes() []byte {
	if capture == nil {
		return nil
	}
	return capture.prefix.Bytes()
}

func (capture *responseBodyCapture) TailBytes() []byte {
	if capture == nil {
		return nil
	}
	return capture.tail
}

func (capture *responseBodyCapture) Len() int {
	if capture == nil {
		return 0
	}
	return capture.prefix.Len()
}

func (capture *responseBodyCapture) Truncated() bool {
	return capture != nil && capture.total > int64(capture.prefix.Len())
}

// protocolUsageObjectFromJSONTail reconstructs only the final top-level usage
// object from a bounded JSON suffix. It does not attempt to reconstruct output
// content that preceded the suffix.
func protocolUsageObjectFromJSONTail(tail []byte, defaultModel string) map[string]any {
	usageKey := []byte(`"usage"`)
	index := bytes.LastIndex(tail, usageKey)
	if index < 0 {
		return nil
	}
	remainder := tail[index+len(usageKey):]
	colon := bytes.IndexByte(remainder, ':')
	if colon < 0 {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(remainder[colon+1:]))
	var usage map[string]any
	if err := decoder.Decode(&usage); err != nil || len(usage) == 0 {
		return nil
	}
	object := map[string]any{"usage": usage}
	if defaultModel != "" {
		object["model"] = defaultModel
	}
	return object
}
