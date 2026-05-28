package executor

import (
	"encoding/json"
	"testing"
)

func TestConvertOpenAIRerankRequestToDashScope(t *testing.T) {
	raw := []byte(`{"model":"qwen3-rerank","query":"什么是文本排序模型","documents":["a","b"],"top_n":2,"return_documents":true}`)
	out, err := ConvertOpenAIRerankRequestToDashScope(raw, "")
	if err != nil {
		t.Fatalf("ConvertOpenAIRerankRequestToDashScope() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if got["model"] != "qwen3-rerank" {
		t.Fatalf("model = %v", got["model"])
	}
	input := got["input"].(map[string]any)
	if input["query"] != "什么是文本排序模型" {
		t.Fatalf("query = %v", input["query"])
	}
	docs := input["documents"].([]any)
	if len(docs) != 2 || docs[0] != "a" || docs[1] != "b" {
		t.Fatalf("documents = %#v", docs)
	}
	params := got["parameters"].(map[string]any)
	if params["top_n"].(float64) != 2 || params["return_documents"] != true {
		t.Fatalf("parameters = %#v", params)
	}
}

func TestConvertDashScopeRerankResponseToCompatible(t *testing.T) {
	raw := []byte(`{"output":{"results":[{"document":{"text":"a"},"index":0,"relevance_score":0.9}]},"usage":{"total_tokens":12},"request_id":"req-1"}`)
	out, err := ConvertDashScopeRerankResponseToCompatible(raw, "qwen3-rerank")
	if err != nil {
		t.Fatalf("ConvertDashScopeRerankResponseToCompatible() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if got["model"] != "qwen3-rerank" || got["id"] != "req-1" {
		t.Fatalf("unexpected response = %#v", got)
	}
	if _, exists := got["object"]; exists {
		t.Fatalf("response should not include OpenAI object field: %#v", got)
	}
	if _, exists := got["data"]; exists {
		t.Fatalf("response should not include OpenAI data field: %#v", got)
	}
	results := got["results"].([]any)
	row := results[0].(map[string]any)
	if row["index"].(float64) != 0 || row["relevance_score"].(float64) != 0.9 {
		t.Fatalf("unexpected row = %#v", row)
	}
	meta := got["meta"].(map[string]any)
	tokens := meta["tokens"].(map[string]any)
	if tokens["input_tokens"].(float64) != 12 || tokens["total_tokens"].(float64) != 12 {
		t.Fatalf("unexpected tokens = %#v", tokens)
	}
}
