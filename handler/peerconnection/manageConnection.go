package peerconnection

import (
	"beembridge-go/handler/framingprotocol"
	"beembridge-go/handler/peerdiscovery"
	"beembridge-go/handler/transfer"
	"encoding/json"
	"errors"
	"log"
	"net"
	"sync"
)

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

func (mc *ManagedConnection) disconnect() {
	mc.conn.Close()
	mc.cleanup()
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
