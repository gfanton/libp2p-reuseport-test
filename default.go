package main

import libp2p "github.com/libp2p/go-libp2p"

// main opts
var defaultP2POpts = []libp2p.Option{
	libp2p.RandomIdentity,
	libp2p.DefaultSecurity,
	libp2p.DefaultMuxers,
	libp2p.DefaultPeerstore,
}
