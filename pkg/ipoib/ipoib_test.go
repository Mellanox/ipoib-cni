package ipoib

import (
	"errors"
	"github.com/Mellanox/ipoib-cni/pkg/types"
	"github.com/Mellanox/ipoib-cni/pkg/types/mocks"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	"github.com/vishvananda/netlink"
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

var _ = Describe("IPoIB", func() {

	Context("Checking CreateIpoibLink function", func() {
		var (
			ifName  string
			netconf *types.NetConf
		)

		BeforeEach(func() {
			ifName = "eth0"
			netconf = &types.NetConf{
				Master: "ib0",
			}
		})

		It("Assuming create link and move it to container", func() {
			var targetNetNS ns.NetNS
			targetNetNS, err := testutils.NewNS()
			defer func() {
				if targetNetNS != nil {
					targetNetNS.Close()
				}
			}()
			Expect(err).NotTo(HaveOccurred())

			mocked := &mocks.NetlinkManager{}
			fakeLink := &FakeLink{}

			mocked.On("LinkByName", mock.AnythingOfType("string")).Return(fakeLink, nil)
			mocked.On("LinkAdd", mock.Anything).Return(nil)
			mocked.On("LinkSetNsFd", fakeLink, mock.AnythingOfType("int")).Return(nil)
			mocked.On("LinkDel", mock.Anything).Return(nil)
			mocked.On("SetSysVal", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return("", nil)
			mocked.On("LinkSetDown", fakeLink).Return(nil)
			mocked.On("LinkSetName", fakeLink, mock.AnythingOfType("string")).Return(nil)
			mocked.On("LinkSetUp", fakeLink).Return(nil)
			im := ipoibManager{nLink: mocked}
			ipoibLink, err := im.CreateIpoibLink(netconf, ifName, targetNetNS)
			Expect(err).NotTo(HaveOccurred())
			Expect(ipoibLink).NotTo(BeNil())
		})
		It("Assuming not existing master", func() {
			var targetNetNS ns.NetNS
			targetNetNS, err := testutils.NewNS()
			defer func() {
				if targetNetNS != nil {
					targetNetNS.Close()
				}
			}()
			Expect(err).NotTo(HaveOccurred())

			mocked := &mocks.NetlinkManager{}

			mocked.On("LinkByName", mock.AnythingOfType("string")).Return(nil, errors.New("not found"))
			im := ipoibManager{nLink: mocked}
			ipoibLink, err := im.CreateIpoibLink(netconf, ifName, targetNetNS)
			Expect(err).To(HaveOccurred())
			Expect(ipoibLink).To(BeNil())
		})
		It("Assuming failed to create link", func() {
			var targetNetNS ns.NetNS
			targetNetNS, err := testutils.NewNS()
			defer func() {
				if targetNetNS != nil {
					targetNetNS.Close()
				}
			}()
			Expect(err).NotTo(HaveOccurred())

			mocked := &mocks.NetlinkManager{}
			fakeLink := &FakeLink{}

			mocked.On("LinkByName", mock.AnythingOfType("string")).Return(fakeLink, nil)
			mocked.On("LinkAdd", mock.Anything).Return(errors.New("failed"))
			im := ipoibManager{nLink: mocked}
			ipoibLink, err := im.CreateIpoibLink(netconf, ifName, targetNetNS)
			Expect(err).To(HaveOccurred())
			Expect(ipoibLink).To(BeNil())
		})
		It("Assuming failed to set proxy value", func() {
			var targetNetNS ns.NetNS
			targetNetNS, err := testutils.NewNS()
			defer func() {
				if targetNetNS != nil {
					targetNetNS.Close()
				}
			}()
			Expect(err).NotTo(HaveOccurred())

			mocked := &mocks.NetlinkManager{}
			fakeLink := &FakeLink{}

			mocked.On("LinkByName", mock.AnythingOfType("string")).Return(fakeLink, nil)
			mocked.On("LinkAdd", mock.Anything).Return(nil)
			mocked.On("LinkSetNsFd", fakeLink, mock.AnythingOfType("int")).Return(nil)
			mocked.On("LinkDel", mock.Anything).Return(nil)
			mocked.On("SetSysVal", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return("", errors.New("failed"))
			im := ipoibManager{nLink: mocked}
			ipoibLink, err := im.CreateIpoibLink(netconf, ifName, targetNetNS)
			Expect(err).To(HaveOccurred())
			Expect(ipoibLink).To(BeNil())
		})
		It("Assuming failed to change name", func() {
			var targetNetNS ns.NetNS
			targetNetNS, err := testutils.NewNS()
			defer func() {
				if targetNetNS != nil {
					targetNetNS.Close()
				}
			}()
			Expect(err).NotTo(HaveOccurred())

			mocked := &mocks.NetlinkManager{}
			fakeLink := &FakeLink{}

			mocked.On("LinkByName", mock.AnythingOfType("string")).Return(fakeLink, nil)
			mocked.On("LinkAdd", mock.Anything).Return(nil)
			mocked.On("LinkSetNsFd", fakeLink, mock.AnythingOfType("int")).Return(nil)
			mocked.On("LinkDel", mock.Anything).Return(nil)
			mocked.On("SetSysVal", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return("", nil)
			mocked.On("LinkSetDown", fakeLink).Return(nil)
			mocked.On("LinkSetName", fakeLink, mock.AnythingOfType("string")).Return(errors.New("failed"))
			im := ipoibManager{nLink: mocked}
			ipoibLink, err := im.CreateIpoibLink(netconf, ifName, targetNetNS)
			Expect(err).To(HaveOccurred())
			Expect(ipoibLink).To(BeNil())
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
			var targetNetNS ns.NetNS
			targetNetNS, err := testutils.NewNS()
			defer func() {
				if targetNetNS != nil {
					targetNetNS.Close()
				}
			}()
			Expect(err).NotTo(HaveOccurred())
			mocked := &mocks.NetlinkManager{}

			Expect(err).NotTo(HaveOccurred())

			fakeLink := &FakeLink{netlink.LinkAttrs{}}

			mocked.On("LinkByName", mock.AnythingOfType("string")).Return(fakeLink, nil)
			mocked.On("LinkDel", fakeLink).Return(nil)
			im := ipoibManager{nLink: mocked}
			err = im.RemoveIpoibLink(ifName, targetNetNS)
			Expect(err).NotTo(HaveOccurred())
		})
		It("Assuming non existing interface, failed after add", func() {
			var targetNetNS ns.NetNS
			targetNetNS, err := testutils.NewNS()
			defer func() {
				if targetNetNS != nil {
					targetNetNS.Close()
				}
			}()
			Expect(err).NotTo(HaveOccurred())
			mocked := &mocks.NetlinkManager{}

			mocked.On("LinkByName", mock.AnythingOfType("string")).Return(nil, errors.New("not found"))
			im := ipoibManager{nLink: mocked}
			err = im.RemoveIpoibLink(ifName, targetNetNS)
			Expect(err).NotTo(HaveOccurred())
		})
		It("Assuming existing interface and failed to remove", func() {
			var targetNetNS ns.NetNS
			targetNetNS, err := testutils.NewNS()
			defer func() {
				if targetNetNS != nil {
					targetNetNS.Close()
				}
			}()
			Expect(err).NotTo(HaveOccurred())
			mocked := &mocks.NetlinkManager{}

			Expect(err).NotTo(HaveOccurred())

			fakeLink := &FakeLink{netlink.LinkAttrs{}}

			mocked.On("LinkByName", mock.AnythingOfType("string")).Return(fakeLink, nil)
			mocked.On("LinkDel", fakeLink).Return(errors.New("failed to remove"))
			im := ipoibManager{nLink: mocked}
			err = im.RemoveIpoibLink(ifName, targetNetNS)
			Expect(err).To(HaveOccurred())
		})
	})
})
