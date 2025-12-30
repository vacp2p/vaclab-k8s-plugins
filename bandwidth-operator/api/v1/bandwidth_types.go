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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type BandwidthDefinition struct {
	// Uplink bandwidth in Mbps
	UlMbps int64 `json:"ulMbps"`
	// Downlink bandwidth in Mbps
	DlMbps int64 `json:"dlMbps"`
}
type BandwidthRequest struct {
	// pod name
	// +optional
	PodName string `json:"podName"`
	// namespace
	// +optional
	Namespace string `json:"namespace"`
	// pod uid
	PodUid string `json:"podUid"`
	// If the request is for local datapath
	// +optional
	LinkLocal bool `json:"linkLocal"`
	// requested bandwidth
	// +optional
	Bandwidth BandwidthDefinition `json:"bandwidth"`
}
type ReservationInfo struct {
	// pod name
	// +optional
	PodName string `json:"podName"`
	// namespace
	Namespace string `json:"namespace"`
	// pod uid
	PodUid string `json:"podUid"`
	// pod ip
	// +optional
	PodIp string `json:"ip"`
	// deployment type statefulset/job/deployment/others
	// +optional
	DeploymentType string `json:"deploymentType"`
	// deployment name
	// +optional
	DeploymentName string `json:"deploymentName"`
	// Deployment uid
	// +optional
	DeploymentUid string `json:"deploymentUid"`
	// reserved bandwidth
	Bandwidth Capacity `json:"bandwidth"`
}

type Capacity struct {
	// local datapath (ovs/linux-bridge) bandwidth within the host
	// +optional
	Local BandwidthDefinition `json:"local"`
	// network bandwidth (via physical ethernet interface)
	// +optional
	Network BandwidthDefinition `json:"network"`
}

// BandwidthSpec defines the desired state of Bandwidth
type BandwidthSpec struct {
	// The node this bandwidth resource applies to
	Node string `json:"node"`
	// The max capacity defined in this bandwidth resource
	Capacity Capacity `json:"capacity"`
	// List of Bandwidth requests
	// +optional
	Requests []BandwidthRequest `json:"requests,omitempty"`

	// The current status of this bandwidth resource
	// +optional
	Status Status `json:"status,omitempty"`
}

// +kubebuilder:validation:Enum=Created;Creating;Error;Deleting;Deleted;None
type Status string

const (
	Created  Status = "Created"
	Creating Status = "Creating"
	Deleted  Status = "Deleted"
	Deleting Status = "Deleting"
	Error    Status = "Error"
	None     Status = "None"
)

type EventVaclabNodeBandwidth string

const (
	EventVaclabNodeBandwidthCreated  EventVaclabNodeBandwidth = "vaclab_node_bandwidth_created"
	EventVaclabNodeBandwidthCreating EventVaclabNodeBandwidth = "vaclab_node_bandwidth_creating"
	EventVaclabNodeBandwidthDeleted  EventVaclabNodeBandwidth = "vaclab_node_bandwidth_deleted"
	EventVaclabNodeBandwidthDeleting EventVaclabNodeBandwidth = "vaclab_node_bandwidth_deleting"
	EventVaclabNodeBandwidthFailed   EventVaclabNodeBandwidth = "vaclab_node_bandwidth_failed"
)

// BandwidthStatus defines the observed state of Bandwidth.
type BandwidthStatus struct {
	// +optional
	// +kubebuilder:default="None"
	Status Status `json:"status,omitempty"`

	// +optional
	ErrorReason string `json:"errorReason"`

	// Consumed bandwidth
	// +optional
	Used Capacity `json:"used"`

	// Remaining bandwidth
	// +optional
	Remaining Capacity `json:"remaining"`

	// List of current reservations
	// +optional
	Reservations []ReservationInfo `json:"reservations"`

	// The max capacity defined in this bandwidth resource
	// +optional
	Capacity Capacity `json:"capacity"`

	// Last updated time
	UpdatedAt metav1.Time `json:"updatedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=".spec.node"
// +kubebuilder:printcolumn:name="UL",type=integer,JSONPath=".spec.capacity.network.ulMbps"
// +kubebuilder:printcolumn:name="DL",type=integer,JSONPath=".spec.capacity.network.dlMbps"
// +kubebuilder:printcolumn:name="uUL",type=integer,JSONPath=".status.used.network.ulMbps"
// +kubebuilder:printcolumn:name="uDL",type=integer,JSONPath=".status.used.network.dlMbps"
// +kubebuilder:printcolumn:name="LUL",type=integer,JSONPath=".spec.capacity.local.ulMbps"
// +kubebuilder:printcolumn:name="LDL",type=integer,JSONPath=".spec.capacity.local.dlMbps"
// +kubebuilder:printcolumn:name="uLUL",type=integer,JSONPath=".status.used.local.ulMbps"
// +kubebuilder:printcolumn:name="uLDL",type=integer,JSONPath=".status.used.local.dlMbps"
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"

// Bandwidth is the Schema for the bandwidths API
type Bandwidth struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Bandwidth
	// +required
	Spec BandwidthSpec `json:"spec"`

	// status defines the observed state of Bandwidth
	// +optional
	Status BandwidthStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// BandwidthList contains a list of Bandwidth
type BandwidthList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Bandwidth `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Bandwidth{}, &BandwidthList{})
}
