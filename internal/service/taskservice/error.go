/*
Copyright 2023 IONOS Cloud.

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

package taskservice

import (
	"fmt"
	"time"
)

// RequeueError signals that a certain time must pass before continuing reconciliation.
//
// The duration can be retrieved with RequeueAfter.
type RequeueError struct {
	error        string
	requeueAfter time.Duration
}

func (r *RequeueError) Error() string {
	return fmt.Sprintf("%s, requeuing after %s", r.error, r.requeueAfter.String())
}

// RequeueAfter returns the duration after which the controller will requeue the object.
func (r *RequeueError) RequeueAfter() time.Duration {
	return r.requeueAfter
}

// NewRequeueError returns an error of type RequeueError.
func NewRequeueError(msg string, d time.Duration) error {
	return &RequeueError{error: msg, requeueAfter: d}
}
