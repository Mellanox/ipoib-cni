package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/containernetworking/plugins/pkg/utils/sysctl"
	"github.com/j-keck/arping"
	"github.com/vishvananda/netlink"
	"net"

	"runtime"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/cni/pkg/version"

	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"
	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"
)

const (
	ipV4InterfaceArpProxySysctlTemplate = "net.ipv4.conf.%s.proxy_arp"
)

// NetConf extends cni NetConf
type NetConf struct {
	types.NetConf
	Master string `json:"master"`
}

func init() {
	runtime.LockOSThread()
}

func loadConf(bytes []byte) (*NetConf, string, error) {
	n := &NetConf{}
	if err := json.Unmarshal(bytes, n); err != nil {
		return nil, "", fmt.Errorf("failed to load netconf: %v", err)
	}
	if n.Master == "" {
		return nil, "", fmt.Errorf("host master interface is missing")
	}
	return n, n.CNIVersion, nil
}

func createIpoibLink(conf *NetConf, ifName string, netns ns.NetNS) (*current.Interface, error) {
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

func cmdAdd(args *skel.CmdArgs) error {
	n, cniVersion, err := loadConf(args.StdinData)
	if err != nil {
		return err
	}

	isIpamProvided := n.IPAM.Type != ""

	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", netns, err)
	}
	defer netns.Close()

	ibLink, err := createIpoibLink(n, args.IfName, netns)
	if err != nil {
		return err
	}

	// Delete link if err to avoid link leak in this ns
	defer func() {
		if err != nil {
			netns.Do(func(_ ns.NetNS) error {
				return ip.DelLinkByName(args.IfName)
			})
		}
	}()

	// Assume L2 interface only
	result := &current.Result{CNIVersion: cniVersion, Interfaces: []*current.Interface{ibLink}}

	if isIpamProvided {
		if n.IPAM.Type == "dhcp" {
			return fmt.Errorf("ipam dhcp type is not supported")
		}
		if err := handleIpamConfig(n, args, netns, result); err != nil {
			return err
		}
	} else {
		// For L2 just change interface status to up
		err = netns.Do(func(_ ns.NetNS) error {
			macvlanInterfaceLink, err := netlink.LinkByName(args.IfName)
			if err != nil {
				return fmt.Errorf("failed to find interface name %q: %v", ibLink.Name, err)
			}

			if err := netlink.LinkSetUp(macvlanInterfaceLink); err != nil {
				return fmt.Errorf("failed to set %q UP: %v", args.IfName, err)
			}

			return nil
		})
		if err != nil {
			return err
		}
	}

	result.DNS = n.DNS

	return types.PrintResult(result, cniVersion)
}

func cmdDel(args *skel.CmdArgs) error {
	n, _, err := loadConf(args.StdinData)
	if err != nil {
		return err
	}

	isIpamProvided := n.IPAM.Type != ""

	if isIpamProvided {
		if n.IPAM.Type == "dhcp" {
			return fmt.Errorf("ipam dhcp type is not supported")
		}
		err = ipam.ExecDel(n.IPAM.Type, args.StdinData)
		if err != nil {
			return err
		}
	}

	if args.Netns == "" {
		return nil
	}

	// There is a netns so try to clean up. Delete can be called multiple times
	// so don't return an error if the device is already removed.
	err = ns.WithNetNSPath(args.Netns, func(_ ns.NetNS) error {
		if err := ip.DelLinkByName(args.IfName); err != nil {
			if err != ip.ErrLinkNotFound {
				return err
			}
		}
		return nil
	})

	return err
}

func main() {
	skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.All, bv.BuildString("ipoib-cni"))
}

