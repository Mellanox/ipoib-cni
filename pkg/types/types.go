/*
2022 NVIDIA CORPORATION & AFFILIATES

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

package types

import (
	"github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
)

// NetConf extends cni NetConf
type NetConf struct {
	types.NetConf
	Master string `json:"master"`
}

// Manager provides interface invoke ipoib nic related operations
type Manager interface {
	CreateIpoibLink(conf *NetConf, ifName string, netns ns.NetNS) (*current.Interface, error)
	RemoveIpoibLink(ifName string, netns ns.NetNS) error
}

// NetlinkManager is an interface to mock nelink library
type NetlinkManager interface {
	LinkByName(string) (netlink.Link, error)
	LinkSetUp(netlink.Link) error
	LinkSetDown(netlink.Link) error
	LinkSetName(netlink.Link, string) error
	LinkSetNsFd(netlink.Link, int) error
	LinkAdd(link netlink.Link) error
	LinkDel(link netlink.Link) error
	SetSysVal(attribute, value string) (string, error)
}
