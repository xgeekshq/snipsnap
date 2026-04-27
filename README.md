# snipsnap

Aggressive LLM switching operator for resource-starved Kubernetes clusters.

snipsnap manages a single GPU, loading one model at a time. When a different model is requested via the OpenAI-compatible API, the current model is immediately terminated and the new one is spun up. Persistent volume caching avoids re-downloading model weights.

## Architecture

```
Client (OpenAI SDK)
       |
       v
 snipsnap Proxy (:8000)     <-- OpenAI-compatible API
       |
       v
 Workspace Controller       <-- Detects model mismatch, kills old pod, creates new
       |
       v
 Inference Pod (Ollama/vLLM) <-- Mounts cache PVC, claims GPU
       |
       v
   GPU (RTX 4090)
```

## Quickstart

### Prerequisites

- Kubernetes cluster with GPU support (NVIDIA device plugin)
- `kubectl` configured
- Helm 3+

### Deploy with Helm

CRDs ship inside the chart at `charts/snipsnap/crds/`, so Helm installs them on the first apply:

```bash
helm install snipsnap charts/snipsnap --namespace snipsnap --create-namespace
```

For a local dev cluster, use the `values-dev.yaml` overrides (always-pull image, metrics on, sample model pre-seeded) via the convenience target:

```bash
make dev-deploy
```

### Register models

The chart templates `Model` CRs from the `models:` array in your values file. Add entries inline:

```yaml
models:
  - name: llama3
    url: "ollama://llama3"
    engine: OLlama
    cache:
      enabled: true
      storageSize: "20Gi"
    resources:
      limits:
        nvidia.com/gpu: "1"
```

Then `helm upgrade snipsnap charts/snipsnap -f your-values.yaml` to apply.

### Use the API

```bash
# The proxy auto-switches models. First request to llama3 will load it:
curl http://snipsnap-api.snipsnap:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "llama3", "messages": [{"role": "user", "content": "Hello!"}]}'

# Requesting mistral-7b will kill llama3 and load mistral:
curl http://snipsnap-api.snipsnap:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "mistral-7b", "messages": [{"role": "user", "content": "Hello!"}]}'

# List available models:
curl http://snipsnap-api.snipsnap:8000/v1/models
```

## Custom Resources

### Model

Defines an LLM that can be loaded. Supports Ollama and vLLM engines.

```yaml
apiVersion: snipsnap.xgeeks.com/v1
kind: Model
metadata:
  name: llama3
spec:
  url: "ollama://llama3"
  engine: OLlama
  cache:
    enabled: true
    storageSize: "20Gi"
  resources:
    limits:
      nvidia.com/gpu: "1"
```

### Workspace

Tracks which model is currently active on the GPU.

```yaml
apiVersion: snipsnap.xgeeks.com/v1
kind: Workspace
metadata:
  name: default
spec:
  activeModel: "llama3"
```

## Development

```bash
# Generate CRDs and code
make generate manifests

# Run locally against a cluster
make run

# Run tests
make test

# Build container image
make docker-build IMG=snipsnap:dev
```

## License

Apache License 2.0