func cmdCheck(args *skel.CmdArgs) error {

	n, _, err := loadConf(args.StdinData)
	if err != nil {
		return err
	}
	isIpamProvided := n.IPAM.Type != ""

	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", args.Netns, err)
	}
	defer netns.Close()

	if isIpamProvided {
		// run the IPAM plugin and get back the config to apply
		err = ipam.ExecCheck(n.IPAM.Type, args.StdinData)
		if err != nil {
			return err
		}
	}

	// Parse previous result.
	if n.NetConf.RawPrevResult == nil {
		return fmt.Errorf("required prevResult missing")
	}

	if err := version.ParsePrevResult(&n.NetConf); err != nil {
		return err
	}

	result, err := current.NewResultFromResult(n.PrevResult)
	if err != nil {
		return err
	}

	var contIface current.Interface
	// Find interfaces for names whe know, ipoib device name inside container
	for _, iface := range result.Interfaces {
		if args.IfName == iface.Name {
			if args.Netns == iface.Sandbox {
				contIface = *iface
				continue
			}
		}
	}

	// The namespace must be the same as what was configured
	if args.Netns != contIface.Sandbox {
		return fmt.Errorf("sandbox in prevResult %s doesn't match configured netns: %s",
			contIface.Sandbox, args.Netns)
	}

	m, err := netlink.LinkByName(n.Master)
	if err != nil {
		return fmt.Errorf("failed to lookup master %q: %v", n.Master, err)
	}

	// Check prevResults for ips, routes and dns against values found in the container
	if err := netns.Do(func(_ ns.NetNS) error {

		// Check interface against values found in the container
		err := validateCniContainerInterface(contIface, m.Attrs().Index)
		if err != nil {
			return err
		}

		err = ip.ValidateExpectedInterfaceIPs(args.IfName, result.IPs)
		if err != nil {
			return err
		}

		err = ip.ValidateExpectedRoute(result.Routes)
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

func validateCniContainerInterface(iface current.Interface, parentIndex int) error {

	var link netlink.Link
	var err error

	if iface.Name == "" {
		return fmt.Errorf("container interface name missing in prevResult: %v", iface.Name)
	}
	link, err = netlink.LinkByName(iface.Name)
	if err != nil {
		return fmt.Errorf("container Interface name in prevResult: %s not found", iface.Name)
	}
	if iface.Sandbox == "" {
		return fmt.Errorf("container interface %s should not be in host namespace", link.Attrs().Name)
	}

	_, isIpoib := link.(*netlink.IPoIB)
	if !isIpoib {
		return fmt.Errorf("container interface %s not of type macvlan", link.Attrs().Name)
	}

	if iface.Mac != "" {
		if iface.Mac != link.Attrs().HardwareAddr.String() {
			return fmt.Errorf("interface %s Mac %s doesn't match container Mac: %s", iface.Name, iface.Mac, link.Attrs().HardwareAddr)
		}
	}

	return nil
}

func handleIpamConfig(config *NetConf, args *skel.CmdArgs, netns ns.NetNS, result *current.Result) error {
	// run the IPAM plugin and get back the config to apply
	r, err := ipam.ExecAdd(config.IPAM.Type, args.StdinData)
	if err != nil {
		return err
	}

	// Invoke ipam del if err to avoid ip leak
	defer func() {
		if err != nil {
			ipam.ExecDel(config.IPAM.Type, args.StdinData)
		}
	}()

	// Convert whatever the IPAM result was into the current Result type
	ipamResult, err := current.NewResultFromResult(r)
	if err != nil {
		return err
	}

	if len(ipamResult.IPs) == 0 {
		return errors.New("IPAM plugin returned missing IP config")
	}

	result.IPs = ipamResult.IPs
	result.Routes = ipamResult.Routes

	for _, ipc := range result.IPs {
		// All addresses apply to the container macvlan interface
		ipc.Interface = current.Int(0)
	}

	err = netns.Do(func(_ ns.NetNS) error {
		if err := ipam.ConfigureIface(args.IfName, result); err != nil {
			return err
		}

		contIface, err := net.InterfaceByName(args.IfName)
		if err != nil {
			return fmt.Errorf("failed to look up %q: %v", args.IfName, err)
		}

		for _, ipc := range result.IPs {
			if ipc.Version == "4" {
				_ = arping.GratuitousArpOverIface(ipc.Address.IP, *contIface)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}
