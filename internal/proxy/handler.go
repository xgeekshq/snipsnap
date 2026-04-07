package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"os"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	snipsnapv1 "github.com/xgeekshq/snipsnap/api/v1"
)

const (
	defaultSwitchTimeout = 10 * time.Minute
	pollInterval         = 500 * time.Millisecond
	debugLogPath         = "/home/eugenio/work/snipsnap/snipsnap/.cursor/debug-e00151.log"
)

// #region agent log
func debugLogProxy(location, message string, data map[string]interface{}, hypothesisID string) {
	entry := fmt.Sprintf(`{"sessionId":"e00151","location":%q,"message":%q,"data":%s,"hypothesisId":%q,"timestamp":%d}`, location, message, mustJSON(data), hypothesisID, time.Now().UnixMilli())
	if f, err := os.OpenFile(debugLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil { f.WriteString(entry + "\n"); f.Close() }
}
func mustJSON(v interface{}) string { b, _ := json.Marshal(v); return string(b) }

// #endregion

// Handler is the OpenAI-compatible reverse proxy that triggers model switches.
type Handler struct {
	K8sClient     client.Client
	WorkspaceName string
	Namespace     string
	MaxRetries    int
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mr, err := parseModelFromRequest(r)
	if err != nil {
		sendError(w, http.StatusBadRequest, "failed to parse request: %v", err)
		return
	}

	log.Printf("proxy: model=%q path=%q", mr.ModelName, r.URL.Path)

	ctx := r.Context()

	addr, err := h.ensureModel(ctx, mr.ModelName)
	if err != nil {
		sendError(w, http.StatusServiceUnavailable, "model switch failed: %v", err)
		return
	}

	h.proxyRequest(w, r, addr, mr.Body)
}

// ensureModel checks the Workspace status and triggers a model switch if needed.
// It blocks until the model is ready or the context times out.
func (h *Handler) ensureModel(ctx context.Context, modelName string) (string, error) {
	ws := &snipsnapv1.Workspace{}
	wsKey := types.NamespacedName{Name: h.WorkspaceName, Namespace: h.Namespace}

	if err := h.K8sClient.Get(ctx, wsKey, ws); err != nil {
		return "", fmt.Errorf("getting workspace: %w", err)
	}

	if ws.Status.LoadedModel == modelName && ws.Status.Phase == snipsnapv1.WorkspacePhaseReady {
		// #region agent log
		debugLogProxy("handler.go:ensureModel:cached", "model already loaded, returning cached addr", map[string]interface{}{"addr": ws.Status.InferenceAddress, "model": modelName, "phase": string(ws.Status.Phase)}, "H2,H5")
		// #endregion
		return ws.Status.InferenceAddress, nil
	}

	// Patch the workspace to request the new model.
	patch := client.MergeFrom(ws.DeepCopy())
	ws.Spec.ActiveModel = modelName
	if err := h.K8sClient.Patch(ctx, ws, patch); err != nil {
		return "", fmt.Errorf("patching workspace active model: %w", err)
	}

	log.Printf("proxy: triggered model switch to %q, waiting for ready...", modelName)

	return h.waitForReady(ctx, wsKey, modelName)
}

func (h *Handler) waitForReady(ctx context.Context, wsKey types.NamespacedName, modelName string) (string, error) {
	timeout, cancel := context.WithTimeout(ctx, defaultSwitchTimeout)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeout.Done():
			return "", fmt.Errorf("timed out waiting for model %q to become ready", modelName)
		case <-ticker.C:
			ws := &snipsnapv1.Workspace{}
			if err := h.K8sClient.Get(timeout, wsKey, ws); err != nil {
				log.Printf("proxy: error polling workspace: %v", err)
				continue
			}
			// #region agent log
			debugLogProxy("handler.go:waitForReady:poll", "polling workspace status", map[string]interface{}{"loadedModel": ws.Status.LoadedModel, "phase": string(ws.Status.Phase), "addr": ws.Status.InferenceAddress, "wantModel": modelName}, "H1,H2,H5")
			// #endregion
			if ws.Status.LoadedModel == modelName && ws.Status.Phase == snipsnapv1.WorkspacePhaseReady && ws.Status.InferenceAddress != "" {
				log.Printf("proxy: model %q ready at %s", modelName, ws.Status.InferenceAddress)
				return ws.Status.InferenceAddress, nil
			}
		}
	}
}

func (h *Handler) proxyRequest(w http.ResponseWriter, r *http.Request, addr string, body []byte) {
	// #region agent log
	debugLogProxy("handler.go:proxyRequest", "proxying to backend", map[string]interface{}{"addr": addr, "path": r.URL.Path, "method": r.Method, "bodyLen": len(body)}, "H3,H4,H5")
	// #endregion
	target := &url.URL{
		Scheme: "http",
		Host:   addr,
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = r.Host
		},
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		if resp.StatusCode >= 500 {
			log.Printf("proxy: backend returned %d", resp.StatusCode)
		}
		return nil
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("proxy: reverse proxy error: %v", err)
		sendError(w, http.StatusBadGateway, "proxy error: %v", err)
	}

	clone := r.Clone(r.Context())
	clone.Body = io.NopCloser(bytes.NewReader(body))
	clone.ContentLength = int64(len(body))

	proxy.ServeHTTP(w, clone)
}

func sendError(w http.ResponseWriter, status int, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Printf("proxy: error %d: %s", status, msg)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{
		Error: msg,
	})
}
