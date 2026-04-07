package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// modelRequest holds the parsed model name and the buffered body.
type modelRequest struct {
	ModelName string
	Body      []byte
}

// parseModelFromRequest extracts the model name from an OpenAI-style JSON body.
// It buffers the body so it can be re-read later during proxying.
func parseModelFromRequest(r *http.Request) (*modelRequest, error) {
	if r.Body == nil {
		return nil, fmt.Errorf("empty request body")
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("reading request body: %w", err)
	}
	r.Body.Close()

	r.Body = io.NopCloser(bytes.NewReader(body))

	var payload struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parsing JSON body: %w", err)
	}

	model := strings.TrimSpace(payload.Model)
	if model == "" {
		return nil, fmt.Errorf("missing or empty 'model' field in request body")
	}

	return &modelRequest{
		ModelName: model,
		Body:      body,
	}, nil
}

// isProxiedPath returns true if the path should be reverse-proxied to the inference backend.
func isProxiedPath(path string) bool {
	proxied := []string{
		"/v1/chat/completions",
		"/v1/completions",
		"/v1/embeddings",
	}
	for _, p := range proxied {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}
