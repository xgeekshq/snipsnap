package controller

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	snipsnapv1 "github.com/xgeekshq/snipsnap/api/v1"
	"github.com/xgeekshq/snipsnap/internal/engine"
)

// ModelReconciler reconciles a Model object
type ModelReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=snipsnap.xgeeks.com,resources=models,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=snipsnap.xgeeks.com,resources=models/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=snipsnap.xgeeks.com,resources=models/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete

func (r *ModelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Reconciling Model")

	model := &snipsnapv1.Model{}
	if err := r.Get(ctx, req.NamespacedName, model); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	statusBefore := model.Status.DeepCopy()

	defer func() {
		if !reflect.DeepEqual(statusBefore, &model.Status) {
			if err := r.Status().Update(ctx, model); err != nil {
				log.Error(err, "Failed to update Model status")
			}
		}
	}()

	if !model.Spec.Cache.Enabled {
		model.Status.CacheReady = false
		return ctrl.Result{}, nil
	}

	if model.Spec.Cache.ExistingPVCName != "" {
		pvc := &corev1.PersistentVolumeClaim{}
		err := r.Get(ctx, client.ObjectKey{
			Namespace: model.Namespace,
			Name:      model.Spec.Cache.ExistingPVCName,
		}, pvc)
		if err != nil {
			if errors.IsNotFound(err) {
				log.Info("Existing PVC not found", "pvc", model.Spec.Cache.ExistingPVCName)
				model.Status.CacheReady = false
				return ctrl.Result{}, nil
			}
			return ctrl.Result{}, fmt.Errorf("checking existing PVC: %w", err)
		}
		model.Status.CacheReady = pvc.Status.Phase == corev1.ClaimBound
		return ctrl.Result{}, nil
	}

	pvcName := engine.CachePVCName(model)
	pvc := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, client.ObjectKey{Namespace: model.Namespace, Name: pvcName}, pvc)
	if err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("getting PVC: %w", err)
		}

		storageSize := model.Spec.Cache.StorageSize
		if storageSize == "" {
			storageSize = "50Gi"
		}

		pvc = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcName,
				Namespace: model.Namespace,
				Labels: map[string]string{
					snipsnapv1.LabelManagedBy: snipsnapv1.ManagedByValue,
					snipsnapv1.LabelModel:     model.Name,
				},
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse(storageSize),
					},
				},
			},
		}
		if model.Spec.Cache.StorageClassName != nil {
			pvc.Spec.StorageClassName = model.Spec.Cache.StorageClassName
		}

		if err := controllerutil.SetControllerReference(model, pvc, r.Scheme); err != nil {
			return ctrl.Result{}, fmt.Errorf("setting controller reference on PVC: %w", err)
		}

		log.Info("Creating PVC for model cache", "pvc", pvcName)
		if err := r.Create(ctx, pvc); err != nil {
			if errors.IsAlreadyExists(err) {
				log.Info("PVC already exists", "pvc", pvcName)
			} else {
				return ctrl.Result{}, fmt.Errorf("creating PVC: %w", err)
			}
		}
	}

	model.Status.CacheReady = pvc.Status.Phase == corev1.ClaimBound
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ModelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&snipsnapv1.Model{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Named("model").
		Complete(r)
}
