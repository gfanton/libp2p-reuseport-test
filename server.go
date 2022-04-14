package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"

	libp2p "github.com/libp2p/go-libp2p"
	host "github.com/libp2p/go-libp2p-core/host"
	quic "github.com/libp2p/go-libp2p-quic-transport"
	"github.com/libp2p/go-tcp-transport"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

type tptMask string

const (
	tcpTptMask  tptMask = "/tcp/%d"
	quicTptMask tptMask = "/udp/%d/quic"
)

// options

type serverOptions struct {
	addr         string
	nserver      int
	startingPort int
	transports   string
	announce     string
}

var serverFlags = flag.NewFlagSet("server", flag.ExitOnError)

func (opts *serverOptions) getMask() (ms []tptMask, err error) {
	switch opts.transports {
	case "quic":
		ms = []tptMask{quicTptMask}
	case "tcp":
		ms = []tptMask{tcpTptMask}
	case "both":
		ms = []tptMask{tcpTptMask, quicTptMask}
	default:
		err = fmt.Errorf("unknown transport given: `%s`", opts.transports)
	}

	return
}

func (opts *serverOptions) Parse(args ...string) (err error) {
	serverFlags.StringVar(&opts.transports, "tpt", "tcp", "transport to listen, can be `tcp,quic,both")
	serverFlags.StringVar(&opts.announce, "announce", "", "announce addr")
	serverFlags.StringVar(&opts.addr, "l", "0.0.0.0", "listen addr")
	serverFlags.IntVar(&opts.nserver, "n", 1, "number of server to serve")
	serverFlags.IntVar(&opts.startingPort, "pstart", 4242, "starting port to serve")

	// parse args
	if err := serverFlags.Parse(args); err != nil {
		return fmt.Errorf("unable to parse args: %w", err)
	}

	if opts.addr == "" || opts.nserver <= 0 || opts.startingPort <= 0 {
		err = fmt.Errorf("invalid argument(s)")
	}

	return
}

func (m tptMask) Cast(port int) multiaddr.Multiaddr {
	return multiaddr.StringCast(fmt.Sprintf(string(m), port))
}

// run server

func runServer(ctx context.Context, args ...string) error {
	var sopts serverOptions
	if err := sopts.Parse(args...); err != nil {
		return fmt.Errorf("unable to parse options: %w", err)
	}

	hs, err := generateServerHosts(&sopts)
	if err != nil {
		return fmt.Errorf("failed to generate hosts: %w", err)
	}
	defer hs.Close()

	// wait until context is done
	<-ctx.Done()
	return ctx.Err()
}

type hostSlice []host.Host

func (s hostSlice) Hosts() []host.Host { return s }

func (s hostSlice) Close() {
	for _, h := range s {
		h.Close()
	}
}

func generateServerHosts(s *serverOptions) (hostSlice, error) {
	opts := append(defaultP2POpts,
		// add transports
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Transport(quic.NewTransport),
	)

	// get listen addr
	laddr, err := multiaddr.NewMultiaddr(s.addr)
	if err != nil {
		laddr, err = manet.FromIP(net.ParseIP(s.addr))
		if err != nil {
			return nil, fmt.Errorf("unable to resolve ip `%s`: %w", s.addr, err)
		}
	}

	var target multiaddr.Multiaddr
	if s.announce != "" {
		target, err = multiaddr.NewMultiaddr(s.announce)
		if err != nil {
			target, err = manet.FromIP(net.ParseIP(s.announce))
			if err != nil {
				return nil, fmt.Errorf("unable to resolve ip `%s`: %w", s.announce, err)
			}
		}
	}

	// get transport
	masks, err := s.getMask()
	if err != nil {
		return nil, err
	}

	// generate hosts
	logger.Printf("generating %d host, from port %d -> %d \n", s.nserver, s.startingPort, s.startingPort+s.nserver)

	hosts := make([]host.Host, s.nserver)
	for n := 0; n < s.nserver; n++ {
		laddrs := make([]multiaddr.Multiaddr, len(masks))
		for i, t := range masks {
			laddrs[i] = laddr.Encapsulate(t.Cast(s.startingPort + n))
		}
		copts := append(opts, libp2p.ListenAddrs(laddrs...))

		if hosts[n], err = libp2p.NewWithoutDefaults(copts...); err != nil {
			return nil, fmt.Errorf("unable to init host[%d]: %w", n, err)
		}

		rawaddrs, err := json.Marshal(hosts[n].Network().ListenAddresses())
		if err == nil {
			logger.Printf("starting host[%d]: %s, listeners: %s\n", n, hosts[n].ID().String(), string(rawaddrs))
		}

		// print targets
		if target != nil {
			for _, t := range masks {
				addr := target.Encapsulate(t.Cast(s.startingPort + n))
				fmt.Printf("%s/p2p/%s\n", addr.String(), hosts[n].ID().String())
			}
		} else {
			for _, addr := range hosts[n].Addrs() {
				fmt.Printf("%s/p2p/%s\n", addr.String(), hosts[n].ID().String())
			}
		}
	}

	return hosts, nil
}
