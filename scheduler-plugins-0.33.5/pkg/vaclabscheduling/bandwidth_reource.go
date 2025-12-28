package vaclabscheduling

import (
	"context"
	"fmt"
	"strconv"

	//networkingv1 "sigs.k8s.io/scheduler-plugins/api/v1"
	networkingv1 "github.com/vacp2p/vaclab-k8s-plugins/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// BandwidthConfig holds the CNI-specific bandwidth annotation keys
type BandwidthConfig struct {
	EgressBandwidthAnnotation  string
	IngressBandwidthAnnotation string
	DefaultEgressMbps          int64
	DefaultIngressMbps         int64
}

// Common bandwidth annotation keys
const (
	// Default Kubernetes bandwidth annotations
	DefaultEgressBandwidthAnnotation  = "kubernetes.io/egress-bandwidth"
	DefaultIngressBandwidthAnnotation = "kubernetes.io/ingress-bandwidth"

	// KubeOVN bandwidth annotations
	KubeOVNEgressBandwidthAnnotation  = "ovn.kubernetes.io/egress_rate"
	KubeOVNIngressBandwidthAnnotation = "ovn.kubernetes.io/ingress_rate"

	// Calico bandwidth annotations
	CalicoEgressBandwidthAnnotation  = "kubernetes.io/egress-bandwidth"
	CalicoIngressBandwidthAnnotation = "kubernetes.io/ingress-bandwidth"
)

// BandwidthHandle handles interactions with the vaclab Bandwidth CRD
type BandwidthHandle struct {
	client client.Client
	config BandwidthConfig
}

// NewBandwidthHandle creates a new handle for interacting with Bandwidth resources
func NewBandwidthHandle(cfg BandwidthConfig) (*BandwidthHandle, error) {
	// Get kubeconfig
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %v", err)
	}

	// Create a new scheme and register the Bandwidth CRD
	s := runtime.NewScheme()
	if err := scheme.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("failed to add core scheme: %v", err)
	}
	if err := networkingv1.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("failed to add bandwidth scheme: %v", err)
	}

	// Create the client
	c, err := client.New(kubeConfig, client.Options{Scheme: s})
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %v", err)
	}

	return &BandwidthHandle{
		client: c,
		config: cfg,
	}, nil
}

// GetNodeBandwidth retrieves the Bandwidth resource for a specific node
func (h *BandwidthHandle) GetNodeBandwidth(ctx context.Context, nodeName string) (*networkingv1.Bandwidth, error) {
	var bandwidth networkingv1.Bandwidth
	if err := h.client.Get(ctx, types.NamespacedName{Name: nodeName}, &bandwidth); err != nil {
		return nil, fmt.Errorf("failed to get bandwidth for node %s: %v", nodeName, err)
	}
	return &bandwidth, nil
}

// ListBandwidths lists all Bandwidth resources
func (h *BandwidthHandle) ListBandwidths(ctx context.Context) (*networkingv1.BandwidthList, error) {
	var bandwidthList networkingv1.BandwidthList
	if err := h.client.List(ctx, &bandwidthList); err != nil {
		return nil, fmt.Errorf("failed to list bandwidth resources: %v", err)
	}
	return &bandwidthList, nil
}

// GetAvailableBandwidth returns the remaining capacity for a node
func (h *BandwidthHandle) GetAvailableBandwidth(ctx context.Context, nodeName string) (*networkingv1.Capacity, error) {
	bw, err := h.GetNodeBandwidth(ctx, nodeName)
	if err != nil {
		return nil, err
	}

	if bw.Status.Status != networkingv1.Created {
		return nil, fmt.Errorf("bandwidth resource for node %s is not in Created state: %s", nodeName, bw.Status.Status)
	}

	return &bw.Status.Remaining, nil
}

// AddBandwidthRequest adds a bandwidth request to the node's Bandwidth resource
func (h *BandwidthHandle) AddBandwidthRequest(ctx context.Context, nodeName, podName, namespace, podUID string, ulMbps, dlMbps int64) error {
	var bandwidth networkingv1.Bandwidth
	if err := h.client.Get(ctx, types.NamespacedName{Name: nodeName}, &bandwidth); err != nil {
		return fmt.Errorf("failed to get bandwidth for node %s: %v", nodeName, err)
	}

	// Filter new request from existing ones
	newRequestSlice := []networkingv1.BandwidthRequest{}
	for _, req := range bandwidth.Spec.Requests {
		if req.PodUid != podUID {
			newRequestSlice = append(newRequestSlice, req)
		}
	}

	// Create new request
	newRequest := networkingv1.BandwidthRequest{
		PodName:   podName,
		Namespace: namespace,
		PodUid:    podUID,
		Bandwidth: networkingv1.BandwidthDefinition{
			UlMbps: ulMbps,
			DlMbps: dlMbps,
		},
	}

	// Add request to spec
	newRequestSlice = append(newRequestSlice, newRequest)
	bandwidth.Spec.Requests = newRequestSlice

	// Update the resource
	if err := h.client.Update(ctx, &bandwidth); err != nil {
		return fmt.Errorf("failed to update bandwidth for node %s: %v", nodeName, err)
	}

	klog.V(2).InfoS("Added bandwidth request", "node", nodeName, "pod", podName, "ul", ulMbps, "dl", dlMbps)
	return nil
}

