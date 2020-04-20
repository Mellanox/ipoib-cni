package ipoib

import (
	"fmt"

	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/utils/sysctl"
	"github.com/vishvananda/netlink"

	"github.com/Mellanox/ipoib-cni/pkg/types"
)

const (
	ipV4InterfaceArpProxySysctlTemplate = "net.ipv4.conf.%s.proxy_arp"
)

type ipoibManager struct {
	nLink types.NetlinkManager
}

type netLink struct {
}

// LinkByName implements NetlinkManager
func (n *netLink) LinkByName(name string) (netlink.Link, error) {
	return netlink.LinkByName(name)
}

// LinkSetUp using NetlinkManager
func (n *netLink) LinkSetUp(link netlink.Link) error {
	return netlink.LinkSetUp(link)
}

// LinkSetDown using NetlinkManager
func (n *netLink) LinkSetDown(link netlink.Link) error {
	return netlink.LinkSetDown(link)
}

// LinkSetName using NetlinkManager
func (n *netLink) LinkSetName(link netlink.Link, name string) error {
	return netlink.LinkSetName(link, name)
}

// LinkSetNsFd using NetlinkManager
func (n *netLink) LinkSetNsFd(link netlink.Link, fd int) error {
	return netlink.LinkSetNsFd(link, fd)
}

// LinkAdd using NetLinkManager
func (n *netLink) LinkAdd(link netlink.Link) error {
	return netlink.LinkAdd(link)
}

// LinkDel using NetLinkManager
func (n *netLink) LinkDel(link netlink.Link) error {
	return netlink.LinkDel(link)
}

// SetSysVal set value for sysctl attribute
func (n *netLink) SetSysVal(attribute, value string) (string, error) {
	return sysctl.Sysctl(attribute, value)
}

// NewIpoibManager returns an instance of IpoibManager
func NewIpoibManager() types.Manager {
	return &ipoibManager{
		nLink: &netLink{},
	}
}

// CreateIpoibLink create a link in pod netns
func (im *ipoibManager) CreateIpoibLink(conf *types.NetConf, ifName string, netns ns.NetNS) (
	*current.Interface, error) {
	iface := &current.Interface{}
	m, err := im.nLink.LinkByName(conf.Master)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup master %q: %v", conf.Master, err)
	}

	tmpName, err := ip.RandomVethName()
	if err != nil {
		return nil, err
	}

	ipoibLink := &netlink.IPoIB{
		LinkAttrs: netlink.LinkAttrs{
			Name:        tmpName,
			ParentIndex: m.Attrs().Index,
			// Due to kernal bug create the link then move it to the desired namespace
			//		Namespace:   netlink.NsFd(int(curNetns.Fd())),
		},
		Pkey:   0x7fff,
		Mode:   netlink.IPOIB_MODE_DATAGRAM,
		Umcast: 1,
	}

	if err = im.nLink.LinkAdd(ipoibLink); err != nil {
		return nil, fmt.Errorf("failed to create interface: %v", err)
	}
	link, err := im.nLink.LinkByName(tmpName)
	if err != nil {
		return nil, err
	}

	if err = im.nLink.LinkSetNsFd(link, int(netns.Fd())); err != nil {
		return nil, fmt.Errorf("failed to move interfaceee %s to netns: %v", tmpName, err)
	}

	err = netns.Do(func(_ ns.NetNS) error {
		ipv4SysctlValueName := fmt.Sprintf(ipV4InterfaceArpProxySysctlTemplate, tmpName)
		if _, innerErr := im.nLink.SetSysVal(ipv4SysctlValueName, "1"); innerErr != nil {
			// remove the newly added link and ignore errors, because we already are in a failed state
			_ = im.nLink.LinkDel(ipoibLink)
			return fmt.Errorf("failed to set proxy_arp on newly added interface %q: %v", tmpName, innerErr)
		}

		if innerErr := im.nLink.LinkSetDown(link); innerErr != nil {
			return innerErr
		}
		if innerErr := im.nLink.LinkSetName(link, ifName); innerErr != nil {
			_ = im.nLink.LinkDel(ipoibLink)
			return fmt.Errorf("failed to rename interface to %q: %v", ifName, innerErr)
		}
		if innerErr := im.nLink.LinkSetUp(link); innerErr != nil {
			return innerErr
		}
		iface.Name = ifName

		ipoibContLink, innerErr := im.nLink.LinkByName(ifName)
		if innerErr != nil {
			return fmt.Errorf("failed to refetch interface %q: %v", ifName, innerErr)
		}
		iface.Mac = ipoibContLink.Attrs().HardwareAddr.String()
		iface.Sandbox = netns.Path()

		return nil
	})
	if err != nil {
		return nil, err
	}

	return iface, nil
}

func (im *ipoibManager) RemoveIpoibLink(ifName string, netns ns.NetNS) error {
	// There is a netns so try to clean up. Delete can be called multiple times
	// so don't return an error if the device is already removed.
	return netns.Do(func(_ ns.NetNS) error {
		link, err := im.nLink.LinkByName(ifName)
		if err != nil {
			// Link not in the container if cni Add failed
			return nil
		}

		if err := im.nLink.LinkDel(link); err != nil {
			return err
		}
		return nil
	})
}
