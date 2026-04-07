### The Improved Prompt

**Role & Context:**
Act as an expert Kubernetes (K8s) architect and Go developer specializing in the Operator pattern (using Kubebuilder/Operator SDK). I want to build a custom Kubernetes operator named `snipsnap`. 

**Problem Statement:**
I need an operator inspired by KubeAI, but tailored for heavily constrained edge/homelab environments (e.g., a single RTX 4090). Existing tools like KubeAI assume multi-GPU clusters and high-bandwidth local model registries. `snipsnap` must address these limitations by aggressively managing single-GPU resources and utilizing persistent volumes for local model caching.

**Core Architecture & Requirements:**

1. **OpenAI-Compatible Proxy:**
   * The operator must deploy/manage a lightweight proxy that exposes an OpenAI-compatible API to the cluster.
2. **Aggressive Model Switching (Zero-Concurrency on GPU):**
   * The proxy must handle aggressive model swapping. If a new request asks for Model B while Model A is loaded, the operator/proxy must immediately terminate or scale down Model A and spin up/route to Model B.
   * Cold starts are explicitly acceptable. Do not optimize for concurrent loading. Prioritize fitting the model into the constrained GPU.
   * The system should keep the *last requested* model running indefinitely until a different model is requested.
3. **Persistent Volume (PV) Caching:**
   * The Custom Resource Definition (CRD) for a `Model` must include native support for defining PVs and PVCs.
   * When a model is requested for the first time, it downloads to the PVC. Subsequent loads must mount this PVC to avoid re-downloading gigabytes of data over the internet.
   * The volume mounts should be structured cleverly to support standard inference engine configurations, specifically targeting **Ollama** and **vLLM**.

**Expected Output:**
Please provide the following to help me bootstrap this project:
1. **System Architecture:** A brief overview of how the Proxy, Operator, and Inference Pods (Ollama/vLLM) will interact.
2. **CRD Definition:** The Go struct definitions (or YAML) for the `Model` Custom Resource, specifically showing how the PVC caching and engine selection (Ollama vs. vLLM) are defined.
3. **Reconciliation Logic:** A conceptual breakdown or pseudo-code of the Operator's Reconcile loop. How does it handle the aggressive scale-to-zero for the old model and scale-to-one for the new model?
4. **Proxy Routing Logic:** A conceptual explanation of how the proxy intercepts the OpenAI API call, triggers the operator/deployments to switch models, waits for readiness, and forwards the request.


First step to achieve this is to look into https://github.com/kubeai-project/kubeai kubeai's source code and figure out what we want to "import" or "reuse" from that project, things like the proxy api should be very similar