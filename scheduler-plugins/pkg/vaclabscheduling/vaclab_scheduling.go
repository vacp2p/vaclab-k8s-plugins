package vaclabscheduling

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	apiconfig "sigs.k8s.io/scheduler-plugins/apis/config"
	"sigs.k8s.io/scheduler-plugins/apis/config/validation"
)

type VaclabScheduling struct {
	bandwidth *BandwidthHandle
	handle    framework.Handle
}

// Name is the name of the plugin used in the Registry and configurations.
const Name = "VaclabScheduling"

var _ framework.PreFilterPlugin = &VaclabScheduling{}
var _ framework.FilterPlugin = &VaclabScheduling{}
var _ framework.ScorePlugin = &VaclabScheduling{}
var _ framework.ReservePlugin = &VaclabScheduling{}
var _ framework.PreBindPlugin = &VaclabScheduling{}
var _ framework.PostBindPlugin = &VaclabScheduling{}

// preFilterState stores computed data for the scheduling cycle
type preFilterState struct {
	ulMbps int64
	dlMbps int64
	//ulMbpsLocal int64
	//dlMbpsLocal int64
	ownerUID    string
	ownerKind   string
	hasWorkload bool
}

// Clone implements the StateData interface
func (s *preFilterState) Clone() framework.StateData {
	return &preFilterState{
		ulMbps:      s.ulMbps,
		dlMbps:      s.dlMbps,
		ownerUID:    s.ownerUID,
		ownerKind:   s.ownerKind,
		hasWorkload: s.hasWorkload,
		//ulMbpsLocal: s.ulMbpsLocal,
		//dlMbpsLocal: s.dlMbpsLocal,
	}
}

// stateKey is the key used to store preFilterState in CycleState
const stateKey = "VaclabScheduling"

// getPreFilterState retrieves the cached state
func getPreFilterState(state *framework.CycleState) (*preFilterState, error) {
	data, err := state.Read(stateKey)
	if err != nil {
		return nil, err
	}
	s, ok := data.(*preFilterState)
	if !ok {
		return nil, fmt.Errorf("invalid state type")
	}
	return s, nil
}

// Name returns name of the plugin.
func (v *VaclabScheduling) Name() string {
	return Name
}

// New initializes a new plugin and returns it.
func New(ctx context.Context, obj runtime.Object, h framework.Handle) (framework.Plugin, error) {
	args, ok := obj.(*apiconfig.VaclabSchedulingArgs)
	if !ok {
		return nil, fmt.Errorf("[VaclabScheduling] want args to be of type VaclabSchedulingArgs, got %T", obj)
	}

	if err := validation.ValidateVaclabSchedulingArgs(nil, args); err != nil {
		return nil, fmt.Errorf("[VaclabScheduling] invalid configuration: %w", err)
	}

	klog.InfoS("VaclabScheduling plugin initialized",
		"egressAnnotation", args.EgressBandwidthAnnotation,
		"ingressAnnotation", args.IngressBandwidthAnnotation,
		"defaultEgress", args.DefaultEgressMbps,
		"defaultIngress", args.DefaultIngressMbps)

	bandwidthConfig := BandwidthConfig{
		EgressBandwidthAnnotation:  args.EgressBandwidthAnnotation,
		IngressBandwidthAnnotation: args.IngressBandwidthAnnotation,
		DefaultEgressMbps:          args.DefaultEgressMbps,
		DefaultIngressMbps:         args.DefaultIngressMbps,
	}

	bandwidthHandle, err := NewBandwidthHandle(bandwidthConfig)
	if err != nil {
		return nil, fmt.Errorf("[VaclabScheduling] failed to create bandwidth handle: %w", err)
	}

	return &VaclabScheduling{
		handle:    h,
		bandwidth: bandwidthHandle,
	}, nil
}

