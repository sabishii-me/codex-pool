package main

import (
	"bytes"
	"fmt"
	"io"
)

func copyHostedMCPFilteredSSE(destination io.Writer, source io.Reader, limit int64) error {
	filter := newHostedMCPResponseFilterWriter(destination, limit)
	_, copyErr := io.Copy(filter, source)
	if finalizeErr := filter.Finalize(); copyErr == nil {
		copyErr = finalizeErr
	}
	return copyErr
}

type hostedMCPResponseFilterWriter struct {
	writer io.Writer
	limit  int64
	buffer []byte
}

func newHostedMCPResponseFilterWriter(writer io.Writer, limit int64) *hostedMCPResponseFilterWriter {
	return &hostedMCPResponseFilterWriter{writer: writer, limit: limit}
}

func (filter *hostedMCPResponseFilterWriter) Write(data []byte) (int, error) {
	filter.buffer = append(filter.buffer, data...)
	for {
		event, advance, ok := nextCompleteSSEEvent(filter.buffer)
		if !ok {
			if filter.limit > 0 && int64(len(filter.buffer)) > filter.limit {
				return len(data), fmt.Errorf("Responses SSE event exceeded bounded transformation limit of %d bytes", filter.limit)
			}
			return len(data), nil
		}
		raw := filter.buffer[:advance]
		filter.buffer = filter.buffer[advance:]
		eventName, payload := parseSSEEvent(event)
		if len(payload) == 0 || bytes.Equal(bytes.TrimSpace(payload), []byte("[DONE]")) {
			if _, err := filter.writer.Write(raw); err != nil {
				return len(data), err
			}
			continue
		}
		filtered, drop, changed, err := filterHostedMCPResponseJSON(bytes.TrimSpace(payload), filter.limit)
		if err != nil {
			return len(data), err
		}
		if drop {
			continue
		}
		if !changed {
			if _, err := filter.writer.Write(raw); err != nil {
				return len(data), err
			}
			continue
		}
		if eventName != "" {
			if _, err := fmt.Fprintf(filter.writer, "event: %s\ndata: %s\n\n", eventName, filtered); err != nil {
				return len(data), err
			}
		} else if _, err := fmt.Fprintf(filter.writer, "data: %s\n\n", filtered); err != nil {
			return len(data), err
		}
	}
}

func (filter *hostedMCPResponseFilterWriter) Finalize() error {
	if len(filter.buffer) == 0 {
		return nil
	}
	if filter.limit > 0 && int64(len(filter.buffer)) > filter.limit {
		return fmt.Errorf("Responses SSE event exceeded bounded transformation limit of %d bytes", filter.limit)
	}
	// A trailing event without a blank-line delimiter is still a complete SSE
	// event at EOF, matching the gateway's stream finalization contract.
	raw := append([]byte(nil), filter.buffer...)
	eventName, payload := parseSSEEvent(bytes.TrimSpace(raw))
	if len(payload) == 0 {
		_, err := filter.writer.Write(raw)
		filter.buffer = nil
		return err
	}
	filtered, drop, changed, err := filterHostedMCPResponseJSON(bytes.TrimSpace(payload), filter.limit)
	if err != nil {
		return err
	}
	filter.buffer = nil
	if drop {
		return nil
	}
	if !changed {
		_, err = filter.writer.Write(raw)
		return err
	}
	if eventName != "" {
		_, err = fmt.Fprintf(filter.writer, "event: %s\ndata: %s\n\n", eventName, filtered)
	} else {
		_, err = fmt.Fprintf(filter.writer, "data: %s\n\n", filtered)
	}
	return err
}

func nextCompleteSSEEvent(buffer []byte) ([]byte, int, bool) {
	if index := bytes.Index(buffer, []byte("\n\n")); index >= 0 {
		return buffer[:index], index + 2, true
	}
	if index := bytes.Index(buffer, []byte("\r\n\r\n")); index >= 0 {
		return buffer[:index], index + 4, true
	}
	return nil, 0, false
}