// RemoveBandwidthRequest removes a bandwidth request from the node's Bandwidth resource
func (h *BandwidthHandle) RemoveBandwidthRequest(ctx context.Context, nodeName, podUID string) error {
	var bandwidth networkingv1.Bandwidth
	if err := h.client.Get(ctx, types.NamespacedName{Name: nodeName}, &bandwidth); err != nil {
		return fmt.Errorf("failed to get bandwidth for node %s: %v", nodeName, err)
	}

	// Find and remove the request
	found := false
	newRequests := make([]networkingv1.BandwidthRequest, 0, len(bandwidth.Spec.Requests))
	for _, req := range bandwidth.Spec.Requests {
		if req.PodUid != podUID {
			newRequests = append(newRequests, req)
		} else {
			found = true
		}
	}

	if !found {
		klog.V(4).InfoS("Bandwidth request not found for pod", "uid", podUID, "node", nodeName)
		return nil
	}

	bandwidth.Spec.Requests = newRequests

	// Update the resource
	if err := h.client.Update(ctx, &bandwidth); err != nil {
		return fmt.Errorf("failed to update bandwidth for node %s: %v", nodeName, err)
	}

	klog.V(2).InfoS("Removed bandwidth request", "node", nodeName, "podUID", podUID)
	return nil
}

// HasSufficientBandwidth checks if a node has sufficient bandwidth for the requested amounts
func (h *BandwidthHandle) HasSufficientBandwidth(ctx context.Context, nodeName string, ulMbps, dlMbps int64) (bool, error) {
	remaining, err := h.GetAvailableBandwidth(ctx, nodeName)
	if err != nil {
		return false, err
	}

	if remaining.Local.UlMbps < ulMbps || remaining.Local.DlMbps < dlMbps {
		klog.V(4).InfoS("Insufficient virtual bridge bandwidth", "node", nodeName,
			"requiredUL", ulMbps, "availableUL", remaining.Local.UlMbps,
			"requiredDL", dlMbps, "availableDL", remaining.Local.DlMbps)
		return false, nil
	}

	if remaining.Network.UlMbps < ulMbps || remaining.Network.DlMbps < dlMbps {
		klog.V(4).InfoS("Insufficient network bandwidth", "node", nodeName,
			"requiredUL", ulMbps, "availableUL", remaining.Network.UlMbps,
			"requiredDL", dlMbps, "availableDL", remaining.Network.DlMbps)
		return false, nil
	}

	return true, nil
}

// CalculateLocalityScore calculates a locality score for pod placement
// Returns 0-50 based on workload affinity (siblings already on the node)
func (h *BandwidthHandle) CalculateLocalityScore(ctx context.Context, pod *corev1.Pod, nodeName string) (int64, error) {
	// Get workload reference (owner controller)
	ownerRefs := pod.GetOwnerReferences()
	if len(ownerRefs) == 0 {
		// Standalone pod - no locality preference
		return 0, nil
	}

	// Find the controller owner
	var ownerUID string
	var ownerKind string
	for _, owner := range ownerRefs {
		if owner.Controller != nil && *owner.Controller {
			ownerUID = string(owner.UID)
			ownerKind = owner.Kind
			break
		}
	}

	if ownerUID == "" {
		// No controller owner
		return 0, nil
	}

	// Get the bandwidth resource for the node
	bw, err := h.GetNodeBandwidth(ctx, nodeName)
	if err != nil {
		return 0, err
	}

	// Count how many pods from the same workload are already on this node
	siblingCount := 0
	for _, reservation := range bw.Status.Reservations {
		if reservation.DeploymentUid == ownerUID {
			siblingCount++
		}
	}

	// Calculate locality score based on actual sibling count
	// Higher sibling count = higher score (better co-location)
	// Use raw sibling count so normalization will favor nodes with more siblings
	localityScore := int64(siblingCount)

	klog.V(5).InfoS("Calculated locality score", "node", nodeName, "pod", pod.Name,
		"workload", ownerKind, "workloadUID", ownerUID, "siblings", siblingCount, "score", localityScore)

	return localityScore, nil
}

