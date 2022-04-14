# libp2p reuseport test

## build

`make build`

## run server

- quic : `./bin/reuseport server -tpt=quic -n=10 -announce=$SERVER_IP`
- tcp : `./bin/reuseport server -tpt=tcp -n=10 -announce=$SERVER_IP`

should output somehting like this: 

```
/ip4/$SERVER_IP/udp/4242/quic/p2p/QmbSuSFiemgX1xtYWUCUzJ3pvypBP9XurmzXtg3stthzde
/ip4/$SERVER_IP/udp/4243/quic/p2p/QmQtBACGhHvALDfYAKh9VSYZ3HrP19CoTAkdKYLWGPfNZB
/ip4/$SERVER_IP/udp/4244/quic/p2p/QmRM1kdqi4MrbAzt6pbyTecQwGNWboaKmfTANzYyVzP17v
/ip4/$SERVER_IP/udp/4245/quic/p2p/Qma4HsMS42ud4Jye96RuDLF6fAT9Ravr1NWCGGKy7awP3s
/ip4/$SERVER_IP/udp/4246/quic/p2p/QmesW4sqYsLWahhUDCroHNEoHUmiX5uMaW2YGJnv6wM1g7
...
```

## run client

take the output of server and past it to client stdin

`./bin/reuseport client --stdin [-reuseport]`


