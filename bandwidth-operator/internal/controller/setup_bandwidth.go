package controller

import (
	"context"
	"strings"
	"time"

	networkingv1 "github.com/vacp2p/vaclab-k8s-plugins/api/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *BandwidthReconciler) SetupBandwidthResource(ctx context.Context, bandwidth *networkingv1.Bandwidth) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("setting up vaclab Bandwidth resource")

	// Set initial status to Creating
	bandwidth.Status.Status = networkingv1.Creating
	nodeName := bandwidth.Spec.Node
	bwName := bandwidth.Name
	//make sure node exists and bw resource is valid
	var node corev1.Node
	nodeErr := r.Get(ctx, types.NamespacedName{Name: nodeName}, &node)
	if nodeErr != nil {
		if apierrors.IsNotFound(nodeErr) {
			// Node gone: need to stop and clean up any existing bandwidth resource
			log.Info("Node not found, cleaning up Bandwidth resource", "node", nodeName)
			var bw networkingv1.Bandwidth
			if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, &bw); err == nil {
				_ = r.Delete(ctx, &bw) // ignore error
			}
			bandwidth.Status.Status = networkingv1.Error
			bandwidth.Status.ErrorReason = "node not found"
			r.generateEvent(networkingv1.EventVaclabNodeBandwidthFailed, bandwidth)
			return ctrl.Result{}, nil
		}
		bandwidth.Status.Status = networkingv1.Error
		bandwidth.Status.ErrorReason = "unable to fetch node information"
		log.Info("Node not found, cleaning up Bandwidth resource", "node", nodeName)
		var bw networkingv1.Bandwidth
		if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, &bw); err == nil {
			_ = r.Delete(ctx, &bw) // ignore error
		}
		r.generateEvent(networkingv1.EventVaclabNodeBandwidthFailed, bandwidth)
		return ctrl.Result{}, nodeErr
	}
	if !strings.EqualFold(bwName, node.Name) {
		log.Error(nil, "Node name mismatch", "expected", nodeName, "found", node.Name)
		bandwidth.Status.Status = networkingv1.Error
		bandwidth.Status.ErrorReason = "name mismatch between nodeName and bandwidthName"
		log.Info("Node not found, cleaning up Bandwidth resource", "node", nodeName)
		var bw networkingv1.Bandwidth
		if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, &bw); err == nil {
			_ = r.Delete(ctx, &bw) // ignore error
		}
		r.generateEvent(networkingv1.EventVaclabNodeBandwidthFailed, bandwidth)
		return ctrl.Result{}, nil
	}

	// return if resource already exists
	var bw networkingv1.Bandwidth
	if err := r.Get(ctx, types.NamespacedName{Name: bwName}, &bw); err == nil {
		log.Info("Bandwidth resource already exists, skipping creation", "name", bwName)
		return ctrl.Result{}, nil
	}
	// set default values if not set
	r.setDefaultBandwidthValues(bandwidth)
	bwRequests := bandwidth.Spec.Requests
	reservationInfo := []networkingv1.ReservationInfo{}
	var usedUlLocal, usedDlLocal, usedUlNetwork, usedDlNetwork int64
	usedCapacity := networkingv1.Capacity{}
	remainingCapacity := networkingv1.Capacity{}
	cleanRequests := []networkingv1.BandwidthRequest{}
	if len(bwRequests) > 0 {
		// List pods of the node
		var podList corev1.PodList
		if err := r.List(ctx, &podList, client.MatchingFields{"spec.nodeName": nodeName}); err != nil {
			log.Error(err, "failed to list pods for specified node", "node", nodeName)
			bandwidth.Status.Status = networkingv1.Error
			r.generateEvent(networkingv1.EventVaclabNodeBandwidthFailed, bandwidth)
			return ctrl.Result{}, err
		}
		podsWithBwAnnotation := make(map[string]corev1.Pod)
		for _, pod := range podList.Items {
			if _, exists := pod.Annotations[r.Config.EgressBandwidthAnnotation]; exists {
				podsWithBwAnnotation[string(pod.GetUID())] = pod
			} else if _, exists := pod.Annotations[r.Config.IngressBandwidthAnnotation]; exists {
				podsWithBwAnnotation[string(pod.GetUID())] = pod
			} else {
				continue
			}
		}

		// Calculate used bandwidth from existing requests
		for _, req := range bwRequests {
			if pod, exists := podsWithBwAnnotation[req.PodUid]; exists {
				ul, dl := r.GetBandwidthFromAnnotation(pod)
				cleanRequests = append(cleanRequests, networkingv1.BandwidthRequest{
					PodName:   pod.GetName(),
					Namespace: pod.GetNamespace(),
					PodUid:    string(pod.GetUID()),
					LinkLocal: req.LinkLocal,
					Bandwidth: networkingv1.BandwidthDefinition{
						UlMbps: ul,
						DlMbps: dl,
					},
				})

				workloadRef := GetWorkloadRefFromPod(&pod)
				if req.LinkLocal {
					usedUlLocal += ul
					usedDlLocal += dl
					reservationInfo = append(reservationInfo, networkingv1.ReservationInfo{
						PodName: pod.GetName(),
						PodUid:  string(pod.GetUID()),
						Bandwidth: networkingv1.Capacity{
							Local: networkingv1.BandwidthDefinition{
								UlMbps: ul,
								DlMbps: dl,
							},
						},
						Namespace:      pod.GetNamespace(),
						DeploymentType: workloadRef.Kind,
						DeploymentName: workloadRef.Name,
						DeploymentUid:  workloadRef.UID,
					})
				} else {
					usedUlNetwork += ul
					usedDlNetwork += dl
					reservationInfo = append(reservationInfo, networkingv1.ReservationInfo{
						PodName: pod.GetName(),
						PodUid:  string(pod.GetUID()),
						Bandwidth: networkingv1.Capacity{
							Network: networkingv1.BandwidthDefinition{
								UlMbps: ul,
								DlMbps: dl,
							},
						},
						Namespace:      pod.GetNamespace(),
						DeploymentType: workloadRef.Kind,
						DeploymentName: workloadRef.Name,
						DeploymentUid:  workloadRef.UID,
					})
				}

			}
		}
		usedCapacity.Local = networkingv1.BandwidthDefinition{
			UlMbps: usedUlLocal,
			DlMbps: usedDlLocal,
		}
		usedCapacity.Network = networkingv1.BandwidthDefinition{
			UlMbps: usedUlNetwork,
			DlMbps: usedDlNetwork,
		}
		remainingCapacity.Local = networkingv1.BandwidthDefinition{
			UlMbps: bandwidth.Spec.Capacity.Local.UlMbps - usedUlLocal,
			DlMbps: bandwidth.Spec.Capacity.Local.DlMbps - usedDlLocal,
		}
		remainingCapacity.Network = networkingv1.BandwidthDefinition{
			UlMbps: bandwidth.Spec.Capacity.Network.UlMbps - usedUlNetwork,
			DlMbps: bandwidth.Spec.Capacity.Network.DlMbps - usedDlNetwork,
		}
	}
	bandwidth.Spec.Requests = cleanRequests
	bandwidth.Status.Used = usedCapacity
	bandwidth.Status.Remaining = remainingCapacity
	bandwidth.Status.Reservations = reservationInfo
	bandwidth.Status.Capacity = bandwidth.Spec.Capacity
	bandwidth.Status.Status = networkingv1.Created

	if err := r.UpdateBandwidthStatus(ctx, bandwidth); err != nil {
		log.Error(err, "unable to update bandwidth resource status", "node", nodeName)
	}
	r.Update(ctx, bandwidth)
	r.generateEvent(networkingv1.EventVaclabNodeBandwidthCreated, bandwidth)
	// Requeue to check the status later
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// used only when manually changing the spec of an existing Bandwidth resource
// only max capacity changes are allowed manually
func (r *BandwidthReconciler) CheckAndUpdateBandwidth(ctx context.Context, bandwidth *networkingv1.Bandwidth) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("watching vaclab Bandwidth resource for updates")

	nodeName := bandwidth.Spec.Node
	bwName := bandwidth.Name
	//make sure node exists and bw resource is valid
	var node corev1.Node
	nodeErr := r.Get(ctx, types.NamespacedName{Name: nodeName}, &node)
	if nodeErr != nil {
		if apierrors.IsNotFound(nodeErr) {
			// Node gone: need to stop and clean up any existing bandwidth resource
			log.Info("Node not found, cleaning up Bandwidth resource", "node", nodeName)
			var bw networkingv1.Bandwidth
			if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, &bw); err == nil {
				_ = r.Delete(ctx, &bw) // ignore error
			}
			bandwidth.Status.Status = networkingv1.Error
			bandwidth.Status.ErrorReason = "node not found"
			r.generateEvent(networkingv1.EventVaclabNodeBandwidthFailed, bandwidth)
			return ctrl.Result{}, nil
		}
		bandwidth.Status.Status = networkingv1.Error
		bandwidth.Status.ErrorReason = "unable to fetch node information"
		log.Info("Node not found, cleaning up Bandwidth resource", "node", nodeName)
		var bw networkingv1.Bandwidth
		if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, &bw); err == nil {
			_ = r.Delete(ctx, &bw) // ignore error
		}
		r.generateEvent(networkingv1.EventVaclabNodeBandwidthFailed, bandwidth)
		return ctrl.Result{}, nodeErr
	}
	if !strings.EqualFold(bwName, node.Name) {
		log.Error(nil, "Node name mismatch", "expected", nodeName, "found", node.Name)
		bandwidth.Status.Status = networkingv1.Error
		bandwidth.Status.ErrorReason = "name mismatch between nodeName and bandwidthName"
		log.Info("Node not found, cleaning up Bandwidth resource", "node", nodeName)
		var bw networkingv1.Bandwidth
		if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, &bw); err == nil {
			_ = r.Delete(ctx, &bw) // ignore error
		}
		r.generateEvent(networkingv1.EventVaclabNodeBandwidthFailed, bandwidth)
		return ctrl.Result{}, nil
	}

	//currentSpec := bandwidth.Spec.Requests
	state := bandwidth.Status
	if len(state.Reservations) != len(bandwidth.Spec.Requests) {
		// changes in requests are not allowed
		log.Error(nil, "Changes in bandwidth requests are not allowed after creation", "node", nodeName)
		return ctrl.Result{}, nil
	}

	//expectedCapacity := bandwidth.Status.Capacity
	if state.Capacity != bandwidth.Spec.Capacity {
		state.Capacity = bandwidth.Spec.Capacity
		state.Remaining = networkingv1.Capacity{
			Local: networkingv1.BandwidthDefinition{
				UlMbps: bandwidth.Spec.Capacity.Local.UlMbps - state.Used.Local.UlMbps,
				DlMbps: bandwidth.Spec.Capacity.Local.DlMbps - state.Used.Local.DlMbps,
			},
			Network: networkingv1.BandwidthDefinition{
				UlMbps: bandwidth.Spec.Capacity.Network.UlMbps - state.Used.Network.UlMbps,
				DlMbps: bandwidth.Spec.Capacity.Network.DlMbps - state.Used.Network.DlMbps,
			},
		}
	}
	bandwidth.Status = state

	if err := r.UpdateBandwidthStatus(ctx, bandwidth); err != nil {
		log.Error(err, "unable to update bandwidth resource status", "node", nodeName)
		return ctrl.Result{}, err
	}
	r.Update(ctx, bandwidth)
	// Requeue to check the status later
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *BandwidthReconciler) SyncFromSpecAndPods(ctx context.Context, bw *networkingv1.Bandwidth) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	nodeName := bw.Spec.Node
	bwName := bw.Name

	// Validate node exists
	var node corev1.Node
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, &node); err != nil {
		if apierrors.IsNotFound(err) {
			// Node gone => delete BW
			log.Info("Node not found, deleting Bandwidth", "node", nodeName)
			_ = r.Delete(ctx, bw)
			return ctrl.Result{}, nil
		}
		bw.Status.Status = networkingv1.Error
		bw.Status.ErrorReason = "unable to fetch node information"
		_ = r.UpdateBandwidthStatus(ctx, bw)
		return ctrl.Result{}, err
	}

	// Enforce invariant: BW name == node name
	if !strings.EqualFold(bwName, node.Name) {
		bw.Status.Status = networkingv1.Error
		bw.Status.ErrorReason = "name mismatch between nodeName and bandwidthName"
		_ = r.UpdateBandwidthStatus(ctx, bw)
		return ctrl.Result{}, nil
	}

	// set Defaults
	r.setDefaultBandwidthValues(bw)

	// List pods on the node (all namespaces)
	var podList corev1.PodList
	if err := r.List(ctx, &podList, client.MatchingFields{"spec.nodeName": nodeName}); err != nil {
		bw.Status.Status = networkingv1.Error
		bw.Status.ErrorReason = "failed to list pods for specified node"
		_ = r.UpdateBandwidthStatus(ctx, bw)
		return ctrl.Result{}, err
	}

	// Build map of pods that currently have bw annotations
	podsWithBw := make(map[string]corev1.Pod)
	for _, pod := range podList.Items {
		if pod.DeletionTimestamp != nil {
			continue
		}
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		if _, ok := pod.Annotations[r.Config.EgressBandwidthAnnotation]; ok {
			podsWithBw[string(pod.UID)] = pod
			continue
		}
		if _, ok := pod.Annotations[r.Config.IngressBandwidthAnnotation]; ok {
			podsWithBw[string(pod.UID)] = pod
			continue
		}
	}

	// Recompute based on spec.requests and current pod annotations
	bwRequests := bw.Spec.Requests
	cleanRequests := make([]networkingv1.BandwidthRequest, 0, len(bwRequests))
	reservationInfo := make([]networkingv1.ReservationInfo, 0, len(bwRequests))
	var usedUlLocal, usedDlLocal, usedUlNetwork, usedDlNetwork int64

	for _, req := range bwRequests {
		pod, exists := podsWithBw[req.PodUid]
		if !exists {
			// pod gone or no bw annotation anymore => drop request
			continue
		}

		ul, dl := r.GetBandwidthFromAnnotation(pod)
		cleanRequests = append(cleanRequests, networkingv1.BandwidthRequest{
			PodName:   pod.Name,
			Namespace: pod.Namespace,
			PodUid:    string(pod.UID),
			LinkLocal: req.LinkLocal, // scheduler will control this later
			Bandwidth: networkingv1.BandwidthDefinition{
				UlMbps: ul,
				DlMbps: dl,
			},
		})

		workloadRef := GetWorkloadRefFromPod(&pod)

		if req.LinkLocal {
			usedUlLocal += ul
			usedDlLocal += dl
			reservationInfo = append(reservationInfo, networkingv1.ReservationInfo{
				PodName: pod.Name,
				PodUid:  string(pod.UID),
				Bandwidth: networkingv1.Capacity{
					Local: networkingv1.BandwidthDefinition{UlMbps: ul, DlMbps: dl},
				},
				Namespace:      pod.Namespace,
				DeploymentType: workloadRef.Kind,
				DeploymentName: workloadRef.Name,
				DeploymentUid:  workloadRef.UID,
			})
		} else {
			usedUlNetwork += ul
			usedDlNetwork += dl
			reservationInfo = append(reservationInfo, networkingv1.ReservationInfo{
				PodName: pod.Name,
				PodUid:  string(pod.UID),
				Bandwidth: networkingv1.Capacity{
					Network: networkingv1.BandwidthDefinition{UlMbps: ul, DlMbps: dl},
				},
				Namespace:      pod.Namespace,
				DeploymentType: workloadRef.Kind,
				DeploymentName: workloadRef.Name,
				DeploymentUid:  workloadRef.UID,
			})
		}
	}

	// Compute used/remaining bw (manual changes automatically applied)
	usedCapacity := networkingv1.Capacity{
		Local: networkingv1.BandwidthDefinition{
			UlMbps: usedUlLocal,
			DlMbps: usedDlLocal,
		},
		Network: networkingv1.BandwidthDefinition{
			UlMbps: usedUlNetwork,
			DlMbps: usedDlNetwork,
		},
	}

	remainingCapacity := networkingv1.Capacity{
		Local: networkingv1.BandwidthDefinition{
			UlMbps: bw.Spec.Capacity.Local.UlMbps - usedUlLocal,
			DlMbps: bw.Spec.Capacity.Local.DlMbps - usedDlLocal,
		},
		Network: networkingv1.BandwidthDefinition{
			UlMbps: bw.Spec.Capacity.Network.UlMbps - usedUlNetwork,
			DlMbps: bw.Spec.Capacity.Network.DlMbps - usedDlNetwork,
		},
	}

	bw.Spec.Requests = cleanRequests
	bw.Status.Used = usedCapacity
	bw.Status.Remaining = remainingCapacity
	bw.Status.Reservations = reservationInfo
	bw.Status.Capacity = bw.Spec.Capacity
	bw.Status.Status = networkingv1.Created
	bw.Status.ErrorReason = ""

	// Update spec (because we changed spec.requests) then status
	if err := r.Update(ctx, bw); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.UpdateBandwidthStatus(ctx, bw); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
