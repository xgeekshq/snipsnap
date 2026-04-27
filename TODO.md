# snipsnap TODO

Known gaps and missing features identified during initial development.

## High Priority

- [x] **change licencse stuff**

- [x] **local dev lifecycle**: `make dev-deploy` builds + pushes a timestamp-tagged image and `helm upgrade -i --atomic` against `charts/snipsnap/` with `values-dev.yaml` layered on top.

- [ ] **vLLM model download**: Ollama self-downloads via `ollama pull` in the startup probe, but vLLM pods have no mechanism to populate the cache PVC. Needs an init container or cache Job to run `huggingface-cli download` before the inference server starts.

- [ ] **Request queue / concurrency control**: No serialization of requests in the proxy. Concurrent requests for different models race on the Workspace `activeModel` patch. Needs a queue or mutex in `internal/proxy/handler.go` to serialize model switches and drain in-flight requests before swapping. Lets build a statefull queue with sqlite, sotre it a pvc. Should have an in memory queue that persists to sqlite.

- [ ] **Workspace-level resource defaults**: GPU/scheduling constraints are repeated inline on every Model CR (`spec.resources`). The Workspace should define default resource requirements, nodeSelector, tolerations, and affinity that all Models inherit unless overridden.

- [ ] **Purge endpoint**: cancel all requests in the queue and remove all model pods

- [ ] **llama.cpp backend**: Add a third inference engine alongside Ollama and vLLM. Needs an `EngineLlamaCpp` constant in `api/v1`, a builder in `internal/engine/` that produces a llama.cpp server pod (image, args, port, readiness probe) from a `Model` CR, and a `url://` scheme + parsing in the engine factory.

## Medium Priority

- [ ] **In-cluster proxy routing**: The proxy writes raw pod IPs into `Workspace.status.inferenceAddress`, which only works when the operator runs inside the cluster. Running locally via `go run` cannot reach pod IPs. Options: create a Service per inference pod, or add a dev-mode address override flag.

- [ ] **Agent files setup**

- [ ] **Increase proxy test coverage**: `internal/proxy` is at ~22% coverage. Add unit tests for `ensureModel` (cached vs. switch path), `waitForReady` (timeout, polling, ready transition), `proxyRequest` (reverse-proxy wiring, error handler, body replay), and `parseModelFromRequest` edge cases using `httptest` and a fake controller-runtime client.

## Housekeeping

- [x] **Add bin/ to .gitignore**: The `bin/` directory contains dev tools and envtest binaries (~460MB) that should not be committed.

- [x] **Remove debug instrumentation**: Debug logging from the pod IP investigation is gone from `internal/proxy/handler.go` and `internal/controller/workspace_controller.go`.
