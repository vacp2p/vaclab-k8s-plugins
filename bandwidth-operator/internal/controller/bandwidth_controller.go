/*
Copyright 2025.

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
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	//networkingv1 "github.com/vacp2p/vaclab-k8s-plugins/api/v1"
	networkingv1 "github.com/vacp2p/vaclab-k8s-plugins/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// BandwidthReconciler reconciles a Bandwidth object
type BandwidthReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Config   BandwidthConfig
}

// BandwidthConfig holds operator configuration
type BandwidthConfig struct {
	DefaultLocalUL             int64
	DefaultLocalDL             int64
	DefaultNetworkUL           int64
	DefaultNetworkDL           int64
	EgressBandwidthAnnotation  string
	IngressBandwidthAnnotation string
}

const FinalizerName = "finalizer.bandwidth.vaclab.org"

// +kubebuilder:rbac:groups=networking.vaclab.org,resources=bandwidths,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.vaclab.org,resources=bandwidths/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.vaclab.org,resources=bandwidths/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Bandwidth object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.4/pkg/reconcile
func (r *BandwidthReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("reconciling vaclab node bandwidth resource")

	var bandwidth networkingv1.Bandwidth
	err := r.Get(ctx, req.NamespacedName, &bandwidth)
	if err != nil {
		if errors.IsNotFound(err) {
			// This reconcile may come from Pod/Node watch. If node exists, create BW CR.
			var node corev1.Node
			nerr := r.Get(ctx, types.NamespacedName{Name: req.Name}, &node)
			if nerr != nil {
				if errors.IsNotFound(nerr) {
					// Node doesn't exist; nothing to do
					return ctrl.Result{}, nil
				}
				return ctrl.Result{}, nerr
			}

			// Create BW resource named after node
			bw := networkingv1.Bandwidth{
				ObjectMeta: metav1.ObjectMeta{
					Name: req.Name,
				},
				Spec: networkingv1.BandwidthSpec{
					Node: req.Name,
				},
			}
			r.setDefaultBandwidthValues(&bw)

			if !controllerutil.ContainsFinalizer(&bw, FinalizerName) {
				controllerutil.AddFinalizer(&bw, FinalizerName)
			}

			// Generate event
			r.generateEvent(networkingv1.EventVaclabNodeBandwidthCreating, &bw)
			var bwExist bool
			if cerr := r.Create(ctx, &bw); cerr != nil {
				// AlreadyExists can happen under race
				if !errors.IsAlreadyExists(cerr) {
					// Set error status if creation failed
					if fetchErr := r.Get(ctx, client.ObjectKeyFromObject(&bw), &bw); fetchErr == nil {
						bw.Status.Status = networkingv1.Error
						bw.Status.UpdatedAt = metav1.NewTime(time.Now())
						r.Status().Update(ctx, &bw)
						r.generateEvent(networkingv1.EventVaclabNodeBandwidthFailed, &bw)
					}
					return ctrl.Result{}, cerr
				} else {
					bwExist = true
					log.Info("vaclab bandwidth watcher", "message", "bandwidth resource already exists, skipping creation", "name", bw.Name, "node", bw.Spec.Node)
				}
			}

			// Update status after creation (status cannot be set during Create)
			if !bwExist {
				// Fetch the created resource to update its status
				if fetchErr := r.Get(ctx, client.ObjectKeyFromObject(&bw), &bw); fetchErr != nil {
					return ctrl.Result{}, fetchErr
				}
				bw.Status.Status = networkingv1.Created
				bw.Status.Capacity = bw.Spec.Capacity
				bw.Status.UpdatedAt = metav1.NewTime(time.Now())
				if statusErr := r.Status().Update(ctx, &bw); statusErr != nil {
					// If conflict, requeue to retry
					if errors.IsConflict(statusErr) {
						return ctrl.Result{Requeue: true}, nil
					}
					return ctrl.Result{}, statusErr
				}
				r.generateEvent(networkingv1.EventVaclabNodeBandwidthCreated, &bw)
				log.Info("vaclab bandwidth watcher", "message", "created bandwidth resource for node", "name", bw.Name, "node", bw.Spec.Node)
			} // Requeue to let the normal reconcile flow compute status
			return ctrl.Result{Requeue: true}, nil
		}

		log.Error(err, "unable to fetch vaclab node bandwidth resource")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !controllerutil.ContainsFinalizer(&bandwidth, FinalizerName) {
		controllerutil.AddFinalizer(&bandwidth, FinalizerName)
		if err := r.Update(ctx, &bandwidth); err != nil {
			return ctrl.Result{}, err
		}
	}

	if !bandwidth.ObjectMeta.DeletionTimestamp.IsZero() {
		// The resource is being deleted
		r.generateEvent(networkingv1.EventVaclabNodeBandwidthDeleting, &bandwidth)
		if controllerutil.ContainsFinalizer(&bandwidth, FinalizerName) {
			log.Info("Performing cleanup before deleting vaclab node bandwidth resource")
			err := r.Get(ctx, req.NamespacedName, &bandwidth)
			if err != nil {
				return ctrl.Result{}, err
			}
			// Remove the finalizer to allow Kubernetes to delete the resource
			controllerutil.RemoveFinalizer(&bandwidth, FinalizerName)
			log.Info("vaclab bandwidth watcher", "message", "removed finalizer successfully")
			bandwidth.Status.Status = networkingv1.Deleted
			r.generateEvent(networkingv1.EventVaclabNodeBandwidthDeleted, &bandwidth)
			if err := r.Update(ctx, &bandwidth); err != nil {
				log.Error(err, "failed to remove finalizer")
				return ctrl.Result{}, err
			}

		}
		// Stop reconciliation as the object is being deleted
		log.Info("vaclab bandwidth watcher", "message", "successfully removed node bandwidth resource", "name", bandwidth.Name, "node", bandwidth.Spec.Node)
		return ctrl.Result{}, nil
	}

	log.Info(fmt.Sprintf("found vaclab Bandwidth resource to reconcile: %v", bandwidth))

	if bandwidth.Status.Status == networkingv1.None || bandwidth.Status.Status == "" || bandwidth.Status.Status == networkingv1.Created {
		return r.SyncFromSpecAndPods(ctx, &bandwidth)
	}

	if bandwidth.Status.Status == networkingv1.Deleted {
		return ctrl.Result{}, nil
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BandwidthReconciler) SetupWithManager(mgr ctrl.Manager) error {

	ctx := context.Background()

	// Register index: Pod.spec.nodeName to enable efficient Pod listing per node name
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&corev1.Pod{},
		"spec.nodeName",
		func(rawObj client.Object) []string {
			pod := rawObj.(*corev1.Pod)
			if pod.Spec.NodeName == "" {
				return nil
			}
			return []string{pod.Spec.NodeName}
		},
	); err != nil {
		return err
	}

	// Bootstrap: make sure to create one Bandwidth Resource per Node
	if err := mgr.Add(manager.RunnableFunc(func(runCtx context.Context) error {
		return r.createBandwidthResourcesForAllNodes(runCtx)
	})); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.Bandwidth{}).
		Named("bandwidth").

		// 1) Watch Nodes: node "node-1" => reconcile Bandwidth named "node-1"
		Watches(
			&corev1.Node{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				n := obj.(*corev1.Node)
				return []reconcile.Request{{
					NamespacedName: types.NamespacedName{Name: n.Name}, // BW is cluster-scoped
				}}
			}),
		).

		// 2) Watch Pods: pod on node "node-1" => reconcile Bandwidth named "node-1"
		Watches(
			&corev1.Pod{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				p := obj.(*corev1.Pod)
				if p.Spec.NodeName == "" {
					return nil // unscheduled; nothing to update yet
				}
				return []reconcile.Request{{
					NamespacedName: types.NamespacedName{Name: p.Spec.NodeName},
				}}
			}),
		).
		Complete(r)
}
