package main

import (
	"bytes"
	"encoding/json"
	"io"
)

// visionTransparentWriter rewrites the top-level "model" field in responses so
// a vision-fallback route stays attribution-transparent to the downstream
// client. It handles both non-streaming JSON payloads and SSE event streams
// (buffering only the current event, never the whole response).
type visionTransparentWriter struct {
	w          io.Writer
	model      string
	write      func(p []byte) (int, error)
	buf        []byte
	sse        bool
	sawNonSSE  bool
	wroteAny   bool
	flushedAny bool
}

func newVisionTransparentWriter(w io.Writer, model string) *visionTransparentWriter {
	vt := &visionTransparentWriter{w: w, model: model, write: w.Write}
	return vt
}

func (vt *visionTransparentWriter) Write(p []byte) (int, error) {
	if vt.sse {
		return vt.writeSSE(p)
	}
	return vt.writeJSON(p)
}

func (vt *visionTransparentWriter) writeJSON(p []byte) (int, error) {
	vt.buf = append(vt.buf, p...)
	body := append([]byte(nil), vt.buf...)
	vt.buf = vt.buf[:0]
	return vt.wroteAll(body)
}

func (vt *visionTransparentWriter) wroteAll(body []byte) (int, error) {
	rewritten := rewriteModelFieldInJSON(body, vt.model)
	if rewritten == nil {
		return vt.write(body)
	}
	return vt.write(rewritten)
}

func (vt *visionTransparentWriter) writeSSE(p []byte) (int, error) {
	vt.buf = append(vt.buf, p...)
	for {
		idx := bytes.Index(vt.buf, []byte("\n\n"))
		advance := 2
		if idx < 0 {
			idx = bytes.Index(vt.buf, []byte("\r\n\r\n"))
			advance = 4
			if idx < 0 {
				break
			}
		}
		event := append([]byte(nil), vt.buf[:idx]...)
		vt.buf = vt.buf[idx+advance:]
		if err := vt.writeEvent(event); err != nil {
			return len(p), err
		}
	}
	return len(p), nil
}

func (vt *visionTransparentWriter) writeEvent(event []byte) error {
	data := extractSSEEventData(event)
	rewrittenData := rewriteModelFieldInJSON(data, vt.model)
	if rewrittenData != nil && !bytes.Equal(rewrittenData, data) {
		// Rebuild the event with the rewritten data line.
		event = rewriteSSEDataLine(event, data, rewrittenData)
	}
	_, err := vt.write(event)
	return err
}

func (vt *visionTransparentWriter) Flush() error {
	if len(vt.buf) > 0 {
		// Trailing bytes without a terminator: forward as-is (non-SSE tail or
		// truncated stream). Avoid corrupting JSON by only rewriting complete
		// objects; here we forward raw.
		if !vt.sse {
			if _, err := vt.wroteAll(vt.buf); err != nil {
				return err
			}
		} else {
			if _, err := vt.write(vt.buf); err != nil {
				return err
			}
		}
		vt.buf = nil
	}
	return nil
}

// rewriteModelFieldInJSON rewrites a top-level "model" string field in a JSON
// payload, returning nil if the payload has no model field or cannot be parsed.
func rewriteModelFieldInJSON(body []byte, model string) []byte {
	if len(body) == 0 || model == "" {
		return nil
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil
	}
	if _, ok := obj["model"]; !ok {
		return nil
	}
	obj["model"] = model
	rewritten, err := json.Marshal(obj)
	if err != nil {
		return nil
	}
	return rewritten
}

// rewriteSSEDataLine replaces the data: line inside an SSE event while keeping
// the surrounding event framing (event:/data:/id:/retry:) intact.
func rewriteSSEDataLine(event, oldData, newData []byte) []byte {
	marker := []byte("data:")
	idx := bytes.Index(event, marker)
	if idx < 0 {
		return event
	}
	lineStart := idx
	lineEnd := bytes.IndexByte(event[lineStart:], '\n')
	if lineEnd < 0 {
		lineEnd = len(event)
	} else {
		lineEnd += lineStart
	}
	prefix := event[:lineStart]
	suffix := event[lineEnd:]
	out := make([]byte, 0, len(prefix)+len(newData)+len(suffix))
	out = append(out, prefix...)
	out = append(out, newData...)
	out = append(out, suffix...)
	return out
}
