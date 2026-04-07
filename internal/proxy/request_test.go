package proxy

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

func TestParseModelFromRequest(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantModel string
		wantErr   bool
	}{
		{
			name:      "chat completion",
			body:      `{"model": "llama3", "messages": [{"role": "user", "content": "hi"}]}`,
			wantModel: "llama3",
		},
		{
			name:      "completion",
			body:      `{"model": "mistral-7b", "prompt": "hello"}`,
			wantModel: "mistral-7b",
		},
		{
			name:    "missing model",
			body:    `{"messages": [{"role": "user", "content": "hi"}]}`,
			wantErr: true,
		},
		{
			name:    "empty model",
			body:    `{"model": "", "messages": []}`,
			wantErr: true,
		},
		{
			name:    "invalid json",
			body:    `not json`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(tt.body))
			mr, err := parseModelFromRequest(r)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mr.ModelName != tt.wantModel {
				t.Errorf("expected model %q, got %q", tt.wantModel, mr.ModelName)
			}

			// Verify body was preserved for re-reading
			restored, _ := io.ReadAll(r.Body)
			if string(restored) != tt.body {
				t.Errorf("body not preserved, got %q", string(restored))
			}
		})
	}
}

func TestIsProxiedPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/v1/chat/completions", true},
		{"/v1/completions", true},
		{"/v1/embeddings", true},
		{"/v1/models", false},
		{"/healthz", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isProxiedPath(tt.path); got != tt.want {
				t.Errorf("isProxiedPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
