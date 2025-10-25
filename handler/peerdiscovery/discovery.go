package peerdiscovery

import (
	"beembridge-go/handler/framingprotocol"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"sync"
	"time"

	"golang.org/x/net/ipv4"
)

const (
	multicastAddr   = "224.0.0.251:9999"     // Multicast group & port
	broadcastPeriod = 500 * time.Millisecond // How often to announce ourselves
	cleanupInterval = 1000 * time.Millisecond
	peerTimeout     = 1500 * time.Millisecond // Peer removed if not seen in this time
)

// ---- Data Structures ----

// DiscoveryMessage is what each peer sends and receives via multicast.
type DiscoveryMessage struct {
	AppID      string `json:"appId"`
	InstanceID string `json:"instanceId"`
	PeerName   string `json:"peerName"`
	TCPPort    int    `json:"tcpPort"`
	Timestamp  int64  `json:"timestamp"`
}

// DiscoveredPeer represents another peer currently known.
type DiscoveredPeer struct {
	DiscoveryMessage
	LastSeen time.Time `json:"lastSeen"`
	IP       string    `json:"ipAddress"`
}

// PeerDiscovery is the main struct encapsulating discovery logic.
type PeerDiscovery struct {
	appID      string
	instanceID string
	PeerName   string
	tcpPort    int
	multicast  *net.UDPAddr
	conn       *net.UDPConn

	mu      sync.Mutex
	peers   map[string]DiscoveredPeer
	stopCh  chan struct{}
	running bool
}

// ---- Constructor ----

// bindToRandomPort tries to find and bind to an available TCP port.
func bindToRandomPort() (int, error) {
	for {
		port := rand.Intn(55535) + 10000 // Random port between 10000 and 65535
		addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			continue // Should not happen with a valid port
		}

		l, err := net.ListenTCP("tcp", addr)
		if err == nil {
			defer l.Close()
			return l.Addr().(*net.TCPAddr).Port, nil
		}
	}
}

func NewPeerDiscovery(appID, peerName string) (*PeerDiscovery, error) {
	addr, err := net.ResolveUDPAddr("udp4", multicastAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve multicast address: %w", err)
	}

	tcpPort, err := bindToRandomPort()

	if err != nil {
		return nil, fmt.Errorf("failed to bind to a random TCP port: %w", err)
	}

	err = os.Setenv("TCP_PORT", fmt.Sprintf("%d", tcpPort))
	if err != nil {
		fmt.Println("Error setting environment variable:", err)
		return nil, err
	}

	instanceID := fmt.Sprintf("%d-%d", time.Now().UnixNano(), rand.Intn(9999))

	return &PeerDiscovery{
		appID:      appID,
		instanceID: instanceID,
		PeerName:   peerName,
		tcpPort:    tcpPort,
		multicast:  addr,
		peers:      make(map[string]DiscoveredPeer),
	}, nil
}

// ---- Core Methods ----

// Start joins the multicast group and begins broadcasting & listening.
func (pd *PeerDiscovery) Start() error {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	if pd.running {
		return nil // Already running
	}

	addr, err := net.ResolveUDPAddr("udp4", multicastAddr)
	if err != nil {
		return err
	}

	// Bind to all interfaces (0.0.0.0)
	laddr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("0.0.0.0:%d", addr.Port))
	if err != nil {
		return err
	}

	lc := net.ListenConfig{Control: setSocketOptions}

	packetConn, err := lc.ListenPacket(context.Background(), "udp4", laddr.String())
	if err != nil {
		return fmt.Errorf("failed to listen on UDP port: %w", err)
	}

	conn := packetConn.(*net.UDPConn)

	p := ipv4.NewPacketConn(conn)
	p.SetMulticastLoopback(true)
	p.SetMulticastTTL(1)

	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagMulticast != 0 && iface.Flags&net.FlagUp != 0 {
			err := p.JoinGroup(&iface, &net.UDPAddr{IP: addr.IP})
			if err != nil {
				log.Printf("[Discovery] Failed to join on %s: %v", iface.Name, err)
			} else {
				log.Printf("[Discovery] Joined multicast group on %s", iface.Name)
			}
		}
	}

	pd.stopCh = make(chan struct{})
	pd.conn = conn
	pd.running = true
	pd.conn.SetReadBuffer(2048)

	go pd.listen()
	go pd.broadcastLoop()
	go pd.cleanupLoop()

	log.Printf("[Discovery] %s started on %s\n", pd.PeerName, multicastAddr)
	return nil
}

