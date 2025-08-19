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

package cloudinit

import (
	"bytes"
	"net/netip"
	"text/template"

	"github.com/pkg/errors"
)

func is6(addr string) bool {
	return netip.MustParsePrefix(addr).Addr().Is6()
}

func render(name string, tpl string, data BaseCloudInitData) ([]byte, error) {
	f := map[string]any{"is6": is6}
	mt, err := template.New(name).Funcs(f).Parse(tpl)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse %s template", name)
	}

	buffer := &bytes.Buffer{}
	if err = mt.Execute(buffer, data); err != nil {
		return nil, errors.Wrapf(err, "failed to render %s", name)
	}
	return buffer.Bytes(), nil
}
