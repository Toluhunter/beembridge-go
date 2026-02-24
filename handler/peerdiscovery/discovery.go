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
	// one UDPConn per interface to ensure multicast egress on every NIC
	sendConns []struct {
		Iface net.Interface
		Conn  *net.UDPConn
	}

	// single receive socket bound to 0.0.0.0:<port> joined on all interfaces
	recvConn net.PacketConn

	mu      sync.Mutex
	peers   map[string]DiscoveredPeer
	stopCh  chan struct{}
	running bool
}

// ---- Constructor ----

// bindToRandomPort tries to find and bind to an available TCP port.
func bindToRandomPort() (int, error) {
	for {
		port := rand.Intn(55535) + 10000 // Random port for TCP connections (10000-65535)
		addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			continue // try another port
		}

		l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4zero, Port: addr.Port})
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

	// Create receiver socket bound to INADDR_ANY and join group on every interface
	lc := net.ListenConfig{Control: setSocketOptions}
	recvLaddr := &net.UDPAddr{IP: net.IPv4zero, Port: addr.Port}
	packetConn, err := lc.ListenPacket(context.Background(), "udp4", recvLaddr.String())
	if err != nil {
		return fmt.Errorf("failed to listen on UDP port for receiver: %w", err)
	}
	// keep the packetConn (net.PacketConn) for reads; wrap it with ipv4.PacketConn for control ops
	pRecv := ipv4.NewPacketConn(packetConn)
	pRecv.SetMulticastLoopback(true)
	pRecv.SetMulticastTTL(1)

	ifaces, _ := net.Interfaces()
	// join group for receive on all multicast-capable interfaces
	for _, iface := range ifaces {
		if iface.Flags&net.FlagMulticast == 0 || iface.Flags&net.FlagUp == 0 {
			log.Printf("[Discovery] Skipping interface %s (multicast: %v, up: %v)", iface.Name, iface.Flags&net.FlagMulticast != 0, iface.Flags&net.FlagUp != 0)
			continue
		}
		if err := pRecv.JoinGroup(&iface, &net.UDPAddr{IP: addr.IP}); err != nil {
			log.Printf("[Discovery] Failed to join multicast group for recv on %s: %v", iface.Name, err)
		} else {
			log.Printf("[Discovery] Receiver joined multicast group on %s", iface.Name)
		}
	}
	// try to set read buffer if underlying conn is a UDPConn
	if u, ok := packetConn.(*net.UDPConn); ok {
		u.SetReadBuffer(2048)
	}
	pd.recvConn = packetConn
	// start single listener for receive socket
	go pd.listenOnConn(packetConn)

	// Create per-interface send sockets bound to the interface's IPv4 address (ephemeral local port)
	pd.sendConns = nil
	for _, iface := range ifaces {
		if iface.Flags&net.FlagMulticast == 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		var ipv4Addr net.IP
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.To4() == nil {
				continue
			}
			if ip.IsLoopback() {
				continue
			}
			ipv4Addr = ip.To4()
			break
		}
		if ipv4Addr == nil {
			continue
		}
		laddr := &net.UDPAddr{IP: ipv4Addr, Port: 0}
		sendConn, err := net.ListenUDP("udp4", laddr)
		if err != nil {
			log.Printf("[Discovery] Failed to create send socket on %s: %v", iface.Name, err)
			continue
		}
		pd.sendConns = append(pd.sendConns, struct {
			Iface net.Interface
			Conn  *net.UDPConn
		}{Iface: iface, Conn: sendConn})
		log.Printf("[Discovery] Send socket created on %s (%s)", iface.Name, laddr.String())
	}

	pd.stopCh = make(chan struct{})
	pd.running = true
	if pd.recvConn == nil {
		return fmt.Errorf("no multicast receiver available")
	}
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
	conns := pd.sendConns
	pd.sendConns = nil

	pd.mu.Unlock()

	// close send sockets
	for _, c := range conns {
		if c.Conn != nil {
			c.Conn.Close()
		}
	}
	// close recv socket
	if pd.recvConn != nil {
		pd.recvConn.Close()
		pd.recvConn = nil
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

	// Send once per interface socket so the kernel uses that interface as source
	for _, c := range pd.sendConns {
		if c.Conn == nil {
			continue
		}
		_, err := c.Conn.WriteToUDP(data, pd.multicast)
		if err != nil {
			log.Printf("[Discovery] Error broadcasting from %s on %s: %v\n", pd.PeerName, c.Iface.Name, err)
		}
	}
}

// listenOnConn reads from a PacketConn and updates peers.
func (pd *PeerDiscovery) listenOnConn(conn net.PacketConn) {
	log.Println("[Discovery] Listening for peers on a connection...")
	buf := make([]byte, 2048)
	parser := framingprotocol.NewFrameParser()

	for {
		n, srcAddr, err := conn.ReadFrom(buf)
		if err != nil {
			select {
			case <-pd.stopCh:
				return
			default:
				log.Printf("[Discovery] Read error: %v\n", err)
				continue
			}
		}
		src, _ := srcAddr.(*net.UDPAddr)

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
		}
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
