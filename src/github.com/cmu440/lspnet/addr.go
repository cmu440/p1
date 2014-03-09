package lspnet

import "net"

// UDPAddr is a wrapper around net.UDPAddr.
type UDPAddr struct {
	naddr *net.UDPAddr
}

func (a *UDPAddr) String() string { return a.naddr.String() }

func (a *UDPAddr) toNet() *net.UDPAddr {
	return &net.UDPAddr{IP: a.naddr.IP, Port: a.naddr.Port, Zone: a.naddr.Zone}
}
