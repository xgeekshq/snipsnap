package openaiserver

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	snipsnapv1 "github.com/xgeekshq/snipsnap/api/v1"
	"github.com/xgeekshq/snipsnap/internal/proxy"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Handler serves the OpenAI-compatible API.
type Handler struct {
	K8sClient     client.Client
	Namespace     string
	WorkspaceName string
	http.Handler
}

// NewHandler creates a new OpenAI server mux.
func NewHandler(k8sClient client.Client, namespace, workspaceName string) *Handler {
	h := &Handler{
		K8sClient:     k8sClient,
		Namespace:     namespace,
		WorkspaceName: workspaceName,
	}

	proxyHandler := &proxy.Handler{
		K8sClient:     k8sClient,
		WorkspaceName: workspaceName,
		Namespace:     namespace,
		MaxRetries:    3,
	}

	mux := http.NewServeMux()
	mux.Handle("/v1/chat/completions", proxyHandler)
	mux.Handle("/v1/completions", proxyHandler)
	mux.Handle("/v1/embeddings", proxyHandler)
	mux.HandleFunc("/v1/models", h.getModels)
	mux.HandleFunc("/healthz", h.healthz)
	mux.HandleFunc("/readyz", h.readyz)

	h.Handler = mux
	return h
}

func (h *Handler) getModels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	models := &snipsnapv1.ModelList{}
	if err := h.K8sClient.List(r.Context(), models, client.InNamespace(h.Namespace)); err != nil {
		sendError(w, http.StatusInternalServerError, "failed to list models: %v", err)
		return
	}

	type openAIModel struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
		Engine  string `json:"engine,omitempty"`
	}

	data := make([]openAIModel, 0, len(models.Items))
	for _, m := range models.Items {
		data = append(data, openAIModel{
			ID:      m.Name,
			Object:  "model",
			Created: m.CreationTimestamp.Unix(),
			OwnedBy: "snipsnap",
			Engine:  m.Spec.Engine,
		})
	}

	resp := struct {
		Object string        `json:"object"`
		Data   []openAIModel `json:"data"`
	}{
		Object: "list",
		Data:   data,
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("openaiserver: error encoding response: %v", err)
	}
}

func (h *Handler) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "ok")
}

func (h *Handler) readyz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "ok")
}

func sendError(w http.ResponseWriter, status int, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Printf("openaiserver: error %d: %s", status, msg)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{Error: msg})
}
