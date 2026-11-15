# zeroconf, patched for phoneset

A copy of github.com/grandcat/zeroconf v1.0.0 (MIT, see LICENSE) with one change, in `server.go`
(`writeMulticast4`, marked PATCH): on Windows the interface a multicast packet leaves by is chosen explicitly with
`IP_MULTICAST_IF` before every send. golang.org/x/net/ipv4 ignores the control message there, so without it the
packets leave by whichever interface the system prefers for 224.0.0.0/4 (often a WSL, VMware or VPN adapter on a
machine that has them) and a phone on the Wi-Fi never receives the announcements or the answers.

`pc/go.mod` points to this directory with a `replace` line. Drop the copy when upstream has the fix.