// PreFilter extracts and caches pod bandwidth requirements and workload info
// Also checks if the request is feasible across any node in the cluster
func (v *VaclabScheduling) PreFilter(ctx context.Context, state *framework.CycleState, pod *v1.Pod) (*framework.PreFilterResult, *framework.Status) {
	// Extract bandwidth requirements once
	ulMbps, dlMbps, err := v.bandwidth.ExtractPodBandwidthRequirement(ctx, pod)
	if err != nil {
		klog.ErrorS(err, "Failed to extract bandwidth requirement in PreFilter", "pod", pod.Name)
		return nil, framework.NewStatus(framework.Error, fmt.Sprintf("failed to extract bandwidth: %v", err))
	}

	// Extract workload info (owner references)
	var ownerUID, ownerKind string
	hasWorkload := false
	ownerRefs := pod.GetOwnerReferences()
	if len(ownerRefs) > 0 {
		for _, owner := range ownerRefs {
			if owner.Controller != nil && *owner.Controller {
				ownerUID = string(owner.UID)
				ownerKind = owner.Kind
				hasWorkload = true
				break
			}
		}
	}

	// Check if request is feasible: is there at least one node with sufficient capacity?
	// This fails fast for pods that can't be scheduled anywhere
	if ulMbps > 0 || dlMbps > 0 {
		bandwidthList, err := v.bandwidth.ListBandwidths(ctx)
		if err != nil {
			klog.ErrorS(err, "Failed to list bandwidth resources in PreFilter", "pod", pod.Name)
			return nil, framework.NewStatus(framework.Error, fmt.Sprintf("failed to list bandwidth resources: %v", err))
		}

		feasible := false
		for _, bw := range bandwidthList.Items {
			if bw.Status.Remaining.Network.UlMbps >= ulMbps &&
				bw.Status.Remaining.Network.DlMbps >= dlMbps &&
				bw.Status.Remaining.Local.UlMbps >= ulMbps && bw.Status.Remaining.Local.DlMbps >= dlMbps {
				feasible = true
				break
			}
		}

		if !feasible {
			klog.InfoS("Pod is unschedulable: no node has sufficient bandwidth",
				"pod", pod.Name, "namespace", pod.Namespace,
				"requiredUL", ulMbps, "requiredDL", dlMbps)
			return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable,
				fmt.Sprintf("no node has sufficient bandwidth (required: ul=%dMbps, dl=%dMbps)", ulMbps, dlMbps))
		}
	}

	// Store in cycle state for reuse in Filter and Score
	s := &preFilterState{
		ulMbps:      ulMbps,
		dlMbps:      dlMbps,
		ownerUID:    ownerUID,
		ownerKind:   ownerKind,
		hasWorkload: hasWorkload,
	}
	state.Write(stateKey, s)

	klog.V(4).InfoS("PreFilter cached pod info", "pod", pod.Name,
		"ul", ulMbps, "dl", dlMbps, "workload", ownerKind, "hasWorkload", hasWorkload)

	return nil, framework.NewStatus(framework.Success, "")
} // PreFilterExtensions returns nil as we don't need extensions
func (v *VaclabScheduling) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

// Filter checks if a node has sufficient bandwidth for the pod
// If not, it returns Unschedulable status
func (v *VaclabScheduling) Filter(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	nodeName := nodeInfo.Node().Name

	// Get cached bandwidth requirements from PreFilter
	s, err := getPreFilterState(state)
	if err != nil {
		klog.ErrorS(err, "Failed to get prefilter state", "pod", pod.Name)
		return framework.NewStatus(framework.Error, "failed to get cached pod info")
	}

	// Check if node has sufficient bandwidth
	sufficient, err := v.bandwidth.HasSufficientBandwidth(ctx, nodeName, s.ulMbps, s.dlMbps)
	if err != nil {
		klog.ErrorS(err, "Failed to check bandwidth availability", "node", nodeName, "pod", pod.Name)
		return framework.NewStatus(framework.Error, fmt.Sprintf("failed to check bandwidth: %v", err))
	}

	if !sufficient {
		return framework.NewStatus(framework.Unschedulable,
			fmt.Sprintf("node %s has insufficient bandwidth (required: ul=%dMbps, dl=%dMbps)", nodeName, s.ulMbps, s.dlMbps))
	}

	return framework.NewStatus(framework.Success, "")
}

