package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

func closeAllUDP(socks []net.PacketConn) {
	for _, sock := range socks {
		sock.Close()
	}
}

func listenAllUDP(specs []string) ([]net.PacketConn, error) {
	socks := []net.PacketConn{}
	for _, spec := range specs {
		sock, err := listenUDP(spec)
		if err != nil {
			closeAllUDP(socks)
			return nil, fmt.Errorf("%s: %w", spec, err)
		}
		socks = append(socks, sock)
	}
	return socks, nil
}

func listenUDP(spec string) (net.PacketConn, error) {
	if arg, ok := strings.CutPrefix(spec, "udp:"); ok {
		return listenUDPPort(arg)
	} else if arg, ok := strings.CutPrefix(spec, "fd:"); ok {
		return listenUDPFD(arg)
	} else {
		return nil, fmt.Errorf("invalid UDP socket specified")
	}
}

func listenUDPPort(arg string) (net.PacketConn, error) {
	var ipString string
	var portString string
	var err error

	if strings.Contains(arg, ":") {
		ipString, portString, err = net.SplitHostPort(arg)
		if err != nil {
			return nil, fmt.Errorf("UDP listener has invalid argument: %w", err)
		}
	} else {
		portString = arg
	}

	network := "udp"
	address := new(net.UDPAddr)

	if ipString != "" {
		address.IP = net.ParseIP(ipString)
		if address.IP == nil {
			return nil, fmt.Errorf("UDP listener has invalid IP address")
		}

		// Explicitly specify the IP protocol, to ensure that 0.0.0.0
		// and :: work as expected (listen only on IPv4 or IPv6 interfaces)
		if address.IP.To4() == nil {
			network = "udp6"
		} else {
			network = "udp4"
		}
	}

	address.Port, err = strconv.Atoi(portString)
	if err != nil {
		return nil, fmt.Errorf("UDP listener has invalid port: %w", err)
	}

	return net.ListenUDP(network, address)
}

func listenUDPFD(fdString string) (net.PacketConn, error) {
	fd, err := strconv.ParseUint(fdString, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("'%s' is a malformed file descriptor", fdString)
	}

	file := os.NewFile(uintptr(fd), fdString)
	defer file.Close()

	return net.FilePacketConn(file)
}
