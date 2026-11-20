// Copyright 2025 NVIDIA CORPORATION & AFFILIATES
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/containernetworking/cni/pkg/skel"
)

// An L2-only ADD for a spec version whose result cannot hold it must fail before the
// plugin touches the namespace; later versions get past the check and fail on the
// missing namespace instead.
func TestCmdAddRejectsL2OnlyResultForOldSpecVersions(t *testing.T) {
	for _, tc := range []struct {
		version string
		want    string
	}{
		{"0.1.0", "without ipam is not supported"},
		{"0.2.0", "without ipam is not supported"},
		{"0.3.1", "failed to open netns"},
		{"1.0.0", "failed to open netns"},
	} {
		conf := fmt.Sprintf(`{"cniVersion": %q, "name": "ib", "type": "ipoib", "master": "ib0"}`, tc.version)
		err := cmdAdd(&skel.CmdArgs{
			ContainerID: "test",
			Netns:       "/nonexistent/netns",
			IfName:      "ib1",
			StdinData:   []byte(conf),
		})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("cniVersion %s: got error %v, want one containing %q", tc.version, err, tc.want)
		}
	}
}
