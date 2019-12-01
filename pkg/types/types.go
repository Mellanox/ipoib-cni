package types

import (
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
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
