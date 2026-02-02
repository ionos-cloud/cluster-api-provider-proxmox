/*
Copyright 2020 The Kubernetes Authors.

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

// Package annotations contains a few things forked from cluster-api/util/annotations 1.10 to keep v1beta1 support
// see https://main.cluster-api.sigs.k8s.io/developer/providers/migrations/v1.10-to-v1.11#suggested-changes-for-clients-using-cluster-api-go-types :
// Additionally, in v1.11 all the CAPI utils like e.g. IsControlPlaneMachine are now using v1beta2 API types.
// This might lead to additional work for providers to keep using v1beta1 types from CAPI v1.11
// (you have to fork all utils the provider is using from an older CAPI release or replace them with something else).
package annotations

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
)

// IsPaused returns true if the Cluster is paused or the object has the `paused` annotation.
func IsPaused(cluster *clusterv1.Cluster, o metav1.Object) bool {
	if cluster.Spec.Paused {
		return true
	}
	return HasPaused(o)
}

// HasPaused returns true if the object has the `paused` annotation.
func HasPaused(o metav1.Object) bool {
	return hasAnnotation(o, clusterv1.PausedAnnotation)
}

// hasAnnotation returns true if the object has the specified annotation.
func hasAnnotation(o metav1.Object, annotation string) bool {
	annotations := o.GetAnnotations()
	if annotations == nil {
		return false
	}
	_, ok := annotations[annotation]
	return ok
}
