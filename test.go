package main

import (
	"beembridge-go/handler/peerdiscovery"
	"fmt"
	"log"
	"os"
	"time"
)

func test_discovery() {
	hostname, _ := os.Hostname()

	// Create a discovery instance
	pd, err := peerdiscovery.NewPeerDiscovery("myApp", hostname)
	if err != nil {
		log.Fatal(err)
	}

	// Start discovery
	if err := pd.Start(); err != nil {
		log.Fatal(err)
	}

	// Print discovered peers every 1 second
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	go func() {
		for range ticker.C {
			peers := pd.GetPeers()
			fmt.Printf("\n[%s] Known peers (%d):\n", pd.PeerName, len(peers))
			for _, peer := range peers {
				fmt.Printf(" - %s @ %s:%d\n", peer.PeerName, peer.IP, peer.TCPPort)
			}
		}
	}()

	// Keep running for demo
	time.Sleep(30 * time.Second)
	pd.Stop()
}
