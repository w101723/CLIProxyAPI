package auth

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
)

func TestQwenRerankAuthSupportsModelFamily(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	auth := &Auth{ID: "qwen-rerank-test-auth", Provider: "qwen-rerank"}
	reg := registry.GetGlobalRegistry()
	reg.UnregisterClient(auth.ID)
	t.Cleanup(func() { reg.UnregisterClient(auth.ID) })
	reg.RegisterClient(auth.ID, "qwen-rerank", []*registry.ModelInfo{{ID: "qwen3-rerank", Object: "model"}})

	if !manager.authSupportsRouteModel(reg, auth, "qwen-plus-rerank") {
		t.Fatal("expected qwen-rerank auth to support qwen rerank model family")
	}
	if manager.authSupportsRouteModel(reg, auth, "qwen-plus") {
		t.Fatal("expected qwen-rerank auth to reject non-rerank qwen model")
	}
}
