package peerconnection

import (
	"beembridge-go/handler/framingprotocol"
	"beembridge-go/handler/peerdiscovery"
	"beembridge-go/handler/transfer"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

// ==== Constants ====

const (
	AppID             = "MyAwesomeFileTransferApp"
	ConnectionTimeout = 5 * time.Second
)

// ==== Message Types ====

type BaseMessage struct {
	Type           string `json:"type"`
	SenderInstance string `json:"senderInstanceId"`
	SenderName     string `json:"senderPeerName"`
	Timestamp      int64  `json:"timestamp"`
}

type ConnectionRequestMessage struct {
	BaseMessage
	TCPPort int `json:"senderTcpPort"`
}

type ConnectionAcceptMessage struct {
	BaseMessage
}

type ConnectionRejectMessage struct {
	BaseMessage
	Reason string `json:"reason"`
}

// ==== Central Connection Manager ====

type ManagedConnection struct {
	conn            net.Conn
	parser          *framingprotocol.FrameParser
	pc              *PeerConnection
	peer            peerdiscovery.DiscoveredPeer
	writeMutex      sync.Mutex
	activeSenders   sync.Map // map[string]transfer.Sender
	activeReceivers sync.Map // map[string]transfer.Receiver
}

func NewManagedConnection(conn net.Conn, pc *PeerConnection, peer peerdiscovery.DiscoveredPeer) *ManagedConnection {
	return &ManagedConnection{
		conn:   conn,
		parser: framingprotocol.NewFrameParser(),
		pc:     pc,
		peer:   peer,
	}
}

// run is the single, centralized reader loop for a connection.
func (mc *ManagedConnection) run() {
	defer mc.conn.Close()
	defer mc.cleanup()

	buffer := make([]byte, 4096)
	for {
		n, err := mc.conn.Read(buffer)
		if err != nil {
			log.Printf("[ManagedConnection] Read error on connection: %v. Terminating.", err)
			return // This will trigger the deferred cleanup
		}

		messages, err := mc.parser.Feed(buffer[:n])
		if err != nil {
			log.Printf("[ManagedConnection] Frame parse error: %v", err)
			mc.parser.Reset()
			continue
		}

		for _, msg := range messages {
			mc.dispatchMessage(msg)
		}
	}
}

func (mc *ManagedConnection) dispatchMessage(msg framingprotocol.FramedMessage) {
	var base transfer.BaseTransferMessage
	headerBytes, _ := json.Marshal(msg.Header)
	json.Unmarshal(headerBytes, &base)

	if base.FileID == "" {
		log.Printf("[ManagedConnection] Received message with no FileID. Ignoring.")
		return
	}

	// Route based on message type
	switch base.Type {
	case "FILE_METADATA":
		// This is a new incoming file transfer. Create a receiver.
		log.Printf("[ManagedConnection] Received metadata for new transfer %s. Creating receiver.", base.FileID)
		receiver := transfer.NewReceiver(
			mc, // Pass the connection manager
			base.FileID,
			mc.pc.downloadDir,
			mc.peer,
			mc.pc.instanceID,
			mc.pc.peerName,
			mc.pc.transferCallbacks.OnProgress,
			mc.pc.transferCallbacks.OnComplete,
			mc.pc.transferCallbacks.OnError,
			mc.pc.transferCallbacks.OnHashingProgress,
			mc.pc.transferCallbacks.RequestAcceptance,
		)
		// Store a pointer to the receiver to avoid copying sync.Map
		mc.activeReceivers.Store(base.FileID, &receiver)
		receiver.HandleMessage(msg.Header, msg.Payload)

	case "FILE_CHUNK", "FILE_END":
		// Route to an existing receiver
		if r, ok := mc.activeReceivers.Load(base.FileID); ok {
			// Assert as a pointer
			r.(*transfer.Receiver).HandleMessage(msg.Header, msg.Payload)
		} else {
			log.Printf("[ManagedConnection] Received %s for unknown receiver %s. Ignoring.", base.Type, base.FileID)
		}

	case "FILE_METADATA_ACK", "FILE_CHUNK_ACK", "QUEUE_FULL", "QUEUE_FREE", "TRANSFER_ERROR":
		// Route to an existing sender
		if s, ok := mc.activeSenders.Load(base.FileID); ok {
			// Assert as a pointer
			s.(*transfer.Sender).HandleMessage(msg.Header)
		} else {
			log.Printf("[ManagedConnection] Received ACK/status for unknown sender %s. Ignoring.", base.FileID)
		}

	default:
		log.Printf("[ManagedConnection] Received unhandled message type: %s", base.Type)
	}
}

func (mc *ManagedConnection) cleanup() {
	// Notify all active senders and receivers that the connection is dead
	err := errors.New("connection closed")
	mc.activeSenders.Range(func(key, value interface{}) bool {
		// Assert as a pointer
		value.(*transfer.Sender).HandleConnectionError(err)
		return true
	})
	mc.activeReceivers.Range(func(key, value interface{}) bool {
		// Assert as a pointer
		value.(*transfer.Receiver).HandleConnectionError(err)
		return true
	})
}

// --- transfer.ConnectionManager implementation ---

func (mc *ManagedConnection) Write(data []byte) (int, error) {
	mc.writeMutex.Lock()
	defer mc.writeMutex.Unlock()
	return mc.conn.Write(data)
}

// RegisterSender now accepts a pointer to avoid copying the lock value.
func (mc *ManagedConnection) RegisterSender(sender *transfer.Sender) {
	mc.activeSenders.Store(sender.GetFileID(), sender)
}

func (mc *ManagedConnection) DeregisterSender(fileID string) {
	mc.activeSenders.Delete(fileID)
}

func (mc *ManagedConnection) DeregisterReceiver(fileID string) {
	mc.activeReceivers.Delete(fileID)
}

// ==== PeerConnection Struct ====

type PeerConnection struct {
	instanceID string
	peerName   string
	tcpPort    int

	activeConnections sync.Map // map[string]*ManagedConnection

	onConnectionRequest func(peer peerdiscovery.DiscoveredPeer, respond func(accept bool, reason string))
	onPeerConnected     func(peer peerdiscovery.DiscoveredPeer)
	onConnectionStatus  func(peer peerdiscovery.DiscoveredPeer, status string)

	// NEW FIELDS
	downloadDir       string
	transferCallbacks *transfer.TransferCallbacks
}

// ==== Constructor ====

func NewPeerConnection(instanceID, peerName string, tcpPort int, downloadDir string, callbacks *transfer.TransferCallbacks) *PeerConnection {
	return &PeerConnection{
		instanceID:        instanceID,
		peerName:          peerName,
		tcpPort:           tcpPort,
		downloadDir:       downloadDir,
		transferCallbacks: callbacks,
	}
}

// ==== Public API ====

func (pc *PeerConnection) OnConnectionRequest(callback func(peer peerdiscovery.DiscoveredPeer, respond func(accept bool, reason string))) {
	pc.onConnectionRequest = callback
}

func (pc *PeerConnection) OnPeerConnected(callback func(peer peerdiscovery.DiscoveredPeer)) {
	pc.onPeerConnected = callback
}

func (pc *PeerConnection) OnConnectionStatus(callback func(peer peerdiscovery.DiscoveredPeer, status string)) {
	pc.onConnectionStatus = callback
}

// NEW: Method to start a file transfer
func (pc *PeerConnection) InitiateFileTransfer(peerID string, filePath string, parentId string, prefix string, rootDir string) error {
	val, ok := pc.activeConnections.Load(peerID)
	if !ok {
		return fmt.Errorf("no active connection found for peer %s", peerID)
	}
	mc := val.(*ManagedConnection)

	log.Printf("[Connection] Initiating file transfer of %s to peer %s", filePath, peerID)

	sender := transfer.NewSender(
		mc, // Pass the managed connection
		filePath,
		pc.instanceID,
		pc.peerName,
		pc.transferCallbacks.OnProgress,
		pc.transferCallbacks.OnComplete,
		pc.transferCallbacks.OnError,
		pc.transferCallbacks.OnHashingProgress,
		parentId,
		prefix,
		rootDir,
	)
	sender.Start()

	return nil
}

// ==== TCP Server ====

func (pc *PeerConnection) StartTCPServer() error {
	addr := fmt.Sprintf(":%d", pc.tcpPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start TCP server: %w", err)
	}

	log.Printf("[Connection] Listening on %s", addr)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Printf("[Connection] Accept error: %v", err)
				continue
			}
			go pc.handleIncomingConnection(conn)
		}
	}()

	return nil
}

