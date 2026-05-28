package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/runtime/executor/helps"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/thinking"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/util"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

const defaultQwenRerankEndpoint = "https://dashscope.aliyuncs.com/api/v1/services/rerank/text-rerank/text-rerank"

type QwenRerankExecutor struct {
	cfg *config.Config
}

func NewQwenRerankExecutor(cfg *config.Config) *QwenRerankExecutor {
	return &QwenRerankExecutor{cfg: cfg}
}

func (e *QwenRerankExecutor) Identifier() string { return "qwen-rerank" }

func (e *QwenRerankExecutor) PrepareRequest(req *http.Request, auth *cliproxyauth.Auth) error {
	if req == nil {
		return nil
	}
	if key := strings.TrimSpace(authAttribute(auth, "api_key")); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	util.ApplyCustomHeadersFromAttrs(req, authAttributes(auth))
	return nil
}

func (e *QwenRerankExecutor) HttpRequest(ctx context.Context, auth *cliproxyauth.Auth, req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("qwen rerank executor: request is nil")
	}
	if ctx == nil {
		ctx = req.Context()
	}
	httpReq := req.WithContext(ctx)
	if err := e.PrepareRequest(httpReq, auth); err != nil {
		return nil, err
	}
	httpClient := helps.NewProxyAwareHTTPClient(ctx, e.cfg, auth, 0)
	return httpClient.Do(httpReq)
}

func (e *QwenRerankExecutor) Execute(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (resp cliproxyexecutor.Response, err error) {
	if opts.Alt != "rerank" {
		return resp, statusErr{code: http.StatusNotImplemented, msg: "qwen-rerank only supports /v1/rerank"}
	}
	baseModel := thinking.ParseSuffix(req.Model).ModelName
	reporter := helps.NewUsageReporter(ctx, e.Identifier(), baseModel, auth)
	defer reporter.TrackFailure(ctx, &err)

	body, err := ConvertOpenAIRerankRequestToDashScope(req.Payload, baseModel)
	if err != nil {
		return resp, statusErr{code: http.StatusBadRequest, msg: err.Error()}
	}

	url := resolveQwenRerankEndpoint(auth)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return resp, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", codexUserAgent)
	if err := e.PrepareRequest(httpReq, auth); err != nil {
		return resp, err
	}

	var authID, authLabel, authType, authValue string
	if auth != nil {
		authID = auth.ID
		authLabel = auth.Label
		authType, authValue = auth.AccountInfo()
	}
	helps.RecordAPIRequest(ctx, e.cfg, helps.UpstreamRequestLog{
		URL:       url,
		Method:    http.MethodPost,
		Headers:   httpReq.Header.Clone(),
		Body:      body,
		Provider:  e.Identifier(),
		AuthID:    authID,
		AuthLabel: authLabel,
		AuthType:  authType,
		AuthValue: authValue,
	})

	httpClient := helps.NewProxyAwareHTTPClient(ctx, e.cfg, auth, 0)
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		helps.RecordAPIResponseError(ctx, e.cfg, err)
		return resp, err
	}
	defer func() {
		if errClose := httpResp.Body.Close(); errClose != nil {
			log.Errorf("qwen rerank executor: close response body error: %v", errClose)
		}
	}()
	helps.RecordAPIResponseMetadata(ctx, e.cfg, httpResp.StatusCode, httpResp.Header.Clone())
	data, err := io.ReadAll(httpResp.Body)
	if err != nil {
		helps.RecordAPIResponseError(ctx, e.cfg, err)
		return resp, err
	}
	helps.AppendAPIResponseChunk(ctx, e.cfg, data)
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		err = statusErr{code: httpResp.StatusCode, msg: string(data)}
		return resp, err
	}

	out, err := ConvertDashScopeRerankResponseToCompatible(data, baseModel)
	if err != nil {
		return resp, statusErr{code: http.StatusBadGateway, msg: err.Error()}
	}
	reporter.Publish(ctx, parseQwenRerankUsage(data))
	reporter.EnsurePublished(ctx)
	return cliproxyexecutor.Response{Payload: out, Headers: httpResp.Header.Clone()}, nil
}