// Score ranks nodes based on available bandwidth (more available = higher score) and pod locality
func (v *VaclabScheduling) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {

	// Get cached bandwidth requirements and workload info from PreFilter
	s, err := getPreFilterState(state)
	if err != nil {
		klog.ErrorS(err, "Failed to get prefilter state", "pod", pod.Name)
		return 0, framework.NewStatus(framework.Error, "failed to get cached pod info")
	}

	// Get available bandwidth for the node
	remaining, err := v.bandwidth.GetAvailableBandwidth(ctx, nodeName)
	if err != nil {
		klog.V(4).ErrorS(err, "Failed to get available bandwidth", "node", nodeName)
		return 0, framework.NewStatus(framework.Error, fmt.Sprintf("failed to get bandwidth: %v", err))
	}

	// Calculate bandwidth score based on remaining capacity after allocation
	// Use network bandwidth as the bottleneck (minimum of UL/DL)
	remainingUL := remaining.Network.UlMbps - s.ulMbps
	remainingDL := remaining.Network.DlMbps - s.dlMbps

	var bandwidthScore int64
	if remainingUL < remainingDL {
		bandwidthScore = remainingUL
	} else {
		bandwidthScore = remainingDL
	}

	// Ensure non-negative
	if bandwidthScore < 0 {
		bandwidthScore = 0
	}

	// Calculate locality score (higher is better for co-location)
	var localityScore int64
	if s.hasWorkload {
		localityScore, err = v.bandwidth.CalculateLocalityScore(ctx, pod, nodeName)
		if err != nil {
			klog.V(4).ErrorS(err, "Failed to calculate locality score", "node", nodeName, "pod", pod.Name)
			// Don't fail scoring, just skip locality component
			localityScore = 0
		}
	}

	// Combine scores with locality priority (70% locality + 30% bandwidth)
	// Multiply by weights: locality gets 7x weight, bandwidth gets 3x weight
	totalScore := (localityScore * 7) + (bandwidthScore * 3)

	klog.V(5).InfoS("Scored node", "node", nodeName, "pod", pod.Name,
		"bandwidthScore", bandwidthScore, "localityScore", localityScore, "totalScore", totalScore)
	return totalScore, framework.NewStatus(framework.Success, "")
}

// ScoreExtensions returns the score extensions for normalization
func (v *VaclabScheduling) ScoreExtensions() framework.ScoreExtensions {
	return v
}

// NormalizeScore normalizes the scores across all nodes
func (v *VaclabScheduling) NormalizeScore(ctx context.Context, state *framework.CycleState, pod *v1.Pod, scores framework.NodeScoreList) *framework.Status {
	// Find the highest score
	var highestScore int64
	for _, node := range scores {
		if highestScore < node.Score {
			highestScore = node.Score
		}
	}

	// Avoid division by zero
	if highestScore == 0 {
		// All nodes have score 0, assign minimum score to all
		for i := range scores {
			scores[i].Score = 0
		}
		return framework.NewStatus(framework.Success, "")
	}

	// Normalize scores to 0-100 range (framework.MaxNodeScore)
	// Higher raw score = higher normalized score (best node gets 100)
	for i, node := range scores {
		scores[i].Score = node.Score * framework.MaxNodeScore / highestScore
	}

	klog.V(4).InfoS("Normalized node scores", "pod", pod.Name, "scores", scores)
	return framework.NewStatus(framework.Success, "")
}

