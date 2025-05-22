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

package ipoib

import (
	"errors"

	"github.com/containernetworking/plugins/pkg/ns"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"github.com/vishvananda/netlink"

	"github.com/Mellanox/ipoib-cni/pkg/types"
	"github.com/Mellanox/ipoib-cni/pkg/types/mocks"
)

// FakeLink is a dummy netlink struct used during testing
type FakeLink struct {
	netlink.LinkAttrs
}

func (l *FakeLink) Attrs() *netlink.LinkAttrs {
	return &l.LinkAttrs
}

func (l *FakeLink) Type() string {
	return "FakeLink"
}

// Fake NS - implements ns.NetNS interface
type fakeNetNS struct {
	closed bool
	fd     uintptr
	path   string
}

func (f *fakeNetNS) Do(toRun func(ns.NetNS) error) error {
	return toRun(f)
}

func (f *fakeNetNS) Set() error {
	return nil
}

func (f *fakeNetNS) Path() string {
	return f.path
}

func (f *fakeNetNS) Fd() uintptr {
	return f.fd
}

func (f *fakeNetNS) Close() error {
	f.closed = true
	return nil
}

func newFakeNs() ns.NetNS {
	return &fakeNetNS{
		closed: false,
		fd:     17,
		path:   "/proc/4123/ns/net",
	}
}

