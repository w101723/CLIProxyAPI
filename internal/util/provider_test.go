package util

import "testing"

func TestGetProviderNameQwenRerankFallback(t *testing.T) {
	providers := GetProviderName("qwen-plus-rerank")
	if len(providers) != 1 || providers[0] != "qwen-rerank" {
		t.Fatalf("providers = %#v", providers)
	}
}
