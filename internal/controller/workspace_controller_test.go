package controller

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	snipsnapv1 "github.com/xgeekshq/snipsnap/api/v1"
)

var _ = Describe("Workspace Controller", func() {
	const (
		timeout  = 10 * time.Second
		interval = 250 * time.Millisecond
	)

	Context("When setting an active model", func() {
		It("Should create an inference pod", func() {
			model := &snipsnapv1.Model{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ws-test-model",
					Namespace: "default",
				},
				Spec: snipsnapv1.ModelSpec{
					URL:    "ollama://llama3",
					Engine: snipsnapv1.OLlamaEngine,
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							"nvidia.com/gpu": resource.MustParse("1"),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			ws := &snipsnapv1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ws-test",
					Namespace: "default",
				},
				Spec: snipsnapv1.WorkspaceSpec{
					ActiveModel: "ws-test-model",
				},
			}
			Expect(k8sClient.Create(ctx, ws)).To(Succeed())

			// Expect an inference pod to be created with the correct labels.
			podList := &corev1.PodList{}
			Eventually(func() int {
				_ = k8sClient.List(ctx, podList,
					client.InNamespace("default"),
					client.MatchingLabels{snipsnapv1.LabelWorkspace: "ws-test"},
				)
				return len(podList.Items)
			}, timeout, interval).Should(Equal(1))

			pod := podList.Items[0]
			Expect(pod.Labels[snipsnapv1.LabelModel]).To(Equal("ws-test-model"))
			Expect(pod.Labels[snipsnapv1.LabelEngine]).To(Equal(snipsnapv1.OLlamaEngine))
			Expect(pod.Spec.Containers).To(HaveLen(1))
			Expect(pod.Spec.Containers[0].Name).To(Equal("server"))
		})
	})

	Context("When clearing the active model", func() {
		It("Should delete all inference pods and go idle", func() {
			model := &snipsnapv1.Model{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ws-idle-model",
					Namespace: "default",
				},
				Spec: snipsnapv1.ModelSpec{
					URL:    "ollama://llama3",
					Engine: snipsnapv1.OLlamaEngine,
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			ws := &snipsnapv1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ws-idle-test",
					Namespace: "default",
				},
				Spec: snipsnapv1.WorkspaceSpec{
					ActiveModel: "ws-idle-model",
				},
			}
			Expect(k8sClient.Create(ctx, ws)).To(Succeed())

			// Wait for pod to be created.
			podList := &corev1.PodList{}
			Eventually(func() int {
				_ = k8sClient.List(ctx, podList,
					client.InNamespace("default"),
					client.MatchingLabels{snipsnapv1.LabelWorkspace: "ws-idle-test"},
				)
				return len(podList.Items)
			}, timeout, interval).Should(Equal(1))

			// Clear the active model using merge patch to avoid conflicts.
			freshWs := &snipsnapv1.Workspace{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ws-idle-test", Namespace: "default"}, freshWs)).To(Succeed())
			patch := client.MergeFrom(freshWs.DeepCopy())
			freshWs.Spec.ActiveModel = ""
			Expect(k8sClient.Patch(ctx, freshWs, patch)).To(Succeed())

			// Pods should be deleted.
			Eventually(func() int {
				_ = k8sClient.List(ctx, podList,
					client.InNamespace("default"),
					client.MatchingLabels{snipsnapv1.LabelWorkspace: "ws-idle-test"},
				)
				return len(podList.Items)
			}, timeout, interval).Should(Equal(0))

			// Status should be Idle.
			Eventually(func() snipsnapv1.WorkspacePhase {
				ws := &snipsnapv1.Workspace{}
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: "ws-idle-test", Namespace: "default"}, ws)
				return ws.Status.Phase
			}, timeout, interval).Should(Equal(snipsnapv1.WorkspacePhaseIdle))
		})
	})

	Context("When switching models", func() {
		It("Should delete the old pod and create a new one", func() {
			modelA := &snipsnapv1.Model{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "switch-model-a",
					Namespace: "default",
				},
				Spec: snipsnapv1.ModelSpec{
					URL:    "ollama://modelA",
					Engine: snipsnapv1.OLlamaEngine,
				},
			}
			modelB := &snipsnapv1.Model{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "switch-model-b",
					Namespace: "default",
				},
				Spec: snipsnapv1.ModelSpec{
					URL:    "ollama://modelB",
					Engine: snipsnapv1.OLlamaEngine,
				},
			}
			Expect(k8sClient.Create(ctx, modelA)).To(Succeed())
			Expect(k8sClient.Create(ctx, modelB)).To(Succeed())

			ws := &snipsnapv1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ws-switch-test",
					Namespace: "default",
				},
				Spec: snipsnapv1.WorkspaceSpec{
					ActiveModel: "switch-model-a",
				},
			}
			Expect(k8sClient.Create(ctx, ws)).To(Succeed())

			// Wait for model A pod.
			podList := &corev1.PodList{}
			Eventually(func() int {
				_ = k8sClient.List(ctx, podList,
					client.InNamespace("default"),
					client.MatchingLabels{
						snipsnapv1.LabelWorkspace: "ws-switch-test",
						snipsnapv1.LabelModel:     "switch-model-a",
					},
				)
				return len(podList.Items)
			}, timeout, interval).Should(Equal(1))

			// Switch to model B using merge patch to avoid conflicts.
			freshWs := &snipsnapv1.Workspace{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ws-switch-test", Namespace: "default"}, freshWs)).To(Succeed())
			patch := client.MergeFrom(freshWs.DeepCopy())
			freshWs.Spec.ActiveModel = "switch-model-b"
			Expect(k8sClient.Patch(ctx, freshWs, patch)).To(Succeed())

			// Model B pod should appear.
			Eventually(func() int {
				_ = k8sClient.List(ctx, podList,
					client.InNamespace("default"),
					client.MatchingLabels{
						snipsnapv1.LabelWorkspace: "ws-switch-test",
						snipsnapv1.LabelModel:     "switch-model-b",
					},
				)
				return len(podList.Items)
			}, timeout, interval).Should(Equal(1))

			// Model A pods should be gone.
			Eventually(func() int {
				_ = k8sClient.List(ctx, podList,
					client.InNamespace("default"),
					client.MatchingLabels{
						snipsnapv1.LabelWorkspace: "ws-switch-test",
						snipsnapv1.LabelModel:     "switch-model-a",
					},
				)
				return len(podList.Items)
			}, timeout, interval).Should(Equal(0))
		})
	})
})