var _ = Describe("IPoIB", func() {

	Context("Checking CreateIpoibLink function", func() {
		var (
			ifName         string
			netconf        *types.NetConf
			fakeMasterLink *netlink.IPoIB
		)

		BeforeEach(func() {
			ifName = "eth0"
			netconf = &types.NetConf{
				Master: "ib0",
			}
			fakeMasterLink = &netlink.IPoIB{LinkAttrs: netlink.NewLinkAttrs(), Pkey: 0xffff, Mode: netlink.IPOIB_MODE_DATAGRAM}
		})

		It("Assuming create link and move it to container", func() {
			targetNetNS := newFakeNs()
			mocked := &mocks.NetlinkManager{}
			fakeLink := &FakeLink{}

			mocked.On("LinkByName", netconf.Master).Return(fakeMasterLink, nil)
			mocked.On("LinkAdd", mock.MatchedBy(func(l *netlink.IPoIB) bool {
				return l.Pkey == (fakeMasterLink.Pkey&0x7fff) && l.Mode == fakeMasterLink.Mode
			})).Return(nil)
			mocked.On("LinkByName", mock.AnythingOfType("string")).Return(fakeLink, nil)
			mocked.On("LinkSetNsFd", fakeLink, mock.AnythingOfType("int")).Return(nil)
			mocked.On("SetSysVal", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return("", nil)
			mocked.On("LinkSetDown", fakeLink).Return(nil)
			mocked.On("LinkSetName", fakeLink, mock.AnythingOfType("string")).Return(nil)
			mocked.On("LinkSetUp", fakeLink).Return(nil)

			im := ipoibManager{nLink: mocked}
			ipoibLink, err := im.CreateIpoibLink(netconf, ifName, targetNetNS)

			Expect(err).NotTo(HaveOccurred())
			Expect(ipoibLink).NotTo(BeNil())
			mocked.AssertExpectations(GinkgoT())
		})
		It("Assuming not existing master", func() {
			targetNetNS := newFakeNs()
			mocked := &mocks.NetlinkManager{}

			mocked.On("LinkByName", netconf.Master).Return(nil, errors.New("not found"))
			im := ipoibManager{nLink: mocked}
			ipoibLink, err := im.CreateIpoibLink(netconf, ifName, targetNetNS)

			Expect(err).To(HaveOccurred())
			Expect(ipoibLink).To(BeNil())
			mocked.AssertExpectations(GinkgoT())
		})
		It("Assuming failed to create link", func() {
			targetNetNS := newFakeNs()
			mocked := &mocks.NetlinkManager{}

			mocked.On("LinkByName", netconf.Master).Return(fakeMasterLink, nil)
			mocked.On("LinkAdd", mock.Anything).Return(errors.New("failed"))
			im := ipoibManager{nLink: mocked}
			ipoibLink, err := im.CreateIpoibLink(netconf, ifName, targetNetNS)
			Expect(err).To(HaveOccurred())
			Expect(ipoibLink).To(BeNil())

			mocked.AssertExpectations(GinkgoT())
		})
		It("Assuming failed to set proxy value", func() {
			targetNetNS := newFakeNs()
			mocked := &mocks.NetlinkManager{}
			fakeLink := &FakeLink{}

			mocked.On("LinkByName", netconf.Master).Return(fakeMasterLink, nil)
			mocked.On("LinkAdd", mock.Anything).Return(nil)
			mocked.On("LinkByName", mock.AnythingOfType("string")).Return(fakeLink, nil)
			mocked.On("LinkSetNsFd", fakeLink, mock.AnythingOfType("int")).Return(nil)
			mocked.On("LinkDel", mock.Anything).Return(nil)
			mocked.On("SetSysVal", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return("", errors.New("failed"))

			im := ipoibManager{nLink: mocked}
			ipoibLink, err := im.CreateIpoibLink(netconf, ifName, targetNetNS)

			Expect(err).To(HaveOccurred())
			Expect(ipoibLink).To(BeNil())
			mocked.AssertExpectations(GinkgoT())
		})
		It("Assuming failed to change name", func() {
			targetNetNS := newFakeNs()
			mocked := &mocks.NetlinkManager{}
			fakeLink := &FakeLink{}

			mocked.On("LinkByName", netconf.Master).Return(fakeMasterLink, nil)
			mocked.On("LinkAdd", mock.Anything).Return(nil)
			mocked.On("LinkByName", mock.AnythingOfType("string")).Return(fakeLink, nil)
			mocked.On("LinkSetNsFd", fakeLink, mock.AnythingOfType("int")).Return(nil)
			mocked.On("SetSysVal", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return("", nil)
			mocked.On("LinkSetDown", fakeLink).Return(nil)
			mocked.On("LinkSetName", fakeLink, mock.AnythingOfType("string")).Return(errors.New("failed"))
			mocked.On("LinkDel", mock.Anything).Return(nil)

			im := ipoibManager{nLink: mocked}
			ipoibLink, err := im.CreateIpoibLink(netconf, ifName, targetNetNS)
			Expect(err).To(HaveOccurred())
			Expect(ipoibLink).To(BeNil())
			mocked.AssertExpectations(GinkgoT())
		})
	})
	Context("Checking RemoveIpoibLink function", func() {
		var (
			ifName string
		)

		BeforeEach(func() {
			ifName = "eth0"
		})

		It("Assuming existing interface", func() {
			targetNetNS := newFakeNs()
			mocked := &mocks.NetlinkManager{}
			fakeLink := &FakeLink{netlink.LinkAttrs{}}

			mocked.On("LinkByName", mock.AnythingOfType("string")).Return(fakeLink, nil)
			mocked.On("LinkDel", fakeLink).Return(nil)

			im := ipoibManager{nLink: mocked}
			err := im.RemoveIpoibLink(ifName, targetNetNS)

			Expect(err).NotTo(HaveOccurred())
			mocked.AssertExpectations(GinkgoT())
		})
		It("Assuming non existing interface, failed after add", func() {
			targetNetNS := newFakeNs()
			mocked := &mocks.NetlinkManager{}

			mocked.On("LinkByName", mock.AnythingOfType("string")).Return(nil, errors.New("not found"))

			im := ipoibManager{nLink: mocked}
			err := im.RemoveIpoibLink(ifName, targetNetNS)

			Expect(err).NotTo(HaveOccurred())
			mocked.AssertExpectations(GinkgoT())
		})
		It("Assuming existing interface and failed to remove", func() {
			targetNetNS := newFakeNs()
			mocked := &mocks.NetlinkManager{}
			fakeLink := &FakeLink{netlink.LinkAttrs{}}

			mocked.On("LinkByName", mock.AnythingOfType("string")).Return(fakeLink, nil)
			mocked.On("LinkDel", fakeLink).Return(errors.New("failed to remove"))

			im := ipoibManager{nLink: mocked}
			err := im.RemoveIpoibLink(ifName, targetNetNS)

			Expect(err).To(HaveOccurred())
			mocked.AssertExpectations(GinkgoT())
		})
	})
})
