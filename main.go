package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/oklog/run"
)

var (
	debug  = flag.Bool("debug", false, "debug mode")
	logger = log.Default()
)

func main() {
	offset := 2
	flag.Parse()
	if !*debug {
		logger.SetOutput(ioutil.Discard)
	} else {
		offset++ // debug flag is set
	}

	if len(os.Args) < offset {
		fmt.Printf("Usage: %s [-debug] server|client ...\n", os.Args[0])
		os.Exit(1)
	}

	var g run.Group

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, interupt := run.SignalHandler(ctx, os.Interrupt)
	g.Add(exec, interupt)

	args := os.Args[offset:]
	switch flag.Arg(0) {
	case "server":
		g.Add(func() error {
			return runServer(ctx, args...)
		}, func(error) { cancel() })
	case "client":
		g.Add(func() error {
			return runClient(ctx, args...)
		}, func(error) { cancel() })
	default:
		g.Add(func() error {
			return fmt.Errorf("unknown command: %s", flag.Arg(0))
		}, func(error) { cancel() })
	}

	if err := g.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
		os.Exit(1)
	}
}
