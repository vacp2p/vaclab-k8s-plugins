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

// SetDefaults_VaclabSchedulingArgs sets default values for VaclabSchedulingArgs
func SetDefaults_VaclabSchedulingArgs(args *VaclabSchedulingArgs) {
	if args.EgressBandwidthAnnotation == "" {
		args.EgressBandwidthAnnotation = "ovn.kubernetes.io/egress_rate"
	}
	if args.IngressBandwidthAnnotation == "" {
		args.IngressBandwidthAnnotation = "ovn.kubernetes.io/ingress_rate"
	}
	// DefaultEgressMbps and DefaultIngressMbps default to 50Mbps
	if args.DefaultEgressMbps == 0 {
		args.DefaultEgressMbps = 20
	}
	if args.DefaultIngressMbps == 0 {
		args.DefaultIngressMbps = 20
	}
}
