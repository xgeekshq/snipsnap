package controller

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	snipsnapv1 "github.com/xgeekshq/snipsnap/api/v1"
	"github.com/xgeekshq/snipsnap/internal/engine"
)

var _ = Describe("Model Controller", func() {
	const (
		timeout  = 10 * time.Second
		interval = 250 * time.Millisecond
	)

	Context("When creating a Model with cache enabled", func() {
		It("Should create a PVC for model cache", func() {
			model := &snipsnapv1.Model{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model-pvc",
					Namespace: "default",
				},
				Spec: snipsnapv1.ModelSpec{
					URL:    "ollama://llama3",
					Engine: snipsnapv1.OLlamaEngine,
					Cache: snipsnapv1.ModelCache{
						Enabled:     true,
						StorageSize: "10Gi",
					},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			pvcName := engine.CachePVCName(model)
			pvc := &corev1.PersistentVolumeClaim{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      pvcName,
					Namespace: "default",
				}, pvc)
			}, timeout, interval).Should(Succeed())

			Expect(pvc.Labels[snipsnapv1.LabelManagedBy]).To(Equal(snipsnapv1.ManagedByValue))
			Expect(pvc.Labels[snipsnapv1.LabelModel]).To(Equal("test-model-pvc"))
			Expect(pvc.Spec.AccessModes).To(ContainElement(corev1.ReadWriteOnce))

			storage := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
			Expect(storage.String()).To(Equal("10Gi"))
		})
	})

	Context("When creating a Model without cache", func() {
		It("Should not create a PVC", func() {
			model := &snipsnapv1.Model{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model-nocache",
					Namespace: "default",
				},
				Spec: snipsnapv1.ModelSpec{
					URL:    "ollama://llama3",
					Engine: snipsnapv1.OLlamaEngine,
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			pvcName := engine.CachePVCName(model)
			pvc := &corev1.PersistentVolumeClaim{}
			Consistently(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      pvcName,
					Namespace: "default",
				}, pvc)
				return err != nil
			}, 2*time.Second, interval).Should(BeTrue())
		})
	})

	Context("When creating a Model with an existing PVC name", func() {
		It("Should not create a new PVC", func() {
			model := &snipsnapv1.Model{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model-existing",
					Namespace: "default",
				},
				Spec: snipsnapv1.ModelSpec{
					URL:    "ollama://llama3",
					Engine: snipsnapv1.OLlamaEngine,
					Cache: snipsnapv1.ModelCache{
						Enabled:         true,
						ExistingPVCName: "my-preexisting-pvc",
					},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			autoName := engine.CachePVCName(model)
			pvc := &corev1.PersistentVolumeClaim{}
			Consistently(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      autoName,
					Namespace: "default",
				}, pvc)
				return err != nil
			}, 2*time.Second, interval).Should(BeTrue())
		})
	})
})
