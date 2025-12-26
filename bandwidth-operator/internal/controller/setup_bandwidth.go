package controller

import (
	"context"
	"strings"
	"sync"
	"time"

	networkingv1 "github.com/vacp2p/vaclab-k8s-plugins/api/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
					usedUlLocal += ul
					usedDlLocal += dl
					reservationInfo = append(reservationInfo, networkingv1.ReservationInfo{
						PodName: pod.GetName(),
						PodUid:  string(pod.GetUID()),
						Bandwidth: networkingv1.Capacity{
							Network: networkingv1.BandwidthDefinition{
								UlMbps: ul,
								DlMbps: dl,
							},
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
	bandwidth.Status.UpdatedAt = metav1.NewTime(time.Now())

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
	bandwidth.Status.UpdatedAt = metav1.NewTime(time.Now())

	if err := r.UpdateBandwidthStatus(ctx, bandwidth); err != nil {
		log.Error(err, "unable to update bandwidth resource status", "node", nodeName)
		return ctrl.Result{}, err
	}
	r.Update(ctx, bandwidth)
	// Requeue to check the status later
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

/*func (r *BandwidthReconciler) SyncFromSpecAndPods(ctx context.Context, bw *networkingv1.Bandwidth) (ctrl.Result, error) {
	return r.syncFromSpecAndPodsWithVisited(ctx, bw, make(map[string]bool))
}*/

// syncSingleNodeBandwidth updates a single bandwidth resource without cascading to related nodes
// Used for background updates triggered by sibling pod changes
func (r *BandwidthReconciler) syncSingleNodeBandwidth(ctx context.Context, bw *networkingv1.Bandwidth) error {
	log := log.FromContext(ctx)
	nodeName := bw.Spec.Node

	// Validate node exists
	var node corev1.Node
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, &node); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Node not found, deleting Bandwidth", "node", nodeName)
			_ = r.Delete(ctx, bw)
			return nil
		}
		return err
	}

	// Set defaults
	r.setDefaultBandwidthValues(bw)

	// List pods on the node
	var podList corev1.PodList
	if err := r.List(ctx, &podList, client.MatchingFields{"spec.nodeName": nodeName}); err != nil {
		return err
	}

	// Build map of pods with bandwidth annotations
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

	// Recompute bandwidth
	bwRequests := bw.Spec.Requests
	cleanRequests := make([]networkingv1.BandwidthRequest, 0, len(bwRequests))
	reservationInfo := make([]networkingv1.ReservationInfo, 0, len(bwRequests))
	var usedUlLocal, usedDlLocal, usedUlNetwork, usedDlNetwork int64

	// Cache sibling lookups
	type siblingCacheEntry struct {
		isLocal bool
	}
	siblingCache := make(map[string]siblingCacheEntry)

	for _, req := range bwRequests {
		pod, exists := podsWithBw[req.PodUid]
		if !exists {
			log.V(1).Info("dropping request for missing pod", "podUid", req.PodUid, "podName", req.PodName)
			continue
		}

		ul, dl := r.GetBandwidthFromAnnotation(pod)
		usedUlLocal += ul
		usedDlLocal += dl

		var nUl, nDl int64
		linkLocal := false

		workloadRef := GetWorkloadRefFromPod(&pod)
		ownerUID := workloadRef.UID

		var isLocal bool
		if cached, ok := siblingCache[ownerUID]; ok {
			isLocal = cached.isLocal
		} else {
			_, _, localResult := r.allSiblingsOnCurrentNode(ctx, &pod, nodeName)
			isLocal = localResult
			siblingCache[ownerUID] = siblingCacheEntry{isLocal: isLocal}
		}

		if !isLocal {
			nUl = ul
			nDl = dl
		} else {
			linkLocal = true
		}

		cleanRequests = append(cleanRequests, networkingv1.BandwidthRequest{
			PodName:   pod.Name,
			Namespace: pod.Namespace,
			PodUid:    string(pod.UID),
			LinkLocal: linkLocal,
			Bandwidth: networkingv1.BandwidthDefinition{
				UlMbps: ul,
				DlMbps: dl,
			},
		})

		usedUlNetwork += nUl
		usedDlNetwork += nDl

		reservationInfo = append(reservationInfo, networkingv1.ReservationInfo{
			PodName: pod.Name,
			PodUid:  string(pod.UID),
			Bandwidth: networkingv1.Capacity{
				Local:   networkingv1.BandwidthDefinition{UlMbps: ul, DlMbps: dl},
				Network: networkingv1.BandwidthDefinition{UlMbps: nUl, DlMbps: nDl},
			},
			Namespace:      pod.Namespace,
			DeploymentType: workloadRef.Kind,
			DeploymentName: workloadRef.Name,
			DeploymentUid:  workloadRef.UID,
		})
	}

	// Update spec and status
	bw.Spec.Requests = cleanRequests
	bw.Status.Used = networkingv1.Capacity{
		Local:   networkingv1.BandwidthDefinition{UlMbps: usedUlLocal, DlMbps: usedDlLocal},
		Network: networkingv1.BandwidthDefinition{UlMbps: usedUlNetwork, DlMbps: usedDlNetwork},
	}
	bw.Status.Remaining = networkingv1.Capacity{
		Local: networkingv1.BandwidthDefinition{
			UlMbps: bw.Spec.Capacity.Local.UlMbps - usedUlLocal,
			DlMbps: bw.Spec.Capacity.Local.DlMbps - usedDlLocal,
		},
		Network: networkingv1.BandwidthDefinition{
			UlMbps: bw.Spec.Capacity.Network.UlMbps - usedUlNetwork,
			DlMbps: bw.Spec.Capacity.Network.DlMbps - usedDlNetwork,
		},
	}
	bw.Status.Reservations = reservationInfo
	bw.Status.Capacity = bw.Spec.Capacity
	bw.Status.Status = networkingv1.Created
	bw.Status.ErrorReason = ""
	bw.Status.UpdatedAt = metav1.NewTime(time.Now())

	if err := r.Update(ctx, bw); err != nil {
		return err
	}
	if err := r.UpdateBandwidthStatus(ctx, bw); err != nil {
		return err
	}

	log.V(1).Info("single node bandwidth updated", "node", nodeName, "requests", len(cleanRequests))
	return nil
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
		bw.Status.UpdatedAt = metav1.NewTime(time.Now())
		_ = r.UpdateBandwidthStatus(ctx, bw)
		return ctrl.Result{}, err
	}

	// Enforce invariant: BW name == node name
	if !strings.EqualFold(bwName, node.Name) {
		bw.Status.Status = networkingv1.Error
		bw.Status.ErrorReason = "name mismatch between nodeName and bandwidthName"
		bw.Status.UpdatedAt = metav1.NewTime(time.Now())
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
		bw.Status.UpdatedAt = metav1.NewTime(time.Now())
		_ = r.UpdateBandwidthStatus(ctx, bw)
		return ctrl.Result{}, err
	}

	// Build map of pods that currently have bw annotations
	podsWithBw := make(map[string]corev1.Pod)
	for _, pod := range podList.Items {
		if pod.DeletionTimestamp != nil {
			log.V(1).Info("skipping pod marked for deletion", "pod", pod.Name, "namespace", pod.Namespace)
			continue
		}
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			//log.V(1).Info("skipping pod in terminal phase", "pod", pod.Name, "namespace", pod.Namespace)
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
	relatedNodesMaps := make(map[string]*networkingv1.Bandwidth)
	affectedRelatedNodes := make(map[string]bool) // Track which related nodes need updates
	wgSlice := []*sync.WaitGroup{}
	relatedNodesMutex := &sync.Mutex{}

	// Build map of existing requests for comparison
	existingReqs := make(map[string]networkingv1.BandwidthRequest)
	for _, req := range bw.Spec.Requests {
		existingReqs[req.PodUid] = req
	}

	// Cache sibling lookups per owner UID to avoid redundant API calls
	type siblingCacheEntry struct {
		siblings []corev1.Pod
		nodes    []string
		isLocal  bool
	}
	siblingCache := make(map[string]siblingCacheEntry)

	for _, req := range bwRequests {
		pod, exists := podsWithBw[req.PodUid]
		if !exists {
			// Pod gone or no bw annotation anymore => drop request
			// Check if this pod had siblings on other nodes - they need updates
			if oldReq, hadRequest := existingReqs[req.PodUid]; hadRequest {
				log.Info("dropping request for missing pod", "podUid", req.PodUid, "podName", req.PodName)
				// If pod was part of a workload with siblings, mark related nodes as affected
				// We need to fetch sibling info to know which nodes to notify
				// Use a best-effort approach: get pod from old request data if possible
				var oldPod corev1.Pod
				if err := r.Get(ctx, types.NamespacedName{Name: oldReq.PodName, Namespace: oldReq.Namespace}, &oldPod); err == nil {
					_, nodes, _ := r.allSiblingsOnCurrentNode(ctx, &oldPod, nodeName)
					for _, n := range nodes {
						if !strings.EqualFold(n, nodeName) {
							affectedRelatedNodes[n] = true
						}
					}
				}
			}
			continue
		}

		ul, dl := r.GetBandwidthFromAnnotation(pod)
		// local bandwidth is always used
		// since pods are always connected to virtual bridge
		usedUlLocal += ul
		usedDlLocal += dl

		var nUl, nDl int64
		linkLocal := false

		// Check if we already computed siblings for this workload
		workloadRef := GetWorkloadRefFromPod(&pod)
		ownerUID := workloadRef.UID

		var nodes []string
		var isLocal bool
		if cached, ok := siblingCache[ownerUID]; ok {
			// Use cached result
			nodes = cached.nodes
			isLocal = cached.isLocal
			log.V(2).Info("using cached sibling info", "pod", pod.Name, "ownerUID", ownerUID)
		} else {
			// Compute and cache sibling info
			siblings, nodeList, localResult := r.allSiblingsOnCurrentNode(ctx, &pod, nodeName)
			nodes = nodeList
			isLocal = localResult
			siblingCache[ownerUID] = siblingCacheEntry{
				siblings: siblings,
				nodes:    nodes,
				isLocal:  isLocal,
			}
			log.V(2).Info("computed and cached sibling info", "pod", pod.Name, "ownerUID", ownerUID, "siblingCount", len(siblings))
		}

		if !isLocal {
			nUl = ul
			nDl = dl
		} else {
			linkLocal = true
		}

		// Check if this specific request changed - if so, mark related nodes as affected
		newReq := networkingv1.BandwidthRequest{
			PodName:   pod.Name,
			Namespace: pod.Namespace,
			PodUid:    string(pod.UID),
			LinkLocal: linkLocal,
			Bandwidth: networkingv1.BandwidthDefinition{
				UlMbps: ul,
				DlMbps: dl,
			},
		}

		if oldReq, existed := existingReqs[req.PodUid]; !existed || oldReq != newReq {
			// This request is new or changed - mark related nodes as affected
			log.V(2).Info("request changed, marking related nodes as affected", "pod", pod.Name, "relatedNodes", len(nodes))
			for _, n := range nodes {
				if !strings.EqualFold(n, nodeName) {
					affectedRelatedNodes[n] = true
				}
			}
		}

		var wg sync.WaitGroup
		wg.Add(1)
		wgSlice = append(wgSlice, &wg)
		go func() {
			defer wg.Done()
			relatedNodesMutex.Lock()
			defer relatedNodesMutex.Unlock()
			for _, n := range nodes {
				if strings.EqualFold(n, nodeName) { // no need to notify current node
					continue
				}
				if _, found := relatedNodesMaps[n]; !found {
					var relatedBw networkingv1.Bandwidth
					if err := r.Get(ctx, types.NamespacedName{Name: n}, &relatedBw); err == nil {
						relatedNodesMaps[n] = &relatedBw
					}
				}
			}
		}()

		cleanRequests = append(cleanRequests, newReq)

		usedUlNetwork += nUl
		usedDlNetwork += nDl

		reservationInfo = append(reservationInfo, networkingv1.ReservationInfo{
			PodName: pod.Name,
			PodUid:  string(pod.UID),
			Bandwidth: networkingv1.Capacity{
				Local:   networkingv1.BandwidthDefinition{UlMbps: ul, DlMbps: dl},
				Network: networkingv1.BandwidthDefinition{UlMbps: nUl, DlMbps: nDl},
			},
			Namespace:      pod.Namespace,
			DeploymentType: workloadRef.Kind,
			DeploymentName: workloadRef.Name,
			DeploymentUid:  workloadRef.UID,
		})

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

	// Check if spec.requests changed by comparing as sets (order-independent)
	specChanged := (len(bw.Spec.Requests) != len(cleanRequests)) || (bw.Spec.Capacity != bw.Status.Capacity)

	if !specChanged {
		// Check if all clean requests match existing ones
		for _, newReq := range cleanRequests {
			oldReq, exists := existingReqs[newReq.PodUid]
			if !exists || oldReq != newReq {
				specChanged = true
				break
			}
		}
	}

	bw.Spec.Requests = cleanRequests
	// Check if status actually changed before updating
	statusChanged := bw.Status.Used != usedCapacity ||
		bw.Status.Remaining != remainingCapacity ||
		bw.Status.Capacity != bw.Spec.Capacity ||
		bw.Status.Status != networkingv1.Created ||
		bw.Status.ErrorReason != "" ||
		len(bw.Status.Reservations) != len(reservationInfo)

	bw.Status.Used = usedCapacity
	bw.Status.Remaining = remainingCapacity
	bw.Status.Reservations = reservationInfo
	bw.Status.Capacity = bw.Spec.Capacity
	bw.Status.Status = networkingv1.Created
	bw.Status.ErrorReason = ""

	// wait for related nodes fetching
	for _, wg := range wgSlice {
		wg.Wait()
	}

	// Update current node FIRST before notifying related nodes
	// This makes the current node's state consistent as fast as possible
	if specChanged {
		if err := r.Update(ctx, bw); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("bandwidth spec updated", "node", nodeName, "requests", len(cleanRequests))
	}

	// Only update status if it actually changed
	if statusChanged {
		bw.Status.UpdatedAt = metav1.NewTime(time.Now())
		if err := r.UpdateBandwidthStatus(ctx, bw); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("bandwidth status updated", "node", nodeName, "used_local_ul", usedUlLocal, "used_local_dl", usedDlLocal, "used_network_ul", usedUlNetwork, "used_network_dl", usedDlNetwork, "remaining_local_ul", remainingCapacity.Local.UlMbps, "remaining_local_dl", remainingCapacity.Local.DlMbps, "remaining_network_ul", remainingCapacity.Network.UlMbps, "remaining_network_dl", remainingCapacity.Network.DlMbps, "requests", len(cleanRequests))
	}

	// Sync ONLY the specific related nodes that are affected by request changes
	// This is more efficient than syncing all related nodes
	if len(affectedRelatedNodes) > 0 {
		// Use simple single-node sync function to avoid cascading updates
		go func() {
			for relatedNodeName := range affectedRelatedNodes {
				// Only sync if we have the bandwidth object for this node
				if relatedBw, found := relatedNodesMaps[relatedNodeName]; found {
					log.V(1).Info("syncing affected related node in background", "node", relatedNodeName)
					if err := r.syncSingleNodeBandwidth(context.Background(), relatedBw); err != nil {
						log.V(1).Info("failed to sync related node", "node", relatedNodeName, "error", err)
					} else {
						log.V(1).Info("synced related node", "node", relatedNodeName)
					}
				}
			}
		}()
		log.V(1).Info("triggered background sync for affected related nodes", "node", nodeName, "affectedCount", len(affectedRelatedNodes))
	} else if len(relatedNodesMaps) > 0 {
		log.V(2).Info("skipping related node sync (no affected nodes)", "node", nodeName, "totalRelatedNodes", len(relatedNodesMaps))
	}

	return ctrl.Result{}, nil
}
