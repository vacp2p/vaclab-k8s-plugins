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

package validation

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/scheduler-plugins/apis/config"
)

// ValidateVaclabSchedulingArgs validates VaclabSchedulingArgs configuration
func ValidateVaclabSchedulingArgs(path *field.Path, args *config.VaclabSchedulingArgs) error {
	var allErrs field.ErrorList

	if args.EgressBandwidthAnnotation == "" {
		allErrs = append(allErrs, field.Required(path.Child("egressBandwidthAnnotation"), "must specify egress bandwidth annotation key"))
	}

	if args.IngressBandwidthAnnotation == "" {
		allErrs = append(allErrs, field.Required(path.Child("ingressBandwidthAnnotation"), "must specify ingress bandwidth annotation key"))
	}

	if args.DefaultEgressMbps < 0 {
		allErrs = append(allErrs, field.Invalid(path.Child("defaultEgressMbps"), args.DefaultEgressMbps, "must be non-negative"))
	}

	if args.DefaultIngressMbps < 0 {
		allErrs = append(allErrs, field.Invalid(path.Child("defaultIngressMbps"), args.DefaultIngressMbps, "must be non-negative"))
	}

	if len(allErrs) == 0 {
		return nil
	}
	return fmt.Errorf("%v", allErrs.ToAggregate())
}
