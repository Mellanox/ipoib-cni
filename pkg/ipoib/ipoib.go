package ipoib

import (
	"fmt"
	"github.com/Mellanox/ipoib-cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/utils/sysctl"
	"github.com/vishvananda/netlink"
)

const (
	ipV4InterfaceArpProxySysctlTemplate = "net.ipv4.conf.%s.proxy_arp"
)

type ipoibManager struct {
}

// NewIpoibManager returns an instance of IpoibManager
func NewIpoibManager() types.Manager {
	return &ipoibManager{}
}

// CreateIpoibLink create a link in pod netns
func (im *ipoibManager) CreateIpoibLink(conf *types.NetConf, ifName string, netns ns.NetNS) (*current.Interface, error) {
	iface := &current.Interface{}
	m, err := netlink.LinkByName(conf.Master)
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

	if err := netlink.LinkAdd(ipoibLink); err != nil {
		return nil, fmt.Errorf("failed to create interface: %v", err)
	}
	link, err := netlink.LinkByName(tmpName)
	if err != nil {
		return nil, err
	}

	if err = netlink.LinkSetNsFd(link, int(netns.Fd())); err != nil {
		return nil, fmt.Errorf("failed to move interfaceee %s to netns: %v", tmpName, err)
	}

	err = netns.Do(func(_ ns.NetNS) error {
		ipv4SysctlValueName := fmt.Sprintf(ipV4InterfaceArpProxySysctlTemplate, tmpName)
		if _, err := sysctl.Sysctl(ipv4SysctlValueName, "1"); err != nil {
			// remove the newly added link and ignore errors, because we already are in a failed state
			_ = netlink.LinkDel(ipoibLink)
			return fmt.Errorf("failed to set proxy_arp on newly added interface %q: %v", tmpName, err)
		}

		err := ip.RenameLink(tmpName, ifName)
		if err != nil {
			_ = netlink.LinkDel(ipoibLink)
			return fmt.Errorf("failed to rename interface to %q: %v", ifName, err)
		}
		iface.Name = ifName

		ipoibContLink, err := netlink.LinkByName(ifName)
		if err != nil {
			return fmt.Errorf("failed to refetch interface %q: %v", ifName, err)
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

func (im *ipoibManager) RemoveIpoibLink(ifName string, netnsPath string) error {

	// There is a netns so try to clean up. Delete can be called multiple times
	// so don't return an error if the device is already removed.
	return ns.WithNetNSPath(netnsPath, func(_ ns.NetNS) error {
		if err := ip.DelLinkByName(ifName); err != nil {
			if err != ip.ErrLinkNotFound {
				return err
			}
		}
		return nil
	})
}
