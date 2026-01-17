package controller

import (
	"context"
	"strconv"
	"strings"
	"time"

	networkingv1 "github.com/vacp2p/vaclab-k8s-plugins/bandwidth-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *BandwidthReconciler) generateEvent(event networkingv1.EventVaclabNodeBandwidth, bandwidthResource *networkingv1.Bandwidth) {
	var message string

	switch event {
	case networkingv1.EventVaclabNodeBandwidthCreated:
		message = "Created vaclab node bandwidth resource"
	case networkingv1.EventVaclabNodeBandwidthCreating:
		message = "Creating vaclab node bandwidth resource"
	case networkingv1.EventVaclabNodeBandwidthDeleted:
		message = "Deleted vaclab node bandwidth resource"
	case networkingv1.EventVaclabNodeBandwidthFailed:
		message = "Error on vaclab node bandwidth resource:" + bandwidthResource.Status.ErrorReason
	case networkingv1.EventVaclabNodeBandwidthDeleting:
		// should not be reached
		message = "Deleting vaclab node bandwidth resource"
	default:
		message = "Unknown event"
	}

	if event == networkingv1.EventVaclabNodeBandwidthDeleted {
		// should not be reached
		r.Recorder.Event(bandwidthResource, "Normal", string(event), "Deleted:"+message)
		return
	}

	r.Recorder.Event(bandwidthResource, "Normal", string(event), string(bandwidthResource.Status.Status)+":"+message+":"+bandwidthResource.Name)
}

func (r *BandwidthReconciler) setDefaultBandwidthValues(bandwidth *networkingv1.Bandwidth) {
	if bandwidth.Spec.Capacity.Local.UlMbps == 0 {
		bandwidth.Spec.Capacity.Local.UlMbps = r.Config.DefaultLocalUL
	}
	if bandwidth.Spec.Capacity.Local.DlMbps == 0 {
		bandwidth.Spec.Capacity.Local.DlMbps = r.Config.DefaultLocalDL
	}
	if bandwidth.Spec.Capacity.Network.UlMbps == 0 {
		bandwidth.Spec.Capacity.Network.UlMbps = r.Config.DefaultNetworkUL
	}
	if bandwidth.Spec.Capacity.Network.DlMbps == 0 {
		bandwidth.Spec.Capacity.Network.DlMbps = r.Config.DefaultNetworkDL
	}
}

func (r *BandwidthReconciler) UpdateBandwidthStatus(ctx context.Context, bandwidth *networkingv1.Bandwidth) error {
	// UpdatedAt is set by caller only when status actually changes
	err := r.Status().Update(ctx, bandwidth)
	// Return conflict errors so controller can requeue
	return err
}

func (r *BandwidthReconciler) GetBandwidthFromAnnotation(pod corev1.Pod) (ul int64, dl int64) {
	egress, _ := pod.Annotations[r.Config.EgressBandwidthAnnotation]
	ingress, _ := pod.Annotations[r.Config.IngressBandwidthAnnotation]

	// Parse egress bandwidth
	if egress != "" {
		// Try parsing as plain integer first (KubeOVN format: just "20" means 20 Mbps)
		if plainValue, err := strconv.Atoi(egress); err == nil {
			ul = int64(plainValue)
		} else if qEgress, err := resource.ParseQuantity(egress); err == nil {
			// Parse as Kubernetes Quantity (e.g., "100M", "1G")
			// Convert from bytes to Mbps
			ul = (qEgress.Value()) / 1000000
		}
	}

	// Parse ingress bandwidth
	if ingress != "" {
		// Try parsing as plain integer first (KubeOVN format: just "20" means 20 Mbps)
		if plainValue, err := strconv.Atoi(ingress); err == nil {
			dl = int64(plainValue)
		} else if qIngress, err := resource.ParseQuantity(ingress); err == nil {
			// Parse as Kubernetes Quantity (e.g., "100M", "1G")
			// Convert from bytes to Mbps
			dl = (qIngress.Value()) / 1000000
		}
	}

	return
}

// WorkloadRef is a small struct to transport workload metadata
type WorkloadRef struct {
	Kind string
	Name string
	UID  string
}

