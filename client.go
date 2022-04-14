package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	host "github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	quic "github.com/libp2p/go-libp2p-quic-transport"
	"github.com/libp2p/go-libp2p/p2p/protocol/ping"
	"github.com/libp2p/go-tcp-transport"
	"github.com/multiformats/go-multiaddr"
)

// options

type clientOptions struct {
	reuseport  bool
	stdin      bool
	targets    string
	duration   time.Duration
	interval   time.Duration
	transports string
	listeners  string
}

var clientFlags = flag.NewFlagSet("client", flag.ExitOnError)

func (opts *clientOptions) Parse(args ...string) (err error) {
	clientFlags.StringVar(&opts.transports, "tpt", "both", "transport to listen, can be `tcp,quic,both")
	clientFlags.BoolVar(&opts.stdin, "stdin", false, "read a list of addrs from stdin")
	clientFlags.StringVar(&opts.targets, "targets", "", "list of addrs to dials")
	clientFlags.StringVar(&opts.listeners, "l", "/ip4/0.0.0.0/tcp/0,/ip4/0.0.0.0/udp/0/quic", "list of listeners")
	clientFlags.DurationVar(&opts.duration, "duration", time.Minute, "run duration")
	clientFlags.DurationVar(&opts.interval, "i", time.Second, "ping interval")
	clientFlags.BoolVar(&opts.reuseport, "reuseport", false, "enable reuseport (on tcp transport")

	// parse args
	if err := clientFlags.Parse(args); err != nil {
		return fmt.Errorf("unable to parse args: %w", err)
	}

	if !opts.stdin && opts.targets == "" {
		fmt.Printf("no targets given, reading from stdin\n")
		opts.stdin = true
	}

	if opts.reuseport && !tcp.ReuseportIsAvailable() {
		log.Println("WARNING: reuseport enable but not available on this device")
	}

	return
}

func runClient(ctx context.Context, args ...string) error {
	var sopts clientOptions
	if err := sopts.Parse(args...); err != nil {
		return fmt.Errorf("unable to parse options: %w", err)
	}

	h, err := generateClientHost(&sopts)
	if err != nil {
		return fmt.Errorf("failed to generate hosts: %w", err)
	}
	defer h.Close()

	logger.Printf("client host id: %s\n", h.ID().String())

	targets, err := generateTargets(&sopts)
	if err != nil {
		return fmt.Errorf("failed to generate targets: %w", err)
	}

	return pingTargets(ctx, &sopts, h, targets)
}

func pingTargets(ctx context.Context, opts *clientOptions, h host.Host, targets []*peer.AddrInfo) error {
	ctx, cancel := context.WithTimeout(ctx, opts.duration)
	defer cancel()
	fmt.Printf("starting ping process for %d target(s) for %ds\n", len(targets), int64(opts.duration.Seconds()))

	ps := ping.NewPingService(h)
	alive := int64(len(targets))

	var wg sync.WaitGroup
	for i, target := range targets {
		h.Peerstore().AddAddrs(target.ID, target.Addrs, opts.duration)

		wg.Add(1)
		go func(index int, target *peer.AddrInfo) {
			defer wg.Done()

			pingcounter := 0

			var err error
			if err = h.Connect(ctx, *target); err == nil {
				if conns := h.Network().ConnsToPeer(target.ID); len(conns) > 0 {
					for _, conn := range conns {
						logger.Printf("[%d] connected to target(%s) %s -> %s", index, target.ID.ShortString(),
							conn.LocalMultiaddr().String(), conn.RemoteMultiaddr().String())
					}
				}
			}

			fmt.Printf("[%d] [START] - ping process, target(%v)\n", index, target.Loggable())

			for err == nil {
				cp := ps.Ping(ctx, target.ID)

				select {
				case ret := <-cp:
					if err = ret.Error; err == nil {
						pingcounter++
						logger.Printf("[%d] receiving ping, RTT(%dms) ping(%d) target(%s)",
							index, ret.RTT.Milliseconds(), pingcounter, target.ID.String())

						// wait before next ping
						<-time.After(opts.interval)
					}
				case <-time.After(time.Second * 10):
					err = fmt.Errorf("timeout while waiting for ping response")
				case <-ctx.Done():
					err = ctx.Err()
				}
			}

			select {
			case <-ctx.Done():
				fmt.Printf("[%d] [SUCCESS] - ping(%d), target(%s)\n", index, pingcounter, target.ID.String())
			default:
				left := atomic.AddInt64(&alive, ^int64(0))
				fmt.Printf("[%d] [ERROR] - peers_left(%d), ping(%d), target(%v) : %s\n",
					left, index, pingcounter, target.ID.String(), err.Error())
			}
		}(i, target)
	}

	wg.Wait()

	fmt.Printf("process done with [%d/%d] peers alive \n", alive, len(targets))

	if alive == 0 {
		return fmt.Errorf("no peers left")
	}

	return nil
}

func generateTargets(s *clientOptions) ([]*peer.AddrInfo, error) {
	targets := []string{}
	if s.targets != "" {
		targets = strings.Split(s.targets, ",")
	}

	pis := make([]*peer.AddrInfo, len(targets))
	for i, target := range targets {
		if target == "" {
			continue
		}

		m, err := multiaddr.NewMultiaddr(target)
		if err != nil {
			return nil, fmt.Errorf("- unable to parse multiaddr `%s`: %s", target, err.Error())
		}

		pi, err := peer.AddrInfoFromP2pAddr(m)
		if err != nil {
			return nil, fmt.Errorf("unable to get info from p2p addr(%s): %w", m.String(), err)
		}

		pis[i] = pi
	}

	// if stdin is enable read all addrs from there, and append it to our list
	if s.stdin {
		logger.Println("reading stdin")
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := scanner.Text()
			m, err := multiaddr.NewMultiaddr(line)
			if err != nil {
				// skip unmatching line
				continue
			}

			pi, err := peer.AddrInfoFromP2pAddr(m)
			if err != nil {
				return nil, fmt.Errorf("unable to get info from p2p addr(%s): %w", m.String(), err)
			}

			pis = append(pis, pi)
		}

		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("unable to parse stdin: %w", err)
		}
	}

	logger.Printf("successfully parsed %d peers", len(pis))

	return pis, nil
}

func generateClientHost(s *clientOptions) (host.Host, error) {
	tcpOpts := []interface{}{}
	if !s.reuseport {
		tcpOpts = append(tcpOpts, tcp.DisableReuseport())
	} else {
		logger.Println("reuseport enable")
	}

	opts := append(defaultP2POpts,
		// add transports
		libp2p.Transport(tcp.NewTCPTransport, tcpOpts...),
		libp2p.Transport(quic.NewTransport),
	)

	if s.listeners != "" {
		listeners := strings.Split(s.listeners, ",")
		opts = append(opts, libp2p.ListenAddrStrings(listeners...))
	}

	// generate host
	return libp2p.NewWithoutDefaults(opts...)
}
