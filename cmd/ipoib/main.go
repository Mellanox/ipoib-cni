// Copyright 2025 ipoib-cni authors
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

package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"runtime"

	"github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	cniversion "github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"
	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"
	"github.com/j-keck/arping"
	"github.com/vishvananda/netlink"

	"github.com/Mellanox/ipoib-cni/pkg/config"
	"github.com/Mellanox/ipoib-cni/pkg/ipoib"
	"github.com/Mellanox/ipoib-cni/pkg/types"
)

const (
	dhcpType = "dhcp"
)

var (
	version = "master@git"
	commit  = "unknown commit"
	date    = "unknown date"
)

//nolint:gochecknoinits
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
		if n.IPAM.Type == dhcpType {
			return fmt.Errorf("ipam dhcp type is not supported")
		}
		err = handleIpamConfig(n, args, netns, result)
		if err != nil {
			return err
		}
	} else {
		// For L2 just change interface status to up
		err = netns.Do(func(_ ns.NetNS) error {
			ipoibInterfaceLink, innerErr := netlink.LinkByName(args.IfName)
			if innerErr != nil {
				return fmt.Errorf("failed to find interface name %q: %v", ibLink.Name, innerErr)
			}

			err = netlink.LinkSetUp(ipoibInterfaceLink)
			if err != nil {
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
		if n.IPAM.Type == dhcpType {
			return fmt.Errorf("ipam dhcp type is not supported")
		}
		err = ipam.ExecDel(n.IPAM.Type, args.StdinData)
		if err != nil {
			return err
		}
	}

	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		_, ok := err.(ns.NSPathNotExistErr)
		if ok {
			return nil
		}

		return fmt.Errorf("failed to open netns %s: %q", netns, err)
	}
	defer netns.Close()

	ipoibManager := ipoib.NewIpoibManager()

	return ipoibManager.RemoveIpoibLink(args.IfName, netns)
}

func main() {
	// Init command line flags to clear vendor packages' flags, especially in init()
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// add version flag
	versionOpt := false
	flag.BoolVar(&versionOpt, "version", false, "Show application version")
	flag.BoolVar(&versionOpt, "v", false, "Show application version")
	flag.Parse()
	if versionOpt {
		fmt.Printf("%s\n", printVersionString())
		return
	}

	skel.PluginMainFuncs(skel.CNIFuncs{Add: cmdAdd, Check: cmdCheck, Del: cmdDel},
		cniversion.All, bv.BuildString("ipoib-cni"))
}

func printVersionString() string {
	return fmt.Sprintf("ipoib-cni version:%s, commit:%s, date:%s", version, commit, date)
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

	err = cniversion.ParsePrevResult(&n.NetConf)
	if err != nil {
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

	// Check prevResults for ips, routes and dns against values found in the container
	if err := netns.Do(func(_ ns.NetNS) error {
		// Check interface against values found in the container
		err := validateCniContainerInterface(&contIface)
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

func validateCniContainerInterface(iface *current.Interface) error {
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
		return fmt.Errorf("container interface %s not of type ipoib", link.Attrs().Name)
	}

	return nil
}

func handleIpamConfig(netConfig *types.NetConf, args *skel.CmdArgs, netns ns.NetNS, result *current.Result) error {
	// run the IPAM plugin and get back the config to apply
	r, err := ipam.ExecAdd(netConfig.IPAM.Type, args.StdinData)
	if err != nil {
		return err
	}

	// Invoke ipam del if err to avoid ip leak
	defer func() {
		if err != nil {
			_ = ipam.ExecDel(netConfig.IPAM.Type, args.StdinData)
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
		// All addresses apply to the container ipoib interface
		ipc.Interface = current.Int(0)
	}

	err = netns.Do(func(_ ns.NetNS) error {
		if innerErr := ipam.ConfigureIface(args.IfName, result); innerErr != nil {
			return innerErr
		}

		contIface, innerErr := net.InterfaceByName(args.IfName)
		if innerErr != nil {
			return fmt.Errorf("failed to look up %q: %v", args.IfName, innerErr)
		}

		for _, ipc := range result.IPs {
			if ipc.Address.IP.To4() != nil {
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
