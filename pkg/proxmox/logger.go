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

package proxmox

import (
	"github.com/luthermonson/go-proxmox"
	"k8s.io/klog/v2"
)

var _ proxmox.LeveledLoggerInterface = Logger{}

// Logger implements go-proxmox.LeveledLoggerInterface and uses klog as log sink.
//
// Methods from the interface are mapped
//   - Errorf = Errorf
//   - Warnf  = V(0).Infof
//   - Infof  = V(2).Infof
//   - Debugf = V(4).Infof
type Logger struct{}

// Errorf logs message at error level.
func (Logger) Errorf(format string, args ...interface{}) {
	klog.Errorf(format, args...)
}

// Warnf logs message at warn level.
func (Logger) Warnf(format string, args ...interface{}) {
	klog.Infof(format, args...)
}

// Infof logs message at info level.
func (Logger) Infof(format string, args ...interface{}) {
	klog.V(2).Infof(format, args...)
}

// Debugf logs message at debug level.
func (Logger) Debugf(format string, args ...interface{}) {
	klog.V(4).Infof(format, args...)
}