func (pc *PeerConnection) handleIncomingConnection(conn net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[Connection] Panic recovered: %v", r)
		}
	}()

	parser := framingprotocol.NewFrameParser()
	buf := make([]byte, 4096) // Increased buffer size

	// Set a deadline for the initial handshake message
	conn.SetReadDeadline(time.Now().Add(ConnectionTimeout * 2))
	n, err := conn.Read(buf)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Printf("[Connection] Timeout waiting for handshake from %s", conn.RemoteAddr())
		} else {
			log.Printf("[Connection] Handshake read error from %s: %v", conn.RemoteAddr(), err)
		}
		conn.Close()
		return
	}
	// Clear the deadline after the first read
	conn.SetReadDeadline(time.Time{})

	messages, err := parser.Feed(buf[:n])
	if err != nil || len(messages) == 0 {
		log.Printf("[Connection] Frame parse error during handshake: %v", err)
		conn.Close()
		return
	}

	// The first message on a new connection must be a CONNECTION_REQUEST
	framed := messages[0]
	data, _ := json.Marshal(framed.Header)
	var msg BaseMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("[Connection] Failed to parse base message during handshake: %v", err)
		conn.Close()
		return
	}

	if msg.Type != "CONNECTION_REQUEST" {
		log.Printf("[Connection] Unexpected message type on new connection: %s. Closing.", msg.Type)
		conn.Close()
		return
	}

	var req ConnectionRequestMessage
	_ = json.Unmarshal(data, &req)

	// Reconstruct peer info from the request
	remoteTCPAddr, ok := conn.RemoteAddr().(*net.TCPAddr)
	if !ok {
		log.Printf("[Connection] Could not get TCP address from remote connection. Closing.")
		conn.Close()
		return
	}
	peer := peerdiscovery.DiscoveredPeer{
		DiscoveryMessage: peerdiscovery.DiscoveryMessage{
			AppID:      AppID,
			InstanceID: req.SenderInstance,
			PeerName:   req.SenderName,
			TCPPort:    req.TCPPort,
		},
		IP: remoteTCPAddr.IP.String(),
	}

	if pc.onConnectionRequest != nil {
		// The callback will decide to accept or reject.
		pc.onConnectionRequest(peer, func(accept bool, reason string) {
			if accept {
				resp := ConnectionAcceptMessage{
					BaseMessage: BaseMessage{
						Type:           "CONNECTION_ACCEPT",
						SenderInstance: pc.instanceID,
						SenderName:     pc.peerName,
						Timestamp:      time.Now().Unix(),
					},
				}
				data, _ := framingprotocol.BuildFramedMessage(resp, nil)
				_, err := conn.Write(data)
				if err != nil {
					log.Printf("[Connection] Error sending ACCEPT to %s: %v", peer.PeerName, err)
					conn.Close()
					return
				}

				mc := NewManagedConnection(conn, pc, peer)
				pc.activeConnections.Store(peer.InstanceID, mc)

				if pc.onPeerConnected != nil {
					pc.onPeerConnected(peer)
				}

				log.Printf("[Connection] Connection accepted for %s. Starting reader loop.", peer.PeerName)
				go mc.run() // Start the single reader loop
			} else {
				pc.rejectConnection(conn, reason)
			}
		})
	} else {
		// Auto-reject if no handler is present
		pc.rejectConnection(conn, "No connection handler registered")
	}
}