// UpdateBandwidthCapacity updates the capacity of a node's Bandwidth resource
func (h *BandwidthHandle) UpdateBandwidthCapacity(ctx context.Context, nodeName string, capacity networkingv1.Capacity) error {
	var bandwidth networkingv1.Bandwidth
	if err := h.client.Get(ctx, types.NamespacedName{Name: nodeName}, &bandwidth); err != nil {
		return fmt.Errorf("failed to get bandwidth for node %s: %v", nodeName, err)
	}

	bandwidth.Spec.Capacity = capacity

	if err := h.client.Update(ctx, &bandwidth); err != nil {
		return fmt.Errorf("failed to update bandwidth capacity for node %s: %v", nodeName, err)
	}

	klog.V(2).InfoS("Updated bandwidth capacity", "node", nodeName,
		"localUL", capacity.Local.UlMbps, "localDL", capacity.Local.DlMbps,
		"networkUL", capacity.Network.UlMbps, "networkDL", capacity.Network.DlMbps)
	return nil
}

// GetBandwidthStatus returns the current status of a node's bandwidth
func (h *BandwidthHandle) GetBandwidthStatus(ctx context.Context, nodeName string) (*networkingv1.BandwidthStatus, error) {
	bw, err := h.GetNodeBandwidth(ctx, nodeName)
	if err != nil {
		return nil, err
	}
	return &bw.Status, nil
}

// GetBandwidthFromPodAnnotations extracts bandwidth requirements from pod annotations
// Returns ulMbps, dlMbps, and a boolean indicating if bandwidth annotations were found
func (h *BandwidthHandle) GetBandwidthFromPodAnnotations(pod *corev1.Pod) (ulMbps, dlMbps int64, found bool) {
	if pod.Annotations == nil {
		return 0, 0, false
	}

	egress, hasEgress := pod.Annotations[h.config.EgressBandwidthAnnotation]
	ingress, hasIngress := pod.Annotations[h.config.IngressBandwidthAnnotation]

	if !hasEgress && !hasIngress {
		return 0, 0, false
	}

	// Parse egress bandwidth
	if egress != "" {
		ulMbps = parseBandwidthValue(egress)
	}

	// Parse ingress bandwidth
	if ingress != "" {
		dlMbps = parseBandwidthValue(ingress)
	}

	return ulMbps, dlMbps, true
}

// parseBandwidthValue parses bandwidth from annotation value
// Supports plain integers (KubeOVN: "20" = 20 Mbps) and Kubernetes Quantities ("100M")
func parseBandwidthValue(value string) int64 {
	// Try parsing as plain integer first (KubeOVN format: just "20" means 20 Mbps)
	if plainValue, err := strconv.Atoi(value); err == nil {
		return int64(plainValue)
	}

	// Try parsing as Kubernetes Quantity (e.g., "100M", "1G")
	if quantity, err := resource.ParseQuantity(value); err == nil {
		// Convert from bytes to Mbps
		return quantity.Value() / 1000000
	}

	return 0
}

// NormalizePodBandwidthAnnotations checks if pod has bandwidth annotations and normalizes them
// to the expected CNI format. Returns true if annotations were updated.
func (h *BandwidthHandle) NormalizePodBandwidthAnnotations(ctx context.Context, pod *corev1.Pod) (bool, error) {
	if pod.Annotations == nil {
		return false, nil
	}

	updated := false
	ulMbps, dlMbps := int64(0), int64(0)

	// Check all known bandwidth annotation keys
	knownAnnotations := []struct {
		egress  string
		ingress string
	}{
		{DefaultEgressBandwidthAnnotation, DefaultIngressBandwidthAnnotation},
		{KubeOVNEgressBandwidthAnnotation, KubeOVNIngressBandwidthAnnotation},
		{CalicoEgressBandwidthAnnotation, CalicoIngressBandwidthAnnotation},
	}

	// Find bandwidth values from any known annotation format
	for _, annot := range knownAnnotations {
		if egress, ok := pod.Annotations[annot.egress]; ok && egress != "" {
			if val := parseBandwidthValue(egress); val > 0 {
				ulMbps = val
				// Remove non-standard annotation if found
				if annot.egress != h.config.EgressBandwidthAnnotation {
					delete(pod.Annotations, annot.egress)
					updated = true
				}
			}
		}
		if ingress, ok := pod.Annotations[annot.ingress]; ok && ingress != "" {
			if val := parseBandwidthValue(ingress); val > 0 {
				dlMbps = val
				// Remove non-standard annotation if found
				if annot.ingress != h.config.IngressBandwidthAnnotation {
					delete(pod.Annotations, annot.ingress)
					updated = true
				}
			}
		}
	}

	// If bandwidth was found but not in the correct format, add the correct annotations
	if ulMbps > 0 {
		expectedEgress := formatBandwidthValue(ulMbps, h.config.EgressBandwidthAnnotation)
		if pod.Annotations[h.config.EgressBandwidthAnnotation] != expectedEgress {
			pod.Annotations[h.config.EgressBandwidthAnnotation] = expectedEgress
			updated = true
		}
	}

	if dlMbps > 0 {
		expectedIngress := formatBandwidthValue(dlMbps, h.config.IngressBandwidthAnnotation)
		if pod.Annotations[h.config.IngressBandwidthAnnotation] != expectedIngress {
			pod.Annotations[h.config.IngressBandwidthAnnotation] = expectedIngress
			updated = true
		}
	}

	// Update pod if annotations were modified
	if updated {
		if err := h.client.Update(ctx, pod); err != nil {
			return false, fmt.Errorf("failed to update pod annotations: %v", err)
		}
		klog.V(2).InfoS("Normalized pod bandwidth annotations", "pod", pod.Name, "namespace", pod.Namespace,
			"ulMbps", ulMbps, "dlMbps", dlMbps, "annotation", h.config.EgressBandwidthAnnotation)
	}

	return updated, nil
}

