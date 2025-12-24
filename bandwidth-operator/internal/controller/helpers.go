package controller

import (
	"context"
	"fmt"
	"time"

	networkingv1 "github.com/vacp2p/vaclab-k8s-plugins/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
	// If conflict, return without error to allow retry
	if errors.IsConflict(err) {
		return nil
	}
	return err
}

func (r *BandwidthReconciler) GetBandwidthFromAnnotation(pod corev1.Pod) (ul int64, dl int64) {
	egress, _ := pod.Annotations[r.Config.EgressBandwidthAnnotation]
	ingress, _ := pod.Annotations[r.Config.IngressBandwidthAnnotation]

	// Parse egress bandwidth
	if egress != "" {
		if qEgress, err := resource.ParseQuantity(egress); err == nil {
			// Successfully parsed as Quantity (e.g., "100M", "1G")
			ul = qEgress.Value() / (1000000) // convert from bytes to megabits
		} else {
			// Failed to parse as Quantity, try parsing as plain number (e.g., KubeOVN format)
			// Plain numbers are interpreted as Mbps directly
			var plainValue int64
			if _, err := fmt.Sscanf(egress, "%d", &plainValue); err == nil {
				ul = plainValue
			}
		}
	}

	// Parse ingress bandwidth
	if ingress != "" {
		if qIngress, err := resource.ParseQuantity(ingress); err == nil {
			// Successfully parsed as Quantity (e.g., "100M", "1G")
			dl = qIngress.Value() / (1000000) // convert from bytes to megabits
		} else {
			// Failed to parse as Quantity, try parsing as plain number (e.g., KubeOVN format)
			// Plain numbers are interpreted as Mbps directly
			var plainValue int64
			if _, err := fmt.Sscanf(ingress, "%d", &plainValue); err == nil {
				dl = plainValue
			}
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

		// Now update the status after creation
		// Re-get the object to get the latest version
		if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, &bw); err != nil {
			return err
		}

		bw.Status.Status = networkingv1.Created
		bw.Status.Capacity = bw.Spec.Capacity
		bw.Status.UpdatedAt = metav1.NewTime(time.Now())

		if err := r.UpdateBandwidthStatus(ctx, &bw); err != nil {
			return err
		}

		// Generate event
		r.generateEvent(networkingv1.EventVaclabNodeBandwidthCreated, &bw)
	}

	return nil
}
