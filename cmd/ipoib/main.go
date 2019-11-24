package main

import (
	"errors"
	"fmt"
	"github.com/Mellanox/ipoib-cni/pkg/config"
	"github.com/Mellanox/ipoib-cni/pkg/ipoib"
	"github.com/Mellanox/ipoib-cni/pkg/types"
	"github.com/j-keck/arping"
	"github.com/vishvananda/netlink"
	"net"

	"runtime"

	"github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/cni/pkg/version"

	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"
	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"
)

func init() {
	runtime.LockOSThread()
}

func cmdAdd(args *skel.CmdArgs) error {
	n, cniVersion, err := config.LoadConf(args.StdinData)
	if err != nil {
		return err
	}

	isIpamProvided := n.IPAM.Type != ""

	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", netns, err)
	}
	defer netns.Close()

	ipoibManager := ipoib.NewIpoibManager()

	ibLink, err := ipoibManager.CreateIpoibLink(n, args.IfName, netns)
	if err != nil {
		return err
	}

	// Delete link if err to avoid link leak in this ns
	defer func() {
		if err != nil {
			_ = netns.Do(func(_ ns.NetNS) error {
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

	return cniTypes.PrintResult(result, cniVersion)
}

func cmdDel(args *skel.CmdArgs) error {
	n, _, err := config.LoadConf(args.StdinData)
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

	ipoibManager := ipoib.NewIpoibManager()

	return ipoibManager.RemoveIpoibLink(args.IfName, args.Netns)
}

func main() {
	skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.All, bv.BuildString("ipoib-cni"))
}

func cmdCheck(args *skel.CmdArgs) error {

	n, _, err := config.LoadConf(args.StdinData)
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

func handleIpamConfig(config *types.NetConf, args *skel.CmdArgs, netns ns.NetNS, result *current.Result) error {
	// run the IPAM plugin and get back the config to apply
	r, err := ipam.ExecAdd(config.IPAM.Type, args.StdinData)
	if err != nil {
		return err
	}

	// Invoke ipam del if err to avoid ip leak
	defer func() {
		if err != nil {
			_ = ipam.ExecDel(config.IPAM.Type, args.StdinData)
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
