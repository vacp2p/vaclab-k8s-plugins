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

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VaclabSchedulingArgs defines the parameters for VacLab bandwidth-aware scheduling plugin.
type VaclabSchedulingArgs struct {
	metav1.TypeMeta

	// EgressBandwidthAnnotation is the annotation key for egress bandwidth
	// Default: "kubernetes.io/egress-bandwidth"
	// KubeOVN: "ovn.kubernetes.io/egress_rate"
	EgressBandwidthAnnotation string `json:"egressBandwidthAnnotation,omitempty"`

	// IngressBandwidthAnnotation is the annotation key for ingress bandwidth
	// Default: "kubernetes.io/ingress-bandwidth"
	// KubeOVN: "ovn.kubernetes.io/ingress_rate"
	IngressBandwidthAnnotation string `json:"ingressBandwidthAnnotation,omitempty"`

	// DefaultEgressMbps is the default egress bandwidth in Mbps when no annotation is found
	// Default: 0 (no default applied)
	DefaultEgressMbps int64 `json:"defaultEgressMbps,omitempty"`

	// DefaultIngressMbps is the default ingress bandwidth in Mbps when no annotation is found
	// Default: 0 (no default applied)
	DefaultIngressMbps int64 `json:"defaultIngressMbps,omitempty"`
}
