package engine

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	snipsnapv1 "github.com/xgeekshq/snipsnap/api/v1"
)

// PodForModel builds the inference pod spec for a given Model CR.
func PodForModel(model *snipsnapv1.Model, workspaceName string) (*corev1.Pod, error) {
	switch model.Spec.Engine {
	case snipsnapv1.OLlamaEngine:
		return ollamaPodForModel(model, workspaceName), nil
	case snipsnapv1.VLLMEngine:
		return vllmPodForModel(model, workspaceName), nil
	default:
		return nil, fmt.Errorf("unsupported engine %q", model.Spec.Engine)
	}
}

func labelsForPod(model *snipsnapv1.Model, workspaceName string) map[string]string {
	return map[string]string{
		snipsnapv1.LabelManagedBy: snipsnapv1.ManagedByValue,
		snipsnapv1.LabelModel:     model.Name,
		snipsnapv1.LabelWorkspace: workspaceName,
		snipsnapv1.LabelEngine:    model.Spec.Engine,
	}
}

func annotationsForPod(port string) map[string]string {
	return map[string]string{
		snipsnapv1.ModelPodPortAnnotation: port,
	}
}

func sortedEnvVars(env map[string]string) []corev1.EnvVar {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	// Deterministic ordering
	sorted := make([]string, len(keys))
	copy(sorted, keys)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	vars := make([]corev1.EnvVar, 0, len(sorted))
	for _, k := range sorted {
		vars = append(vars, corev1.EnvVar{Name: k, Value: env[k]})
	}
	return vars
}

// patchCacheVolume adds the model cache PVC mount to the pod if caching is enabled.
func patchCacheVolume(podSpec *corev1.PodSpec, model *snipsnapv1.Model, mountPath string) {
	if !model.Spec.Cache.Enabled {
		return
	}
	pvcName := CachePVCName(model)
	if model.Spec.Cache.ExistingPVCName != "" {
		pvcName = model.Spec.Cache.ExistingPVCName
	}
	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name: "model-cache",
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: pvcName,
			},
		},
	})
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == "server" {
			podSpec.Containers[i].VolumeMounts = append(podSpec.Containers[i].VolumeMounts, corev1.VolumeMount{
				Name:      "model-cache",
				MountPath: mountPath,
			})
		}
	}
}

// CachePVCName returns the expected PVC name for a model's cache.
func CachePVCName(model *snipsnapv1.Model) string {
	return fmt.Sprintf("model-cache-%s", model.Name)
}
