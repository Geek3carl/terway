//go:build privileged
// +build privileged

package driver

import (
	"net"
	"runtime"
	"testing"

	terwayTypes "github.com/AliyunContainerService/terway/types"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

func TestDataPathVPCRoute(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var err error
	hostNS, err := testutils.NewNS()
	assert.NoError(t, err)

	containerNS, err := testutils.NewNS()
	assert.NoError(t, err)

	err = hostNS.Set()
	assert.NoError(t, err)

	defer func() {
		err := containerNS.Close()
		assert.NoError(t, err)

		err = testutils.UnmountNS(containerNS)
		assert.NoError(t, err)

		err = hostNS.Close()
		assert.NoError(t, err)

		err = testutils.UnmountNS(hostNS)
		assert.NoError(t, err)
	}()

	cfg := &SetupConfig{
		HostVETHName:    "veth1",
		ContainerIfName: "eth0",
		ContainerIPNet: &terwayTypes.IPNetSet{
			IPv4: containerIPNet,
			IPv6: containerIPNetIPv6,
		},
		GatewayIP: &terwayTypes.IPSet{
			IPv4: ipv4GW,
			IPv6: ipv6GW,
		},
		MTU:            1499,
		ENIIndex:       0,
		TrunkENI:       false,
		ExtraRoutes:    nil,
		ServiceCIDR:    nil,
		HostStackCIDRs: nil,
		Ingress:        0,
		Egress:         0,
	}
	d := NewVETHDriver(true, true)

	err = d.Setup(cfg, containerNS)
	assert.NoError(t, err)

	_ = containerNS.Do(func(netNS ns.NetNS) error {
		// addr
		containerLink, err := netlink.LinkByName(cfg.ContainerIfName)
		assert.NoError(t, err)
		assert.Equal(t, cfg.MTU, containerLink.Attrs().MTU)
		assert.True(t, containerLink.Attrs().Flags&net.FlagUp != 0)

		ok, err := FindIP(containerLink, cfg.ContainerIPNet)
		assert.NoError(t, err)
		assert.True(t, ok)

		// default via 169.254.1.1 dev eth0
		routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{
			Dst: nil,
		}, netlink.RT_FILTER_DST)
		assert.NoError(t, err)
		assert.Equal(t, len(routes), 1)

		assert.Equal(t, net.ParseIP("169.254.1.1").String(), routes[0].Gw.String())

		return nil
	})

	hostLink, err := netlink.LinkByName(cfg.HostVETHName)
	assert.NoError(t, err)
	assert.Equal(t, cfg.MTU, hostLink.Attrs().MTU)

	// 169.10.0.10 dev eth0
	routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{
		Dst:       cfg.ContainerIPNet.IPv4,
		LinkIndex: hostLink.Attrs().Index,
	}, netlink.RT_FILTER_DST|netlink.RT_FILTER_OIF)
	assert.NoError(t, err)
	assert.Equal(t, len(routes), 1)

	// tear down

	err = d.Teardown(&TeardownCfg{
		HostVETHName:    cfg.HostVETHName,
		ContainerIfName: cfg.ContainerIfName,
		ContainerIPNet:  nil,
	}, containerNS)

	_, err = netlink.LinkByName(cfg.HostVETHName)
	assert.Error(t, err)
	_, ok := err.(netlink.LinkNotFoundError)
	assert.True(t, ok)
}

