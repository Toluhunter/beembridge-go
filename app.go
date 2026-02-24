package main

import (
	"beembridge-go/handler/peerconnection"
	"beembridge-go/handler/peerdiscovery"
	"beembridge-go/handler/transfer"
	"context"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx                context.Context
	peerDiscovery      *peerdiscovery.PeerDiscovery
	peerConnection     *peerconnection.PeerConnection
	connectionRequests map[string]func(bool, string)
	mu                 sync.Mutex
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	peerName, err := os.Hostname()
	if err != nil {
		peerName = "unknown-peer"
	}
	pd, err := peerdiscovery.NewPeerDiscovery("beembridge-go-v1", peerName)
	if err != nil {
		log.Fatalf("Failed to create peer discovery: %v", err)
	}
	a.peerDiscovery = pd

	// --- Peer Connection Logic ---
	tcpPortStr := os.Getenv("TCP_PORT")
	if tcpPortStr == "" {
		log.Fatalf("TCP_PORT environment variable not set")
	}
	tcpPort, err := strconv.Atoi(tcpPortStr)
	if err != nil {
		log.Fatalf("Failed to parse TCP_PORT: %v", err)
	}

	// --- Transfer Callbacks ---
	transferCallbacks := &transfer.TransferCallbacks{
		OnProgress: func(progress transfer.Progress) {
			runtime.EventsEmit(a.ctx, "onProgressUpdate", progress)
		},
		OnComplete: func(result transfer.Result) {
			runtime.EventsEmit(a.ctx, "onTransferComplete", result)
		},
		OnError: func(fileID string, message string) {
			runtime.EventsEmit(a.ctx, "onTransferError", fileID, message)
		},
		OnHashingProgress: func(progress map[string]interface{}) {
			runtime.EventsEmit(a.ctx, "onHashingProgress", progress)
		},
		RequestAcceptance: func(fileID, fileName string, fileSize int64, senderPeerName string, acceptCallback func(string)) {
			log.Printf("Auto-accepting file transfer request for %s from %s", fileName, senderPeerName)
			acceptCallback(fileID)
		},
	}

	// Get download dir
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Could not get user home directory: %v", err)
	}
	downloadDir := filepath.Join(homeDir, "Downloads", "Beembridge")
	os.MkdirAll(downloadDir, 0755)

	pc := peerconnection.NewPeerConnection(pd.GetInstanceID(), peerName, tcpPort, downloadDir, transferCallbacks)
	a.peerConnection = pc
	a.connectionRequests = make(map[string]func(bool, string))

	pc.OnConnectionRequest(func(peer peerdiscovery.DiscoveredPeer, respond func(accept bool, reason string)) {
		a.mu.Lock()
		defer a.mu.Unlock()
		requestID := peer.InstanceID
		a.connectionRequests[requestID] = respond
		runtime.EventsEmit(a.ctx, "onPeerConnectionRequest", peer, requestID)
	})

	pc.OnPeerConnected(func(peer peerdiscovery.DiscoveredPeer) {
		runtime.EventsEmit(a.ctx, "onPeerConnected", peer)
	})

	pc.OnConnectionStatus(func(peer peerdiscovery.DiscoveredPeer, status string) {
		runtime.EventsEmit(a.ctx, "onConnectionResponse", peer, status, "")
	})

	if err := pc.StartTCPServer(); err != nil {
		log.Fatalf("Failed to start TCP server: %v", err)
	}

	// Register drag-and-drop handler in Go. When files are dropped onto the
	// window this will resolve their stats and emit an `onFilesDropped` event
	// to the frontend with the list of SelectedItem objects.
	runtime.OnFileDrop(a.ctx, func(x, y int, paths []string) {
		log.Printf("Files dropped at %d,%d: %v", x, y, paths)
		files, err := a.GetFileStats(paths)
		if err != nil {
			log.Printf("Error getting file stats for dropped files: %v", err)
			return
		}
		runtime.EventsEmit(a.ctx, "onFilesDropped", files)
	})
}

// StartPeerDiscovery starts the peer discovery service.
func (a *App) StartPeerDiscovery() {
	if err := a.peerDiscovery.Start(); err != nil {
		log.Printf("Failed to start peer discovery: %v", err)
	}
}

// StopPeerDiscovery stops the peer discovery service.
func (a *App) StopPeerDiscovery() {
	a.peerDiscovery.Stop()
}

func (a *App) DisconnectFromPeer(peerID string) {
	ok := a.peerConnection.DisconnectPeer(peerID)
	if !ok {
		log.Printf("No active connection found for peer ID/Unable to Disconnect: %s", peerID)
	} else {
		log.Printf("Disconnected from peer: %s", peerID)
	}
}

// GetDiscoveredPeers returns the list of discovered peers.
func (a *App) GetDiscoveredPeers() []peerdiscovery.DiscoveredPeer {
	return a.peerDiscovery.GetPeers()
}

// ConnectToPeer attempts to connect to a given peer.
func (a *App) ConnectToPeer(peer peerdiscovery.DiscoveredPeer) {
	log.Printf("Attempting to connect to peer: %s", peer.PeerName)
	go a.peerConnection.ConnectToPeer(peer)
}

// RespondToPeerConnectionRequest responds to a pending connection request.
func (a *App) RespondToPeerConnectionRequest(requestID string, accept bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if respond, ok := a.connectionRequests[requestID]; ok {
		log.Printf("Responding to connection request %s with accept: %v", requestID, accept)
		respond(accept, "") // Reason is empty for now
		delete(a.connectionRequests, requestID)
	} else {
		log.Printf("No pending connection request found for ID: %s", requestID)
	}
}

// GetConnectedPeers returns the list of currently connected peers.
func (a *App) GetConnectedPeers() []peerdiscovery.DiscoveredPeer {
	return a.peerConnection.GetConnectedPeers()
}

// --- File Transfer Methods ---

type SelectedItem struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	IsDirectory bool   `json:"isDirectory"`
}

func (a *App) OpenFileDialog() ([]string, error) {
	return runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Files",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "All Files (*.*)",
				Pattern:     "*.*",
			},
		},
	})
}

func (a *App) OpenDirectoryDialog() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Directory",
	})
}

func (a *App) GetFileStats(paths []string) ([]SelectedItem, error) {
	var items []SelectedItem
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			log.Printf("Error getting stats for path %s: %v", path, err)
			continue // Or return error?
		}
		items = append(items, SelectedItem{
			Path:        path,
			Name:        info.Name(),
			Size:        info.Size(),
			IsDirectory: info.IsDir(),
		})
	}
	return items, nil
}

func (a *App) InitiateFileTransfer(peerID string, items []SelectedItem) {
	for _, item := range items {
		if !item.IsDirectory {
			err := a.peerConnection.InitiateFileTransfer(peerID, item.Path, "", "", "")
			if err != nil {
				log.Printf("Error initiating transfer for %s: %v", item.Name, err)
			}
		} else {
			log.Printf("Directory transfer not implemented yet for: %s", item.Name)
		}
	}
}
