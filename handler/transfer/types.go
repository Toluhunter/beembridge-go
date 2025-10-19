package transfer

import (
	"beembridge-go/handler/framingprotocol"
	"beembridge-go/handler/peerdiscovery"
	"time"
)

// --- File Transfer Message Structs ---

type BaseTransferMessage struct {
	Type             string `json:"type"`
	FileID           string `json:"fileId"`
	SenderInstanceID string `json:"senderInstanceId"`
	SenderPeerName   string `json:"senderPeerName"`
	Timestamp        int64  `json:"timestamp"`
}

type FileMetadataMessage struct {
	BaseTransferMessage
	Prefix     string `json:"prefix,omitempty"`
	ParentID   string `json:"parentId,omitempty"`
	TotalItems int    `json:"totalItems,omitempty"`
	FileName   string `json:"fileName"`
	FileSize   int64  `json:"fileSize"`
	TotalChunks int   `json:"totalChunks"`
	ChunkSize  int    `json:"chunkSize"`
}

type FileMetadataAckMessage struct {
	BaseTransferMessage
	Accepted         bool   `json:"accepted"`
	ExistingTransfer bool   `json:"existingTransfer"`
	ContinueIndex    *int   `json:"continueIndex,omitempty"`
	Reason           string `json:"reason,omitempty"`
}

type FileChunkMessage struct {
	BaseTransferMessage
	ChunkIndex      int    `json:"chunkIndex"`
	ActualChunkSize int    `json:"actualChunkSize"`
	Checksum        string `json:"checksum"`
}

type FileChunkAckMessage struct {
	BaseTransferMessage
	ChunkIndex int    `json:"chunkIndex"`
	Success    bool   `json:"success"`
	Reason     string `json:"reason,omitempty"`
}

type QueueFullMessage struct {
	BaseTransferMessage
	Message string `json:"message"`
}

type QueueFreeMessage struct {
	BaseTransferMessage
	Message string `json:"message"`
}

type FileEndMessage struct {
	BaseTransferMessage
	Status        string `json:"status"` // "completed" | "cancelled" | "error"
	FinalChecksum string `json:"finalChecksum,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

type TransferErrorMessage struct {
	BaseTransferMessage
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// --- State and Helper Structs ---

type ChunkDebugInfo struct {
	ChunkActualSize   int    `json:"chunkActualSize"`
	ChunkActualChecksum string `json:"chunkActualChecksum"`
	ChunkFileName     string `json:"chunkFileName"`
}

type IncomingTransferState struct {
	FileID            string
	FileName          string
	FileSize          int64
	TotalChunks       int
	ReceivedBytes     int64
	ReceivedChunkMap  map[int]ChunkDebugInfo
	ChunkStorageDir   string
	MetadataFilePath  string
	TimeoutTimer      *time.Timer
	RemotePeer        peerdiscovery.DiscoveredPeer
	OnProgress        TransferProgressCallback
	OnComplete        TransferCompleteCallback
	OnError           func(fileID string, message string)
	CurrentFrameParser *framingprotocol.FrameParser
	ParentID          string
	Prefix            string
}

type Progress struct {
	FileID           string  `json:"fileId"`
	FileName         string  `json:"fileName"`
	TotalBytes       int64   `json:"totalBytes"`
	TransferredBytes int64   `json:"transferredBytes"`
	Percentage       float64 `json:"percentage"`
	SpeedKbps        float64 `json:"speedKbps,omitempty"`
	ParentID         string  `json:"parentId,omitempty"`
	RootDir          string  `json:"rootDir,omitempty"`
}

type Result struct {
	FileID           string `json:"fileId"`
	FileName         string `json:"fileName"`
	Status           string `json:"status"` // "completed" | "cancelled" | "error"
	Message          string `json:"message,omitempty"`
	ReceivedFilePath string `json:"receivedFilePath,omitempty"`
}

// --- Callbacks ---
type TransferProgressCallback func(progress Progress)
type TransferCompleteCallback func(result Result)