func (e *QwenRerankExecutor) ExecuteStream(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
	return nil, statusErr{code: http.StatusNotImplemented, msg: "qwen-rerank streaming is not supported"}
}

func (e *QwenRerankExecutor) Refresh(ctx context.Context, auth *cliproxyauth.Auth) (*cliproxyauth.Auth, error) {
	return auth, nil
}

func (e *QwenRerankExecutor) CountTokens(ctx context.Context, auth *cliproxyauth.Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{}, statusErr{code: http.StatusNotImplemented, msg: "qwen-rerank token counting is not supported"}
}

func resolveQwenRerankEndpoint(auth *cliproxyauth.Auth) string {
	base := strings.TrimSpace(authAttribute(auth, "base_url"))
	if base == "" {
		return defaultQwenRerankEndpoint
	}
	if strings.Contains(base, "/api/v1/services/rerank/") {
		return base
	}
	return strings.TrimRight(base, "/") + "/api/v1/services/rerank/text-rerank/text-rerank"
}

func authAttribute(auth *cliproxyauth.Auth, key string) string {
	if auth == nil || auth.Attributes == nil {
		return ""
	}
	return auth.Attributes[key]
}

func authAttributes(auth *cliproxyauth.Auth) map[string]string {
	if auth == nil {
		return nil
	}
	return auth.Attributes
}

func ConvertOpenAIRerankRequestToDashScope(raw []byte, model string) ([]byte, error) {
	root := gjson.ParseBytes(raw)
	query := strings.TrimSpace(root.Get("query").String())
	if query == "" {
		return nil, fmt.Errorf("missing required field: query")
	}
	documents := root.Get("documents")
	if !documents.IsArray() {
		return nil, fmt.Errorf("missing required field: documents")
	}
	docs := make([]string, 0)
	documents.ForEach(func(_, value gjson.Result) bool {
		docs = append(docs, value.String())
		return true
	})
	if len(docs) == 0 {
		return nil, fmt.Errorf("documents must not be empty")
	}
	if m := strings.TrimSpace(root.Get("model").String()); m != "" {
		model = m
	}
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("missing required field: model")
	}

	payload := map[string]any{
		"model": model,
		"input": map[string]any{
			"query":     query,
			"documents": docs,
		},
	}
	params := make(map[string]any)
	if topN := root.Get("top_n"); topN.Exists() {
		params["top_n"] = topN.Int()
	}
	if returnDocuments := root.Get("return_documents"); returnDocuments.Exists() {
		params["return_documents"] = returnDocuments.Bool()
	}
	if parameters := root.Get("parameters"); parameters.IsObject() {
		parameters.ForEach(func(key, value gjson.Result) bool {
			params[key.String()] = value.Value()
			return true
		})
	}
	if len(params) > 0 {
		payload["parameters"] = params
	}
	return json.Marshal(payload)
}

func ConvertDashScopeRerankResponseToCompatible(raw []byte, model string) ([]byte, error) {
	root := gjson.ParseBytes(raw)
	results := root.Get("output.results")
	if !results.IsArray() {
		return nil, fmt.Errorf("invalid qwen rerank response: missing output.results")
	}
	outResults := make([]map[string]any, 0)
	results.ForEach(func(_, item gjson.Result) bool {
		row := map[string]any{
			"index":           item.Get("index").Int(),
			"relevance_score": item.Get("relevance_score").Float(),
		}
		if doc := item.Get("document"); doc.Exists() {
			row["document"] = doc.Value()
		}
		outResults = append(outResults, row)
		return true
	})
	out := map[string]any{
		"model":   model,
		"results": outResults,
	}
	if requestID := root.Get("request_id"); requestID.Exists() {
		out["id"] = requestID.String()
	}
	if totalTokens := root.Get("usage.total_tokens"); totalTokens.Exists() {
		out["meta"] = map[string]any{
			"tokens": map[string]any{
				"input_tokens": totalTokens.Int(),
				"total_tokens": totalTokens.Int(),
			},
		}
	}
	return json.Marshal(out)
}

func parseQwenRerankUsage(raw []byte) usage.Detail {
	total := gjson.GetBytes(raw, "usage.total_tokens").Int()
	return usage.Detail{InputTokens: total, TotalTokens: total}
}
