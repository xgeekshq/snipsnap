package engine

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	snipsnapv1 "github.com/xgeekshq/snipsnap/api/v1"
)

const (
	defaultOllamaImage = "ollama/ollama:0.20.0"
	ollamaPort         = 8000
)

func ollamaPodForModel(m *snipsnapv1.Model, workspaceName string) *corev1.Pod {
	image := m.Spec.Image
	if image == "" {
		image = defaultOllamaImage
	}

	modelRef := ollamaModelRef(m.Spec.URL)

	env := []corev1.EnvVar{
		{Name: "OLLAMA_HOST", Value: fmt.Sprintf("0.0.0.0:%d", ollamaPort)},
		{Name: "OLLAMA_KEEP_ALIVE", Value: "999999h"},
	}

	cacheMountPath := "/models"
	if m.Spec.Cache.Enabled {
		env = append(env, corev1.EnvVar{Name: "OLLAMA_MODELS", Value: cacheMountPath})
	}

	env = append(env, sortedEnvVars(m.Spec.Env)...)

	startupScript := ollamaStartupScript(modelRef, m.Name)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "model-" + m.Name + "-",
			Namespace:    m.Namespace,
			Labels:       labelsForPod(m, workspaceName),
			Annotations:  annotationsForPod("8000"),
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:  "server",
					Image: image,
					Args:  m.Spec.Args,
					Env:   env,
					Resources: corev1.ResourceRequirements{
						Requests: m.Spec.Resources.Requests,
						Limits:   m.Spec.Resources.Limits,
					},
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: ollamaPort,
							Protocol:      corev1.ProtocolTCP,
							Name:          "http",
						},
					},
					StartupProbe: &corev1.Probe{
						InitialDelaySeconds: 1,
						PeriodSeconds:       3,
						FailureThreshold:    10,
						TimeoutSeconds:      10800,
						ProbeHandler: corev1.ProbeHandler{
							Exec: &corev1.ExecAction{
								Command: []string{"/bin/bash", "-c", startupScript},
							},
						},
					},
					ReadinessProbe: &corev1.Probe{
						FailureThreshold: 3,
						PeriodSeconds:    10,
						TimeoutSeconds:   2,
						SuccessThreshold: 1,
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/",
								Port: intstr.FromString("http"),
							},
						},
					},
					LivenessProbe: &corev1.Probe{
						FailureThreshold:    3,
						InitialDelaySeconds: 900,
						TimeoutSeconds:      3,
						PeriodSeconds:       30,
						SuccessThreshold:    1,
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/",
								Port: intstr.FromString("http"),
							},
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{Name: "dshm", MountPath: "/dev/shm"},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "dshm",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							Medium: corev1.StorageMediumMemory,
						},
					},
				},
			},
		},
	}

	patchCacheVolume(&pod.Spec, m, cacheMountPath)
	return pod
}

// ollamaModelRef strips the "ollama://" prefix if present to get the bare model reference.
func ollamaModelRef(url string) string {
	return strings.TrimPrefix(url, "ollama://")
}

// ollamaStartupScript builds the startup probe script that pulls and loads the model.
func ollamaStartupScript(modelRef, servedName string) string {
	return fmt.Sprintf(
		"/bin/ollama pull %s && /bin/ollama cp %s %s && /bin/ollama run %s hi",
		modelRef, modelRef, servedName, servedName,
	)
}
