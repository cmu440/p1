// This package provides wrapper around the UDP functions/methods in Go's "net" package.
// The wrapper types we provide allow for selective dropping of packets, as well as
// monitoring of packet traffic. You must not use any methods provided in Go's "net"
// package, as otherwise the course staff will be unable to test the robustness of your
// implementation.

package lspnet

import (
	"net"
	"sync"
)

var (
	// Maps UDPConn keys to bool values. If the value is true, then the connection
	// was created by a server. If the value is false, then the connection was
	// created by a client.
	connectionMap = make(map[UDPConn]bool)
	mapMutex      sync.Mutex
)

// ResolveUDPAddr behaves the same as the net.UDPAddr.ResolveUDPAddr method
// (with some additional book-keeping).
func ResolveUDPAddr(ntwk, addr string) (*UDPAddr, error) {
	a, err := net.ResolveUDPAddr(ntwk, addr)
	if err != nil {
		return nil, err
	}
	return &UDPAddr{naddr: a}, nil
}

// ListenUDP behaves the same as the net.ListenUDP method (with some
// additional book-keeping).
//
// Servers should use this method to begin listening for incoming
// client connections.
func ListenUDP(ntwk string, laddr *UDPAddr) (*UDPConn, error) {
	var nladdr *net.UDPAddr
	if laddr != nil {
		nladdr = laddr.toNet()
	}
	nconn, err := net.ListenUDP(ntwk, nladdr)
	if err != nil {
		return nil, err
	}
	conn := UDPConn{nconn: nconn}
	mapMutex.Lock()
	// Add the server connection to the map.
	connectionMap[conn] = true
	mapMutex.Unlock()
	return &conn, nil
}

// DialUDP behaves the same as the net.DialUDP method (with some additional
// book-keeping).
//
// Clients should use this method to connect to the server.
func DialUDP(ntwk string, laddr, raddr *UDPAddr) (*UDPConn, error) {
	var nladdr, nraddr *net.UDPAddr
	if laddr != nil {
		nladdr = laddr.toNet()
	}
	if raddr != nil {
		nraddr = raddr.toNet()
	}
	nconn, err := net.DialUDP(ntwk, nladdr, nraddr)
	if err != nil {
		return nil, err
	}
	conn := UDPConn{nconn: nconn}
	mapMutex.Lock()
	// Add the client connection to the map.
	connectionMap[conn] = false
	mapMutex.Unlock()
	return &conn, nil
}

// JoinHostPort behaves the same as the net.JoinHostPort function.
func JoinHostPort(host, port string) string {
	return net.JoinHostPort(host, port)
}

// SplitHostPort behaves the same as the net.SplitHostPort function.
func SplitHostPort(hostport string) (host, port string, err error) {
	return net.SplitHostPort(hostport)
}
