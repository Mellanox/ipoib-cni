package types

import (
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/plugins/pkg/ns"
)

// NetConf extends cni NetConf
type NetConf struct {
	types.NetConf
	Master string `json:"master"`
}

// Manager provides interface invoke ipoib nic related operations
type Manager interface {
	CreateIpoibLink(conf *NetConf, ifName string, netns ns.NetNS) (*current.Interface, error)
	RemoveIpoibLink(ifName string, netnsPath string) error
}