// Stop gracefully stops all discovery goroutines.
func (pd *PeerDiscovery) Stop() {
	pd.mu.Lock()
	if !pd.running {
		pd.mu.Unlock()
		return
	}

	close(pd.stopCh)
	pd.running = false

	// Clear peers
	pd.peers = make(map[string]DiscoveredPeer)

	connToClose := pd.conn
	pd.conn = nil

	pd.mu.Unlock()

	if connToClose != nil {
		connToClose.Close()
	}
	log.Printf("[Discovery] %s stopped\n", pd.PeerName)
}

// ---- Broadcast & Listen Logic ----

// broadcastLoop sends discovery messages periodically.
func (pd *PeerDiscovery) broadcastLoop() {
	ticker := time.NewTicker(broadcastPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-pd.stopCh:
			return
		case <-ticker.C:
			pd.broadcastPresence()
		}
	}
}

// broadcastPresence sends one multicast message announcing this peer.
func (pd *PeerDiscovery) broadcastPresence() {
	msg := DiscoveryMessage{
		AppID:      pd.appID,
		InstanceID: pd.instanceID,
		PeerName:   pd.PeerName,
		TCPPort:    pd.tcpPort,
		Timestamp:  time.Now().Unix(),
	}

	data, _ := framingprotocol.BuildFramedMessage(msg, nil)

	_, err := pd.conn.WriteToUDP(data, pd.multicast)
	if err != nil {
		log.Printf("[Discovery] Error broadcasting from %s: %v\n", pd.PeerName, err)
	}
}

// listen receives multicast packets and updates peer list.
func (pd *PeerDiscovery) listen() {
	buf := make([]byte, 2048)
	parser := framingprotocol.NewFrameParser()

	for {
		n, src, err := pd.conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-pd.stopCh:
				return
			default:
				log.Printf("[Discovery] Read error: %v\n", err)
				continue
			}
		}

		messages, err := parser.Feed(buf[:n])
		if err != nil {
			log.Printf("[Discovery] Parse error: %v\n", err)
			parser.Reset()
			continue
		}

		for _, framed := range messages {
			headerBytes, _ := json.Marshal(framed.Header)
			var msg DiscoveryMessage
			if err := json.Unmarshal(headerBytes, &msg); err != nil {
				continue
			}

			if msg.InstanceID == pd.instanceID || msg.AppID != pd.appID {
				continue
			}

			pd.mu.Lock()
			pd.peers[msg.InstanceID] = DiscoveredPeer{
				DiscoveryMessage: msg,
				LastSeen:         time.Now(),
				IP:               src.IP.String(),
			}
			pd.mu.Unlock()

			log.Printf("[Discovery] %s discovered peer %s @ %s:%d\n", pd.PeerName, msg.PeerName, src.IP, msg.TCPPort)
		}

		// Ignore messages from ourselves or other apps
	}
}

// cleanupLoop removes peers that have not been seen for a while.
func (pd *PeerDiscovery) cleanupLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pd.stopCh:
			return
		case <-ticker.C:
			pd.cleanupPeers()
		}
	}
}

// cleanupPeers removes stale peers.
func (pd *PeerDiscovery) cleanupPeers() {
	now := time.Now()
	pd.mu.Lock()
	defer pd.mu.Unlock()

	for id, peer := range pd.peers {
		if now.Sub(peer.LastSeen) > peerTimeout {
			log.Printf("[Discovery] %s lost peer %s (%s)\n", pd.PeerName, peer.PeerName, peer.IP)
			delete(pd.peers, id)
		}
	}
}

// ---- Utility ----

// GetPeers returns a slice of currently known peers.
func (pd *PeerDiscovery) GetPeers() []DiscoveredPeer {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	peers := make([]DiscoveredPeer, 0, len(pd.peers))
	for _, p := range pd.peers {
		peers = append(peers, p)
	}
	return peers
}

// GetInstanceID returns the instance ID of the peer.
func (pd *PeerDiscovery) GetInstanceID() string {
	return pd.instanceID
}