func GetWorkloadRefFromPod(pod *corev1.Pod) WorkloadRef {
	owners := pod.GetOwnerReferences()
	if len(owners) == 0 {
		// Pod created directly (no controller)
		return WorkloadRef{
			Kind: "Pod",
			Name: pod.GetName(),
			UID:  string(pod.GetUID()),
		}
	}

	// Prefer the controller owner if present
	for i := 0; i < len(owners); i++ {
		if owners[i].Controller != nil && *owners[i].Controller {
			return WorkloadRef{
				Kind: owners[i].Kind,
				Name: owners[i].Name,
				UID:  string(owners[i].UID),
			}
		}
	}

	// Fallback: first owner ref
	return WorkloadRef{
		Kind: owners[0].Kind,
		Name: owners[0].Name,
		UID:  string(owners[0].UID),
	}
}

func (r *BandwidthReconciler) createBandwidthResourcesForAllNodes(ctx context.Context) error {
	var nodes corev1.NodeList
	if err := r.List(ctx, &nodes); err != nil {
		return err
	}

	for _, n := range nodes.Items {
		nodeName := n.Name

		// If BW already exists, skip
		var existing networkingv1.Bandwidth
		if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, &existing); err == nil {
			continue
		} else if err != nil && !errors.IsNotFound(err) {
			return err
		}

		// Create BW for node
		bw := networkingv1.Bandwidth{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeName,
			},
			Spec: networkingv1.BandwidthSpec{
				Node: nodeName,
			},
		}

		r.setDefaultBandwidthValues(&bw)

		if err := r.Create(ctx, &bw); err != nil {
			// tolerate race
			if errors.IsAlreadyExists(err) {
				continue
			}
			return err
		}

		// Now update the status after creation with retry on conflict
		// Re-get the object to get the latest version
		maxRetries := 3
		for attempt := 0; attempt < maxRetries; attempt++ {
			if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, &bw); err != nil {
				if attempt == maxRetries-1 {
					// Give up on Get failures, let reconcile loop handle it
					break
				}
				time.Sleep(100 * time.Millisecond)
				continue
			}

			bw.Status.Status = networkingv1.Created
			bw.Status.Capacity = bw.Spec.Capacity
			bw.Status.UpdatedAt = metav1.NewTime(time.Now())

			if err := r.Status().Update(ctx, &bw); err != nil {
				if errors.IsConflict(err) && attempt < maxRetries-1 {
					// Retry on conflict
					time.Sleep(100 * time.Millisecond)
					continue
				}
				// On last attempt or non-conflict error, let reconcile loop handle it
				break
			}
			// Success
			r.generateEvent(networkingv1.EventVaclabNodeBandwidthCreated, &bw)
			break
		}
	}

	return nil
}

func (r *BandwidthReconciler) allSiblingsOnCurrentNode(ctx context.Context, pod *corev1.Pod, nodeName string) (siblingSlice []corev1.Pod, nodes []string, answer bool) {
	ownerRef := GetWorkloadRefFromPod(pod)

	// If pod has no owner (standalone pod), return only itself
	if strings.EqualFold(ownerRef.Kind, "Pod") || strings.EqualFold(ownerRef.Kind, "") {
		nodes = append(nodes, pod.Spec.NodeName)
		return
	}
	nodesMap := make(map[string]string)

	var podList corev1.PodList
	// List all pods in the same namespace
	if err := r.List(ctx, &podList, client.InNamespace(pod.Namespace)); err != nil {
		return
	}

	// Filter pods that have the same owner
	var siblingPods, localPods int
	for _, p := range podList.Items {
		// Check if this pod has the same owner
		pOwnerRef := GetWorkloadRefFromPod(&p)
		// creat list of nodes where siblings are running
		if strings.EqualFold(pOwnerRef.UID, ownerRef.UID) && !strings.EqualFold(p.Spec.NodeName, nodeName) {
			// sibling pod on another node
			// need to inform the node, so that it recomputes its bandwidth usage
			// by changing the corresponding BW specs and triggering a reconcile
			nodesMap[p.Spec.NodeName] = p.Spec.NodeName
		}
		// Skip pods that are being deleted
		if p.DeletionTimestamp != nil {
			continue
		}

		if strings.EqualFold(pOwnerRef.UID, ownerRef.UID) && strings.EqualFold(pOwnerRef.Kind, ownerRef.Kind) {
			if strings.EqualFold(p.Spec.NodeName, nodeName) {
				localPods++
			}
			siblingPods++
			siblingSlice = append(siblingSlice, p)
		}
	}
	if siblingPods > 0 && siblingPods == localPods {
		answer = true
	}

	return
}