// ==== TCP Client ====

func (pc *PeerConnection) ConnectToPeer(peer peerdiscovery.DiscoveredPeer) error {
	addr := fmt.Sprintf("%s:%d", peer.IP, peer.TCPPort)
	conn, err := net.DialTimeout("tcp", addr, ConnectionTimeout)
	if err != nil {
		if pc.onConnectionStatus != nil {
			pc.onConnectionStatus(peer, "failed")
		}
		return fmt.Errorf("failed to connect to peer %s: %w", peer.PeerName, err)
	}

	// Send connection request
	req := ConnectionRequestMessage{
		BaseMessage: BaseMessage{
			Type:           "CONNECTION_REQUEST",
			SenderInstance: pc.instanceID,
			SenderName:     pc.peerName,
			Timestamp:      time.Now().Unix(),
		},
		TCPPort: pc.tcpPort,
	}

	data, _ := framingprotocol.BuildFramedMessage(req, nil)
	_, _ = conn.Write(data)

	// Wait for response
	parser := framingprotocol.NewFrameParser()
	buf := make([]byte, 2048)
	conn.SetReadDeadline(time.Now().Add(ConnectionTimeout))

	n, err := conn.Read(buf)
	if err != nil {
		conn.Close()
		if pc.onConnectionStatus != nil {
			pc.onConnectionStatus(peer, "timeout")
		}
		return err
	}

	// Clear the deadline after the first read
	conn.SetReadDeadline(time.Time{})
	messages, err := parser.Feed(buf[:n])
	log.Println("[Sender] Received Connection Response Message From Receiver")
	if err != nil {
		conn.Close()
		return err
	}

	for _, framed := range messages {
		data, _ := json.Marshal(framed.Header)
		switch framed.Header["type"] {
		case "CONNECTION_ACCEPT":
			mc := NewManagedConnection(conn, pc, peer)
			pc.activeConnections.Store(peer.InstanceID, mc)

			if pc.onPeerConnected != nil {
				pc.onPeerConnected(peer)
			}

			log.Printf("[Connection] Connection to %s accepted. Starting reader loop.", peer.PeerName)
			go mc.run() // Start the single reader loop

			return nil
		case "CONNECTION_REJECT":
			var rej ConnectionRejectMessage
			_ = json.Unmarshal(data, &rej)
			conn.Close()
			return errors.New(rej.Reason)
		}
	}

	conn.Close()
	return errors.New("invalid response from peer")
}

// ==== Connection Helpers ====

func (pc *PeerConnection) rejectConnection(conn net.Conn, reason string) {
	resp := ConnectionRejectMessage{
		BaseMessage: BaseMessage{
			Type:           "CONNECTION_REJECT",
			SenderInstance: pc.instanceID,
			SenderName:     pc.peerName,
			Timestamp:      time.Now().Unix(),
		},
		Reason: reason,
	}
	data, _ := framingprotocol.BuildFramedMessage(resp, nil)
	_, _ = conn.Write(data)
	conn.Close()
}

// ==== Utility ====

func (pc *PeerConnection) GetConnectedPeers() []peerdiscovery.DiscoveredPeer {
	var peers []peerdiscovery.DiscoveredPeer
	pc.activeConnections.Range(func(_, value interface{}) bool {
		mc := value.(*ManagedConnection)
		// This is incomplete, but matches original behavior of only returning IP
		peers = append(peers, peerdiscovery.DiscoveredPeer{
			IP: mc.peer.IP,
		})
		return true
	})
	return peers
}
