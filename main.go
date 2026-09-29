package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// All interfaces, so a published Docker port can reach the proxy.
	// Local clients still connect to localhost:2022.
	proxyPort := ":2022"
	configPathFlag := flag.String("cfg", "./config.yaml", "path for config file (.yaml)")
	flag.Parse()

	proxy, err := NewProxy(*configPathFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Creation of new proxy errored: %v\n", err)
		os.Exit(1)
	}

	terminationCh := make(chan os.Signal, 1)
	signal.Notify(terminationCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := proxy.Start(proxyPort); err != nil {
			fmt.Fprintf(os.Stderr, "Proxy exited: %v\n", err)
		}
	}()

	<-terminationCh

	proxy.Stop()
}