// formatBandwidthValue formats bandwidth value according to CNI format
func formatBandwidthValue(mbps int64, annotationKey string) string {
	// KubeOVN uses plain integers (e.g., "20")
	if annotationKey == KubeOVNEgressBandwidthAnnotation || annotationKey == KubeOVNIngressBandwidthAnnotation {
		return strconv.FormatInt(mbps, 10)
	}

	// Default Kubernetes annotations use Quantity format (e.g., "20M")
	return fmt.Sprintf("%dM", mbps)
}

// ExtractPodBandwidthRequirement extracts and normalizes bandwidth from pod, ready for scheduling
// If annotations are missing, it applies defaults and updates the pod spec in-memory
// The updated pod will be persisted when the scheduler binds it to a node
func (h *BandwidthHandle) ExtractPodBandwidthRequirement(ctx context.Context, pod *corev1.Pod) (ulMbps, dlMbps int64, err error) {
	// Extract bandwidth requirements from existing annotations
	ulMbps, dlMbps, found := h.GetBandwidthFromPodAnnotations(pod)

	// Initialize annotations map if needed
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}

	// Apply defaults if annotations are missing
	if !found {
		// No bandwidth annotations at all, use both defaults
		ulMbps = h.config.DefaultEgressMbps
		dlMbps = h.config.DefaultIngressMbps

		if ulMbps > 0 {
			pod.Annotations[h.config.EgressBandwidthAnnotation] = formatBandwidthValue(ulMbps, h.config.EgressBandwidthAnnotation)
		}
		if dlMbps > 0 {
			pod.Annotations[h.config.IngressBandwidthAnnotation] = formatBandwidthValue(dlMbps, h.config.IngressBandwidthAnnotation)
		}

		if ulMbps > 0 || dlMbps > 0 {
			klog.V(3).InfoS("Applied default bandwidth annotations to pod spec", "pod", pod.Name, "namespace", pod.Namespace,
				"ulMbps", ulMbps, "dlMbps", dlMbps)
		}
	} else {
		// Check if only one annotation is provided and apply default for the other
		if ulMbps == 0 && h.config.DefaultEgressMbps > 0 {
			ulMbps = h.config.DefaultEgressMbps
			pod.Annotations[h.config.EgressBandwidthAnnotation] = formatBandwidthValue(ulMbps, h.config.EgressBandwidthAnnotation)
			klog.V(3).InfoS("Applied default egress bandwidth annotation to pod spec", "pod", pod.Name, "namespace", pod.Namespace,
				"ulMbps", ulMbps)
		}

		if dlMbps == 0 && h.config.DefaultIngressMbps > 0 {
			dlMbps = h.config.DefaultIngressMbps
			pod.Annotations[h.config.IngressBandwidthAnnotation] = formatBandwidthValue(dlMbps, h.config.IngressBandwidthAnnotation)
			klog.V(3).InfoS("Applied default ingress bandwidth annotation to pod spec", "pod", pod.Name, "namespace", pod.Namespace,
				"dlMbps", dlMbps)
		}

		klog.V(4).InfoS("Extracted bandwidth requirement from pod", "pod", pod.Name, "namespace", pod.Namespace,
			"ulMbps", ulMbps, "dlMbps", dlMbps)
	}

	return ulMbps, dlMbps, nil
}
