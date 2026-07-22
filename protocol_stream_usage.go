package main

import "encoding/json"

// protocolUsageObserver turns complete SSE data payloads into normalized usage
// records. It owns JSON envelope decoding and split Anthropic event assembly;
// callers own transport policy and durable recording.
type protocolUsageObserver struct {
	parse        func(map[string]any) *RequestUsage
	emit         func(*RequestUsage, bool)
	defaultModel string
	accumulator  splitUsageAccumulator
}

func newProtocolUsageObserver(provider Provider, defaultModel string, emit func(*RequestUsage, bool)) *protocolUsageObserver {
	observer := &protocolUsageObserver{defaultModel: defaultModel, emit: emit}
	if provider != nil {
		observer.parse = provider.ParseUsage
	}
	return observer
}

func (observer *protocolUsageObserver) Observe(data []byte) error {
	if observer == nil || observer.parse == nil {
		return nil
	}
	object, err := decodeProtocolUsagePayload(data)
	if err != nil {
		return err
	}
	usage := observer.parse(object)
	if usage == nil {
		return nil
	}
	eventType, _ := object["type"].(string)
	usage = observer.accumulator.add(eventType, usage)
	if usage != nil {
		observer.emitUsage(usage, false)
	}
	return nil
}

func (observer *protocolUsageObserver) Flush() {
	if observer == nil {
		return
	}
	if usage := observer.accumulator.flush(); usage != nil {
		observer.emitUsage(usage, true)
	}
}

func (observer *protocolUsageObserver) emitUsage(usage *RequestUsage, pending bool) {
	if usage.Model == "" {
		usage.Model = observer.defaultModel
	}
	if observer.emit != nil {
		observer.emit(usage, pending)
	}
}

func decodeProtocolUsagePayload(data []byte) (map[string]any, error) {
	var object map[string]any
	if err := json.Unmarshal(data, &object); err == nil {
		return object, nil
	} else {
		var objects []map[string]any
		if arrayErr := json.Unmarshal(data, &objects); arrayErr != nil {
			return nil, err
		}
		if len(objects) == 0 {
			return nil, err
		}
		return objects[0], nil
	}
}
