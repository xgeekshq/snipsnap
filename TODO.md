# snipsnap TODO

Known gaps and missing features identified during initial development.

## High Priority

- [ ] **vLLM model download**: Ollama self-downloads via `ollama pull` in the startup probe, but vLLM pods have no mechanism to populate the cache PVC. Needs an init container or cache Job to run `huggingface-cli download` before the inference server starts.

- [ ] **Request queue / concurrency control**: No serialization of requests in the proxy. Concurrent requests for different models race on the Workspace `activeModel` patch. Needs a queue or mutex in `internal/proxy/handler.go` to serialize model switches and drain in-flight requests before swapping.

- [ ] **Workspace-level resource defaults**: GPU/scheduling constraints are repeated inline on every Model CR (`spec.resources`). The Workspace should define default resource requirements, nodeSelector, tolerations, and affinity that all Models inherit unless overridden.

- [ ] **Purge endpoint**: cancel all requests in the queue and remove all model pods

## Medium Priority

- [ ] **In-cluster proxy routing**: The proxy writes raw pod IPs into `Workspace.status.inferenceAddress`, which only works when the operator runs inside the cluster. Running locally via `go run` cannot reach pod IPs. Options: create a Service per inference pod, or add a dev-mode address override flag.

- [ ] **Kustomize manager.yaml out of sync**: The scaffolded `config/manager/manager.yaml` passes `--leader-elect` which was removed from `cmd/main.go`. Needs updating with snipsnap-specific args (`--namespace`, `--workspace-name`, `--api-bind-address`, port exposure).

## Housekeeping

- [x] **Add bin/ to .gitignore**: The `bin/` directory contains dev tools and envtest binaries (~460MB) that should not be committed.

- [ ] **Remove debug instrumentation**: Debug logging from the pod IP investigation is still present in `internal/proxy/handler.go` and `internal/controller/workspace_controller.go`. Remove before merging.
