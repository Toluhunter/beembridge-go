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

type MessageData struct {
	BaseMessage
	Content string `json:"content"`
}

// NEW: Callbacks for transfer logic
type TransferCallbacks struct {
	OnProgress        transfer.TransferProgressCallback
	OnComplete        transfer.TransferCompleteCallback
	OnError           func(fileID string, message string)
	OnHashingProgress func(progress map[string]interface{})
	RequestAcceptance func(fileID, fileName string, fileSize int64, senderPeerName string, acceptCallback func(string))
}

// ==== PeerConnection Struct ====

type PeerConnection struct {
	instanceID string
	peerName   string
	tcpPort    int

	activeConnections sync.Map // map[string]net.Conn

	onConnectionRequest func(peer peerdiscovery.DiscoveredPeer, respond func(accept bool, reason string))
	onPeerConnected     func(peer peerdiscovery.DiscoveredPeer)
	onConnectionStatus  func(peer peerdiscovery.DiscoveredPeer, status string)

	// NEW FIELDS
	downloadDir       string
	transferCallbacks *TransferCallbacks
}

// ==== Constructor ====

func NewPeerConnection(instanceID, peerName string, tcpPort int, downloadDir string, callbacks *TransferCallbacks) *PeerConnection {
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
func (pc *PeerConnection) InitiateFileTransfer(peerID string, filePath string, fileId string, parentId string, prefix string, rootDir string) error {
	val, ok := pc.activeConnections.Load(peerID)
	if !ok {
		return fmt.Errorf("no active connection found for peer %s", peerID)
	}
	conn := val.(net.Conn)

	log.Printf("[Connection] Initiating file transfer of %s for file ID %s to peer %s", filePath, fileId, peerID)

	sender := transfer.NewSender(
		conn,
		filePath,
		fileId,
		pc.instanceID,
		pc.peerName,
		pc.transferCallbacks.OnProgress,
		pc.transferCallbacks.OnComplete,
		pc.transferCallbacks.OnError,
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
				pc.acceptConnection(peer, conn)
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
			pc.storeConnection(peer, conn)
			if pc.onPeerConnected != nil {
				pc.onPeerConnected(peer)
			}
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

func (pc *PeerConnection) acceptConnection(peer peerdiscovery.DiscoveredPeer, conn net.Conn) {
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

	pc.storeConnection(peer, conn)

	if pc.onPeerConnected != nil {
		pc.onPeerConnected(peer)
	}

	// Hand off to receiver for file transfer messages
	log.Printf("[Connection] Connection accepted for %s. Handing off to transfer receiver.", peer.PeerName)
	receiver := transfer.NewReceiver(
		conn,
		pc.downloadDir,
		peer,
		pc.instanceID,
		pc.peerName,
		pc.transferCallbacks.OnProgress,
		pc.transferCallbacks.OnComplete,
		pc.transferCallbacks.OnError,
		pc.transferCallbacks.OnHashingProgress,
		pc.transferCallbacks.RequestAcceptance,
	)
	go receiver.Handle()
}

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

func (pc *PeerConnection) storeConnection(peer peerdiscovery.DiscoveredPeer, conn net.Conn) {
	pc.activeConnections.Store(peer.InstanceID, conn)
}

// ==== Utility ====

func (pc *PeerConnection) GetConnectedPeers() []peerdiscovery.DiscoveredPeer {
	var peers []peerdiscovery.DiscoveredPeer
	pc.activeConnections.Range(func(_, value interface{}) bool {
		conn := value.(net.Conn)
		addr := conn.RemoteAddr().(*net.TCPAddr)
		peers = append(peers, peerdiscovery.DiscoveredPeer{
			IP: addr.IP.String(),
		})
		return true
	})
	return peers
}
