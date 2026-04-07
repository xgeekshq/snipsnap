/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	snipsnapv1 "github.com/xgeekshq/snipsnap/api/v1"
	"github.com/xgeekshq/snipsnap/internal/engine"
)

// #region agent log
const debugLogPathCtrl = "/home/eugenio/work/snipsnap/snipsnap/.cursor/debug-e00151.log"
func debugLogController(location, message string, data map[string]interface{}, hypothesisID string) {
	b, _ := json.Marshal(data)
	entry := fmt.Sprintf(`{"sessionId":"e00151","location":%q,"message":%q,"data":%s,"hypothesisId":%q,"timestamp":%d}`, location, message, string(b), hypothesisID, time.Now().UnixMilli())
	if f, err := os.OpenFile(debugLogPathCtrl, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil { f.WriteString(entry + "\n"); f.Close() }
}
// #endregion

// WorkspaceReconciler reconciles a Workspace object
type WorkspaceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=snipsnap.xgeeks.com,resources=workspaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=snipsnap.xgeeks.com,resources=workspaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=snipsnap.xgeeks.com,resources=workspaces/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete

func (r *WorkspaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Reconciling Workspace")

	ws := &snipsnapv1.Workspace{}
	if err := r.Get(ctx, req.NamespacedName, ws); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	statusBefore := ws.Status.DeepCopy()
	defer func() {
		if !reflect.DeepEqual(statusBefore, &ws.Status) {
			if err := r.Status().Update(ctx, ws); err != nil {
				log.Error(err, "Failed to update Workspace status")
			}
		}
	}()

	existingPods := &corev1.PodList{}
	if err := r.List(ctx, existingPods,
		client.InNamespace(ws.Namespace),
		client.MatchingLabels{snipsnapv1.LabelWorkspace: ws.Name},
	); err != nil {
		return ctrl.Result{}, fmt.Errorf("listing workspace pods: %w", err)
	}

	// No active model requested: ensure all inference pods are gone.
	if ws.Spec.ActiveModel == "" {
		for i := range existingPods.Items {
			if err := r.deleteInferencePod(ctx, &existingPods.Items[i]); err != nil {
				return ctrl.Result{}, err
			}
		}
		ws.Status.Phase = snipsnapv1.WorkspacePhaseIdle
		ws.Status.LoadedModel = ""
		ws.Status.InferenceAddress = ""
		return ctrl.Result{}, nil
	}

	// Find the pod for the currently requested model (if any).
	var activePod *corev1.Pod
	var stalePods []*corev1.Pod
	for i := range existingPods.Items {
		pod := &existingPods.Items[i]
		if pod.Labels[snipsnapv1.LabelModel] == ws.Spec.ActiveModel && pod.DeletionTimestamp == nil {
			activePod = pod
		} else {
			stalePods = append(stalePods, pod)
		}
	}

	// Delete any pods that belong to a different model (aggressive switch).
	for _, pod := range stalePods {
		log.Info("Deleting stale inference pod", "pod", pod.Name, "model", pod.Labels[snipsnapv1.LabelModel])
		if err := r.deleteInferencePod(ctx, pod); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Look up the requested Model CR.
	model := &snipsnapv1.Model{}
	if err := r.Get(ctx, types.NamespacedName{Name: ws.Spec.ActiveModel, Namespace: ws.Namespace}, model); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Model CR not found", "model", ws.Spec.ActiveModel)
			ws.Status.Phase = snipsnapv1.WorkspacePhaseIdle
			ws.Status.LoadedModel = ""
			ws.Status.InferenceAddress = ""
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("getting model CR: %w", err)
	}

	// If no pod exists for the active model, create one.
	if activePod == nil {
		ws.Status.Phase = snipsnapv1.WorkspacePhaseSwitching
		ws.Status.LoadedModel = ""
		ws.Status.InferenceAddress = ""

		pod, err := engine.PodForModel(model, ws.Name)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("building pod spec: %w", err)
		}
		if err := controllerutil.SetControllerReference(ws, pod, r.Scheme); err != nil {
			return ctrl.Result{}, fmt.Errorf("setting controller reference: %w", err)
		}

		log.Info("Creating inference pod", "model", model.Name)
		if err := r.Create(ctx, pod); err != nil {
			return ctrl.Result{}, fmt.Errorf("creating inference pod: %w", err)
		}

		ws.Status.Phase = snipsnapv1.WorkspacePhaseLoading
		return ctrl.Result{}, nil
	}

	// Pod exists -- check its readiness.
	// #region agent log
	debugLogController("workspace_controller.go:checkReady", "checking pod readiness", map[string]interface{}{"podName": activePod.Name, "podPhase": string(activePod.Status.Phase), "podIP": activePod.Status.PodIP, "ready": podIsReady(activePod), "conditionCount": len(activePod.Status.Conditions)}, "H1,H2")
	// #endregion
	if podIsReady(activePod) {
		addr := podInferenceAddress(activePod)
		// #region agent log
		debugLogController("workspace_controller.go:setReady", "setting workspace ready", map[string]interface{}{"addr": addr, "model": ws.Spec.ActiveModel}, "H2,H4,H5")
		// #endregion
		ws.Status.Phase = snipsnapv1.WorkspacePhaseReady
		ws.Status.LoadedModel = ws.Spec.ActiveModel
		ws.Status.InferenceAddress = addr
		return ctrl.Result{}, nil
	}

	// Pod exists but not ready yet.
	ws.Status.Phase = snipsnapv1.WorkspacePhaseLoading
	return ctrl.Result{}, nil
}

func (r *WorkspaceReconciler) deleteInferencePod(ctx context.Context, pod *corev1.Pod) error {
	zero := int64(0)
	return client.IgnoreNotFound(r.Delete(ctx, pod, &client.DeleteOptions{
		GracePeriodSeconds: &zero,
		Preconditions:      &metav1.Preconditions{UID: &pod.UID},
	}))
}

func podIsReady(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func podInferenceAddress(pod *corev1.Pod) string {
	port := pod.Annotations[snipsnapv1.ModelPodPortAnnotation]
	if port == "" {
		port = "8000"
	}
	return fmt.Sprintf("%s:%s", pod.Status.PodIP, port)
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkspaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&snipsnapv1.Workspace{}).
		Owns(&corev1.Pod{}).
		Named("workspace").
		Complete(r)
}
