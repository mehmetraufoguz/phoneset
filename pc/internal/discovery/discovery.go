// Package discovery finds the addresses a phone can reach this PC on and
// advertises the service over mDNS so the phone can find it from a 6-digit code.
package discovery

import (
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/grandcat/zeroconf"

	"github.com/mehmetraufoguz/phoneset/pc/internal/proto"
)

// Virtual adapters that a phone on the Wi-Fi can never reach.
var skipAdapter = []string{
	"vethernet", "virtualbox", "vmware", "vmnet", "hyper-v", "docker", "wsl",
	"loopback", "bluetooth", "tailscale", "zerotier", "npcap",
}

// Addrs lists private IPv4 addresses, the one used for the default route first.
func Addrs() []string {
	preferred := outbound()
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []string
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&(net.FlagLoopback|net.FlagPointToPoint) != 0 {
			continue
		}
		if skipped(ifc.Name) {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipnet.IP.To4()
			if ip == nil || !ip.IsPrivate() {
				continue
			}
			out = append(out, ip.String())
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i] == preferred && out[j] != preferred })
	return out
}

func skipped(name string) bool {
	n := strings.ToLower(name)
	if strings.HasPrefix(n, "bridge") { // macOS bridge100 and friends: VM and sharing networks
		return true
	}
	for _, s := range skipAdapter {
		if strings.Contains(n, s) {
			return true
		}
	}
	return false
}

// outbound returns the local address the OS would use to reach the internet.
// Dialling UDP sends nothing; it only asks the routing table.
func outbound() string {
	c, err := net.Dial("udp4", "192.0.2.1:9")
	if err != nil {
		return ""
	}
	defer c.Close()
	if a, ok := c.LocalAddr().(*net.UDPAddr); ok {
		return a.IP.String()
	}
	return ""
}

// advertiseInterfaces are the interfaces whose addresses go into the mDNS answer: the ones a phone on the
// network can reach, which are the ones Addrs lists. Without this the library answers with every address
// of every interface that can multicast, including an alias on the loopback interface (10.77.0.20 on one
// Mac) and the addresses of VPN and virtual adapters. A phone with Android 13 or older is given only one
// address by its system, so one that is not reachable ends in "Couldn't reach the PC".
func advertiseInterfaces() []net.Interface {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []net.Interface
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagMulticast == 0 ||
			ifc.Flags&(net.FlagLoopback|net.FlagPointToPoint) != 0 || skipped(ifc.Name) {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok {
				if ip := ipnet.IP.To4(); ip != nil && ip.IsPrivate() {
					out = append(out, ifc)
					break
				}
			}
		}
	}
	return out
}

// Advertise publishes _phoneset._tcp. The returned function stops it.
func Advertise(id, name string, port int) (func(), error) {
	// With no usable interface (an unusual network) the library's own choice, all of them, is kept.
	srv, err := zeroconf.Register(
		fmt.Sprintf("phoneset-%s", id),
		proto.ServiceType, "local.", port,
		[]string{fmt.Sprintf("v=%d", proto.Version), "id=" + id, "n=" + name},
		advertiseInterfaces(),
	)
	if err != nil {
		return nil, err
	}
	return srv.Shutdown, nil
}
