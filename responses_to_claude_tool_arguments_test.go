package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestResponsesToClaudePreservesToolArgumentsFromDoneEvent(t *testing.T) {
	stream := `event: response.created
data: {"type":"response.created","response":{"id":"resp_spark","model":"gpt-5.3-codex-spark"}}

event: response.output_item.added
data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"write","arguments":""}}

event: response.function_call_arguments.done
data: {"type":"response.function_call_arguments.done","item_id":"fc_1","output_index":0,"arguments":"{\"path\":\"spark-proof.txt\",\"content\":\"SPARK_TOOL_OK\\n\"}"}

event: response.output_item.done
data: {"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"write","arguments":"{\"path\":\"spark-proof.txt\",\"content\":\"SPARK_TOOL_OK\\n\"}"}}

event: response.completed
data: {"type":"response.completed","response":{"id":"resp_spark","model":"gpt-5.3-codex-spark","status":"completed","usage":{"input_tokens":10,"output_tokens":8}}}

`
	wire := translateResponsesToolStream(t, stream)
	arguments := `partial_json":"{\"path\":\"spark-proof.txt\",\"content\":\"SPARK_TOOL_OK\\n\"}`
	if strings.Count(wire, arguments) != 1 {
		t.Fatalf("complete arguments count=%d, wire:\n%s", strings.Count(wire, arguments), wire)
	}
	if !strings.Contains(wire, `"index":0,"delta":{"type":"input_json_delta"`) {
		t.Fatalf("tool argument block index was not preserved:\n%s", wire)
	}
}

func TestResponsesToClaudeDoesNotDuplicateToolArgumentsAfterDeltas(t *testing.T) {
	stream := `event: response.created
data: {"type":"response.created","response":{"id":"resp_delta","model":"gpt-5.3-codex-spark"}}

event: response.output_item.added
data: {"type":"response.output_item.added","item":{"type":"function_call","id":"fc_2","call_id":"call_2","name":"write"}}

event: response.function_call_arguments.delta
data: {"type":"response.function_call_arguments.delta","item_id":"fc_2","delta":"{\"path\":\"a.txt\","}

event: response.function_call_arguments.delta
data: {"type":"response.function_call_arguments.delta","item_id":"fc_2","delta":"\"content\":\"ok\"}"}

event: response.function_call_arguments.done
data: {"type":"response.function_call_arguments.done","item_id":"fc_2","arguments":"{\"path\":\"a.txt\",\"content\":\"ok\"}"}

event: response.output_item.done
data: {"type":"response.output_item.done","item":{"type":"function_call","id":"fc_2","call_id":"call_2","name":"write","arguments":"{\"path\":\"a.txt\",\"content\":\"ok\"}"}}

event: response.completed
data: {"type":"response.completed","response":{"id":"resp_delta","status":"completed","usage":{"input_tokens":10,"output_tokens":8}}}

`
	wire := translateResponsesToolStream(t, stream)
	if strings.Count(wire, `input_json_delta`) != 2 {
		t.Fatalf("argument deltas duplicated or lost:\n%s", wire)
	}
}

func translateResponsesToolStream(t *testing.T, stream string) string {
	t.Helper()
	var output bytes.Buffer
	writer := &responsesToClaudeWriter{w: &output}
	if _, err := writer.Write([]byte(stream)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Finalize(); err != nil {
		t.Fatal(err)
	}
	return output.String()
}