// Reserve updates the bandwidth CRD to reserve bandwidth for the pod
// This happens before binding to avoid race conditions
func (v *VaclabScheduling) Reserve(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) *framework.Status {
	// Get cached bandwidth requirements from PreFilter
	s, err := getPreFilterState(state)
	if err != nil {
		klog.ErrorS(err, "Failed to get prefilter state", "pod", pod.Name)
		return framework.NewStatus(framework.Error, "failed to get cached pod info")
	}

	// Skip if no bandwidth required
	if s.ulMbps == 0 && s.dlMbps == 0 {
		return framework.NewStatus(framework.Success, "")
	}

	// Update CRD to reserve bandwidth (before binding)
	if err := v.bandwidth.AddBandwidthRequest(ctx, nodeName, pod.Name, pod.Namespace, string(pod.UID), s.ulMbps, s.dlMbps); err != nil {
		klog.ErrorS(err, "Failed to reserve bandwidth in CRD", "node", nodeName, "pod", pod.Name)
		return framework.NewStatus(framework.Error, fmt.Sprintf("failed to reserve bandwidth: %v", err))
	}

	klog.InfoS("Reserved bandwidth in CRD", "node", nodeName, "pod", pod.Name, "ul", s.ulMbps, "dl", s.dlMbps)
	return framework.NewStatus(framework.Success, "")
}

// Unreserve releases the bandwidth reservation if binding fails
// This ensures we clean up CRD modifications made in Reserve
func (v *VaclabScheduling) Unreserve(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) {
	// Get cached bandwidth requirements from PreFilter
	s, err := getPreFilterState(state)
	if err != nil {
		klog.ErrorS(err, "Failed to get prefilter state in Unreserve", "pod", pod.Name)
		return
	}

	// Skip if no bandwidth was reserved
	if s.ulMbps == 0 && s.dlMbps == 0 {
		return
	}

	// Remove bandwidth request from CRD (cleanup after failed binding)
	if err := v.bandwidth.RemoveBandwidthRequest(ctx, nodeName, string(pod.UID)); err != nil {
		klog.ErrorS(err, "Failed to unreserve bandwidth in CRD", "node", nodeName, "pod", pod.Name)
		return
	}

	klog.InfoS("Unreserved bandwidth in CRD", "node", nodeName, "pod", pod.Name, "ul", s.ulMbps, "dl", s.dlMbps)
}

// PreBind is called before binding the pod
// Bandwidth was already reserved in Reserve phase, so this just logs
func (v *VaclabScheduling) PreBind(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) *framework.Status {
	// Get cached bandwidth requirements from PreFilter
	s, err := getPreFilterState(state)
	if err != nil {
		klog.ErrorS(err, "Failed to get prefilter state in PreBind", "pod", pod.Name)
		// Don't fail binding for state read errors
		return framework.NewStatus(framework.Success, "")
	}

	// Skip if no bandwidth required
	if s.ulMbps == 0 && s.dlMbps == 0 {
		return framework.NewStatus(framework.Success, "")
	}

	// Bandwidth was already validated and reserved in Reserve phase
	klog.V(3).InfoS("PreBind: bandwidth already reserved", "node", nodeName, "pod", pod.Name, "ul", s.ulMbps, "dl", s.dlMbps)
	return framework.NewStatus(framework.Success, "")
}

// PostBind is called after pod is successfully bound
// Bandwidth was already reserved in Reserve phase, so this just logs success
func (v *VaclabScheduling) PostBind(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) {
	// Get cached bandwidth requirements from PreFilter
	s, err := getPreFilterState(state)
	if err != nil {
		klog.ErrorS(err, "Failed to get prefilter state in PostBind", "pod", pod.Name)
		return
	}

	// Skip if no bandwidth required
	if s.ulMbps == 0 && s.dlMbps == 0 {
		return
	}

	// Bandwidth was already reserved in Reserve phase, just log success
	klog.InfoS("Pod successfully bound with bandwidth reservation", "node", nodeName, "pod", pod.Name, "ul", s.ulMbps, "dl", s.dlMbps)
}
