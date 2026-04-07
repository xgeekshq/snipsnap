package engine

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	snipsnapv1 "github.com/xgeekshq/snipsnap/api/v1"
)

func newTestModel(name, engine, url string) *snipsnapv1.Model {
	return &snipsnapv1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: snipsnapv1.ModelSpec{
			URL:    url,
			Engine: engine,
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					"nvidia.com/gpu": resource.MustParse("1"),
				},
			},
		},
	}
}

func TestVLLMPodForModel(t *testing.T) {
	m := newTestModel("mistral-7b", snipsnapv1.VLLMEngine, "mistralai/Mistral-7B-v0.1")
	pod, err := PodForModel(m, "default-ws")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pod.Spec.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(pod.Spec.Containers))
	}
	c := pod.Spec.Containers[0]
	if c.Name != "server" {
		t.Errorf("expected container name 'server', got %q", c.Name)
	}
	if c.Image != defaultVLLMImage {
		t.Errorf("expected default vLLM image, got %q", c.Image)
	}
	if c.Command[0] != "python3" {
		t.Errorf("expected python3 command, got %q", c.Command[0])
	}

	foundModelArg := false
	for _, arg := range c.Args {
		if arg == "--model=mistralai/Mistral-7B-v0.1" {
			foundModelArg = true
		}
	}
	if !foundModelArg {
		t.Errorf("expected --model arg, got args: %v", c.Args)
	}

	if pod.Labels[snipsnapv1.LabelEngine] != snipsnapv1.VLLMEngine {
		t.Errorf("expected engine label %q, got %q", snipsnapv1.VLLMEngine, pod.Labels[snipsnapv1.LabelEngine])
	}
}

func TestVLLMPodWithCache(t *testing.T) {
	m := newTestModel("mistral-7b", snipsnapv1.VLLMEngine, "mistralai/Mistral-7B-v0.1")
	m.Spec.Cache.Enabled = true
	m.Spec.Cache.StorageSize = "100Gi"

	pod, err := PodForModel(m, "ws")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundCacheVol := false
	for _, v := range pod.Spec.Volumes {
		if v.Name == "model-cache" {
			foundCacheVol = true
			if v.PersistentVolumeClaim.ClaimName != "model-cache-mistral-7b" {
				t.Errorf("expected PVC name model-cache-mistral-7b, got %q", v.PersistentVolumeClaim.ClaimName)
			}
		}
	}
	if !foundCacheVol {
		t.Error("expected model-cache volume")
	}

	foundModelFlag := false
	for _, arg := range pod.Spec.Containers[0].Args {
		if arg == "--model=/models" {
			foundModelFlag = true
		}
	}
	if !foundModelFlag {
		t.Errorf("expected --model=/models for cached model, got args: %v", pod.Spec.Containers[0].Args)
	}
}

func TestOllamaPodForModel(t *testing.T) {
	m := newTestModel("llama3", snipsnapv1.OLlamaEngine, "ollama://llama3")
	pod, err := PodForModel(m, "default-ws")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pod.Spec.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(pod.Spec.Containers))
	}
	c := pod.Spec.Containers[0]
	if c.Name != "server" {
		t.Errorf("expected container name 'server', got %q", c.Name)
	}
	if c.Image != defaultOllamaImage {
		t.Errorf("expected default Ollama image, got %q", c.Image)
	}

	foundHost := false
	for _, e := range c.Env {
		if e.Name == "OLLAMA_HOST" && e.Value == "0.0.0.0:8000" {
			foundHost = true
		}
	}
	if !foundHost {
		t.Error("expected OLLAMA_HOST env var")
	}

	if c.StartupProbe == nil || c.StartupProbe.Exec == nil {
		t.Fatal("expected exec startup probe")
	}

	if pod.Labels[snipsnapv1.LabelEngine] != snipsnapv1.OLlamaEngine {
		t.Errorf("expected engine label %q, got %q", snipsnapv1.OLlamaEngine, pod.Labels[snipsnapv1.LabelEngine])
	}
}

func TestOllamaPodWithCache(t *testing.T) {
	m := newTestModel("llama3", snipsnapv1.OLlamaEngine, "ollama://llama3")
	m.Spec.Cache.Enabled = true

	pod, err := PodForModel(m, "ws")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundModelsEnv := false
	for _, e := range pod.Spec.Containers[0].Env {
		if e.Name == "OLLAMA_MODELS" && e.Value == "/models" {
			foundModelsEnv = true
		}
	}
	if !foundModelsEnv {
		t.Error("expected OLLAMA_MODELS=/models env var when cache enabled")
	}
}

func TestCustomImage(t *testing.T) {
	m := newTestModel("custom", snipsnapv1.VLLMEngine, "my-model")
	m.Spec.Image = "my-registry/vllm:custom"

	pod, err := PodForModel(m, "ws")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pod.Spec.Containers[0].Image != "my-registry/vllm:custom" {
		t.Errorf("expected custom image, got %q", pod.Spec.Containers[0].Image)
	}
}

func TestUnsupportedEngine(t *testing.T) {
	m := newTestModel("bad", "BadEngine", "some-url")
	_, err := PodForModel(m, "ws")
	if err == nil {
		t.Fatal("expected error for unsupported engine")
	}
}

func TestExistingPVCName(t *testing.T) {
	m := newTestModel("llama3", snipsnapv1.OLlamaEngine, "ollama://llama3")
	m.Spec.Cache.Enabled = true
	m.Spec.Cache.ExistingPVCName = "my-existing-pvc"

	pod, err := PodForModel(m, "ws")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, v := range pod.Spec.Volumes {
		if v.Name == "model-cache" {
			if v.PersistentVolumeClaim.ClaimName != "my-existing-pvc" {
				t.Errorf("expected existing PVC name, got %q", v.PersistentVolumeClaim.ClaimName)
			}
			return
		}
	}
	t.Error("expected model-cache volume")
}
