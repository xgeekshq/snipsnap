package engine

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	snipsnapv1 "github.com/xgeekshq/snipsnap/api/v1"
)

const (
	defaultVLLMImage = "vllm/vllm-openai:latest"
	vllmPort         = 8000
)

func vllmPodForModel(m *snipsnapv1.Model, workspaceName string) *corev1.Pod {
	image := m.Spec.Image
	if image == "" {
		image = defaultVLLMImage
	}

	modelFlag := m.Spec.URL
	cacheMountPath := "/models"
	if m.Spec.Cache.Enabled {
		modelFlag = cacheMountPath
	}

	args := []string{
		"--model=" + modelFlag,
		"--served-model-name=" + m.Name,
	}
	args = append(args, m.Spec.Args...)

	env := sortedEnvVars(m.Spec.Env)

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
					Name:    "server",
					Image:   image,
					Command: []string{"python3", "-m", "vllm.entrypoints.openai.api_server"},
					Args:    args,
					Env:     env,
					Resources: corev1.ResourceRequirements{
						Requests: m.Spec.Resources.Requests,
						Limits:   m.Spec.Resources.Limits,
					},
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: vllmPort,
							Protocol:      corev1.ProtocolTCP,
							Name:          "http",
						},
					},
					StartupProbe: &corev1.Probe{
						FailureThreshold: 1800,
						PeriodSeconds:    2,
						TimeoutSeconds:   2,
						SuccessThreshold: 1,
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/health",
								Port: intstr.FromString("http"),
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
								Path: "/health",
								Port: intstr.FromString("http"),
							},
						},
					},
					LivenessProbe: &corev1.Probe{
						FailureThreshold: 3,
						PeriodSeconds:    30,
						TimeoutSeconds:   3,
						SuccessThreshold: 1,
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/health",
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