func TestDataPathPolicyRoute(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var err error
	hostNS, err := testutils.NewNS()
	assert.NoError(t, err)

	containerNS, err := testutils.NewNS()
	assert.NoError(t, err)

	err = hostNS.Set()
	assert.NoError(t, err)

	defer func() {
		err := containerNS.Close()
		assert.NoError(t, err)

		err = testutils.UnmountNS(containerNS)
		assert.NoError(t, err)

		err = hostNS.Close()
		assert.NoError(t, err)

		err = testutils.UnmountNS(hostNS)
		assert.NoError(t, err)
	}()

	err = netlink.LinkAdd(&netlink.Dummy{
		LinkAttrs: netlink.LinkAttrs{Name: "eni"},
	})
	assert.NoError(t, err)
	eni, err := netlink.LinkByName("eni")
	assert.NoError(t, err)

	cfg := &SetupConfig{
		HostVETHName:    "hostveth",
		ContainerIfName: "eth0",
		ContainerIPNet: &terwayTypes.IPNetSet{
			IPv4: containerIPNet,
			IPv6: containerIPNetIPv6,
		},
		GatewayIP: &terwayTypes.IPSet{
			IPv4: ipv4GW,
			IPv6: ipv6GW,
		},
		MTU:            1499,
		ENIIndex:       eni.Attrs().Index,
		TrunkENI:       false,
		ExtraRoutes:    nil,
		ServiceCIDR:    nil,
		HostStackCIDRs: nil,
		Ingress:        0,
		Egress:         0,
		HostIPSet: &terwayTypes.IPNetSet{
			IPv4: eth0IPNet,
			IPv6: eth0IPNetIPv6,
		},
	}
	d := NewVETHDriver(true, true)

	err = d.Setup(cfg, containerNS)
	assert.NoError(t, err)

	_ = containerNS.Do(func(netNS ns.NetNS) error {
		// addr
		containerLink, err := netlink.LinkByName(cfg.ContainerIfName)
		assert.NoError(t, err)
		assert.Equal(t, cfg.MTU, containerLink.Attrs().MTU)
		assert.True(t, containerLink.Attrs().Flags&net.FlagUp != 0)

		ok, err := FindIP(containerLink, cfg.ContainerIPNet)
		assert.NoError(t, err)
		assert.True(t, ok)

		// default via 169.254.1.1 dev eth0
		routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{
			Dst: nil,
		}, netlink.RT_FILTER_DST)
		assert.NoError(t, err)
		assert.Equal(t, len(routes), 1)

		assert.Equal(t, net.ParseIP("169.254.1.1").String(), routes[0].Gw.String())

		return nil
	})

	// eth0's ip 169.20.20.10/32 dev eni
	ok, err := FindIP(eni, &terwayTypes.IPNetSet{
		IPv4: &net.IPNet{
			IP:   cfg.HostIPSet.IPv4.IP,
			Mask: net.CIDRMask(32, 32),
		},
		IPv6: nil,
	})
	assert.NoError(t, err)
	assert.True(t, ok)

	hostVETHLink, err := netlink.LinkByName(cfg.HostVETHName)
	assert.NoError(t, err)
	assert.Equal(t, cfg.MTU, hostVETHLink.Attrs().MTU)

	addrs, err := netlink.AddrList(hostVETHLink, netlink.FAMILY_V4)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(addrs))

	// 169.10.0.10 dev hostVETH
	routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{
		Dst: &net.IPNet{
			IP:   cfg.ContainerIPNet.IPv4.IP,
			Mask: net.CIDRMask(32, 32),
		},
		LinkIndex: hostVETHLink.Attrs().Index,
	}, netlink.RT_FILTER_DST|netlink.RT_FILTER_OIF)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(routes))

	// 512 from all to 169.10.0.10 look up main
	rules, err := netlink.RuleListFiltered(netlink.FAMILY_V4, &netlink.Rule{
		Priority: toContainerPriority,
		Table:    unix.RT_TABLE_MAIN,
		Dst: &net.IPNet{
			IP:   cfg.ContainerIPNet.IPv4.IP,
			Mask: net.CIDRMask(32, 32),
		},
	}, netlink.RT_FILTER_TABLE|netlink.RT_FILTER_DST|netlink.RT_FILTER_PRIORITY)
	assert.NoError(t, err)
	assert.Equal(t, len(rules), 1)
	assert.Nil(t, rules[0].Src)

	// 2048 from 169.10.0.10 iif hostVETH lookup table
	rules, err = netlink.RuleListFiltered(netlink.FAMILY_V4, &netlink.Rule{
		Priority: fromContainerPriority,
		Table:    getRouteTableID(eni.Attrs().Index),
		Src: &net.IPNet{
			IP:   cfg.ContainerIPNet.IPv4.IP,
			Mask: net.CIDRMask(32, 32),
		},
		IifName: cfg.HostVETHName,
	}, netlink.RT_FILTER_TABLE|netlink.RT_FILTER_SRC|netlink.RT_FILTER_PRIORITY|netlink.RT_FILTER_IIF)
	assert.NoError(t, err)
	assert.Equal(t, len(rules), 1)

	// add some ip rule make sure we don't delete it
	dummyRule := netlink.NewRule()
	dummyRule.Priority = toContainerPriority
	dummyRule.Table = unix.RT_TABLE_MAIN
	dummyRule.Dst = &net.IPNet{
		IP:   cfg.ContainerIPNet.IPv4.IP,
		Mask: net.CIDRMask(24, 32),
	}
	err = netlink.RuleAdd(dummyRule)
	assert.NoError(t, err)
	// tear down

	err = d.Teardown(&TeardownCfg{
		HostVETHName:    cfg.HostVETHName,
		ContainerIfName: cfg.ContainerIfName,
		ContainerIPNet:  nil,
	}, containerNS)

	_, err = netlink.LinkByName(cfg.HostVETHName)
	assert.Error(t, err)
	_, ok = err.(netlink.LinkNotFoundError)
	assert.True(t, ok)

	rules, err = netlink.RuleList(netlink.FAMILY_V4)
	assert.NoError(t, err)
	assert.Equal(t, 4, len(rules))
}

func FindIP(link netlink.Link, ipNetSet *terwayTypes.IPNetSet) (bool, error) {
	exec := func(ip net.IP, family int) (bool, error) {
		addrList, err := netlink.AddrList(link, family)
		if err != nil {
			return false, err
		}
		for _, addr := range addrList {
			if addr.IP.Equal(ip) {
				return true, nil
			}
		}
		return false, nil
	}
	if ipNetSet.IPv4 != nil {
		ok, err := exec(ipNetSet.IPv4.IP, netlink.FAMILY_V4)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	if ipNetSet.IPv6 != nil {
		ok, err := exec(ipNetSet.IPv6.IP, netlink.FAMILY_V6)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}
