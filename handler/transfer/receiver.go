package transfer

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"beembridge-go/handler/framingprotocol"
	"beembridge-go/handler/peerdiscovery"
)

const (
	maxQueueMemoryPerFile = 256 * 1024 * 1024 // 256 MB
	transferTimeout       = 30 * time.Second
)

type WriteJob struct {
	ChunkIndex    int
	ChunkBuffer   []byte
	ChunkFileName string
	State         *IncomingTransferState
}

// Receiver handles the receiving side of a file transfer.
// It is now driven by a ConnectionManager instead of having its own read loop.
type Receiver struct {
	connManager       ConnectionManager
	fileId            string
	downloadDir       string
	remotePeer        peerdiscovery.DiscoveredPeer
	myInstanceID      string
	myPeerName        string
	onProgress        TransferProgressCallback
	onComplete        TransferCompleteCallback
	onError           func(fileID string, message string)
	onHashingProgress func(progress map[string]interface{})
	requestAcceptance func(fileID, fileName string, fileSize int64, senderPeerName string, acceptCallback func(string))

	activeTransfers sync.Map // map[string]*IncomingTransferState
	fileWriteQueues sync.Map // map[string]chan WriteJob
}

func NewReceiver(
	connManager ConnectionManager,
	fileID string,
	downloadDir string,
	remotePeer peerdiscovery.DiscoveredPeer,
	myInstanceID string,
	myPeerName string,
	onProgress TransferProgressCallback,
	onComplete TransferCompleteCallback,
	onError func(fileID string, message string),
	onHashingProgress func(progress map[string]interface{}),
	requestAcceptance func(fileID, fileName string, fileSize int64, senderPeerName string, acceptCallback func(string)),
) Receiver {
	return Receiver{
		connManager:       connManager,
		fileId:            fileID,
		downloadDir:       downloadDir,
		remotePeer:        remotePeer,
		myInstanceID:      myInstanceID,
		myPeerName:        myPeerName,
		onProgress:        onProgress,
		onComplete:        onComplete,
		onError:           onError,
		onHashingProgress: onHashingProgress,
		requestAcceptance: requestAcceptance,
	}
}

// HandleMessage is the main entry point for the Receiver, called by the ConnectionManager dispatcher.
func (r *Receiver) HandleMessage(header map[string]interface{}, payload []byte) {
	var base BaseTransferMessage
	headerBytes, _ := json.Marshal(header)
	json.Unmarshal(headerBytes, &base)
	log.Printf("[Receiver] Received Message From Sender: %s", base.Type)

	switch base.Type {
	case "FILE_METADATA":
		var metadata FileMetadataMessage
		json.Unmarshal(headerBytes, &metadata)
		r.handleMetadata(metadata)
	case "FILE_CHUNK":
		var chunkMsg FileChunkMessage
		json.Unmarshal(headerBytes, &chunkMsg)
		r.handleChunk(chunkMsg, payload)
	case "FILE_END":
		var endMsg FileEndMessage
		json.Unmarshal(headerBytes, &endMsg)
		r.handleFileEnd(endMsg)
	case "TRANSFER_ERROR":
		var errMsg TransferErrorMessage
		json.Unmarshal(headerBytes, &errMsg)
		r.handleTransferError(errMsg)
	}
}

func (r *Receiver) sendMetadataAck(fileID string, accepted bool, existingTransfer bool, reason string, continueIndex *int) {
	ack := FileMetadataAckMessage{
		BaseTransferMessage: BaseTransferMessage{
			Type:             "FILE_METADATA_ACK",
			FileID:           fileID,
			SenderInstanceID: r.myInstanceID,
			SenderPeerName:   r.myPeerName,
			Timestamp:        time.Now().UnixMilli(),
		},
		Accepted:         accepted,
		ExistingTransfer: existingTransfer,
		ContinueIndex:    continueIndex,
		Reason:           reason,
	}
	msgBytes, _ := framingprotocol.BuildFramedMessage(ack, nil)
	log.Printf("[Receiver] Sending metadata ACK for fileID %s: accepted=%v, existingTransfer=%v", fileID, accepted, existingTransfer)
	r.connManager.Write(msgBytes)
}

func (r *Receiver) handleMetadata(metadata FileMetadataMessage) {
	if _, ok := r.activeTransfers.Load(metadata.FileID); ok {
		r.sendMetadataAck(metadata.FileID, false, false, "Duplicate transfer request", nil)
		return
	}

	log.Printf("[Receiver] Received metadata for file %s (%d bytes) from %s.", metadata.FileName, metadata.FileSize, metadata.SenderPeerName)

	r.requestAcceptance(metadata.FileID, metadata.FileName, metadata.FileSize, metadata.SenderPeerName, func(acceptedFileID string) {
		chunkStorageDir := filepath.Join(r.downloadDir, acceptedFileID)
		metadataFilePath := filepath.Join(chunkStorageDir, "metadata.json")
		os.MkdirAll(chunkStorageDir, 0755)

		initialState := &IncomingTransferState{
			FileID:           acceptedFileID,
			FileName:         metadata.FileName,
			FileSize:         metadata.FileSize,
			TotalChunks:      metadata.TotalChunks,
			ReceivedBytes:    0,
			ReceivedChunkMap: make(map[int]ChunkDebugInfo),
			ChunkStorageDir:  chunkStorageDir,
			MetadataFilePath: metadataFilePath,
			RemotePeer:       r.remotePeer,
			OnProgress:       r.onProgress,
			OnComplete:       r.onComplete,
			OnError:          r.onError,
			ParentID:         metadata.ParentID,
			Prefix:           metadata.Prefix,
		}
		r.activeTransfers.Store(acceptedFileID, initialState)

		// TODO: Implement transfer resumption logic by checking metadata file

		r.sendMetadataAck(acceptedFileID, true, false, "", nil)

		initialState.TimeoutTimer = time.AfterFunc(transferTimeout, func() {
			log.Printf("[Receiver] Transfer %s timed out.", initialState.FileName)
			initialState.OnComplete(Result{FileID: acceptedFileID, FileName: initialState.FileName, Status: "error", Message: "Transfer timeout"})
			r.cleanupTransfer(initialState)
		})
	})
}

func (r *Receiver) handleChunk(chunkMsg FileChunkMessage, payload []byte) {
	val, ok := r.activeTransfers.Load(chunkMsg.FileID)
	if !ok {
		log.Printf("[Receiver] Received chunk for unknown/unaccepted file ID %s. Ignoring.", chunkMsg.FileID)
		return
	}
	state := val.(*IncomingTransferState)

	// Reset timeout
	state.TimeoutTimer.Reset(transferTimeout)

	// Validate chunk
	if len(payload) != chunkMsg.ActualChunkSize {
		r.sendChunkAck(chunkMsg.FileID, chunkMsg.ChunkIndex, false, "Mismatched size")
		return
	}
	checksum := md5.Sum(payload)
	if hex.EncodeToString(checksum[:]) != chunkMsg.Checksum {
		r.sendChunkAck(chunkMsg.FileID, chunkMsg.ChunkIndex, false, "Checksum mismatch")
		return
	}

	// TODO: Implement backpressure logic

	// Add to write queue
	job := WriteJob{
		ChunkIndex:    chunkMsg.ChunkIndex,
		ChunkBuffer:   payload,
		ChunkFileName: fmt.Sprintf("chunk_%d.part", chunkMsg.ChunkIndex),
		State:         state,
	}

	queue, _ := r.fileWriteQueues.LoadOrStore(state.FileID, make(chan WriteJob, 100))
	(queue.(chan WriteJob)) <- job

	go r.processWriteQueue(state.FileID)
}

func (r *Receiver) processWriteQueue(fileID string) {
	val, _ := r.fileWriteQueues.Load(fileID)
	queue := val.(chan WriteJob)

	for job := range queue {
		chunkFilePath := filepath.Join(job.State.ChunkStorageDir, job.ChunkFileName)
		err := os.WriteFile(chunkFilePath, job.ChunkBuffer, 0644)
		if err != nil {
			log.Printf("[Receiver] Error writing chunk %d to disk: %v", job.ChunkIndex, err)
			r.sendChunkAck(job.State.FileID, job.ChunkIndex, false, "Failed to write chunk to disk")
			continue
		}

		r.sendChunkAck(job.State.FileID, job.ChunkIndex, true, "")

		checksum := md5.Sum(job.ChunkBuffer)
		debugInfo := ChunkDebugInfo{
			ChunkActualSize:     len(job.ChunkBuffer),
			ChunkActualChecksum: hex.EncodeToString(checksum[:]),
			ChunkFileName:       job.ChunkFileName,
		}
		if _, exists := job.State.ReceivedChunkMap[job.ChunkIndex]; !exists {
			job.State.ReceivedBytes += int64(len(job.ChunkBuffer))
		}
		job.State.ReceivedChunkMap[job.ChunkIndex] = debugInfo

		// Persist metadata
		metaBytes, _ := json.MarshalIndent(job.State.ReceivedChunkMap, "", "  ")
		os.WriteFile(job.State.MetadataFilePath, metaBytes, 0644)

		job.State.OnProgress(Progress{
			FileID:           job.State.FileID,
			FileName:         job.State.FileName,
			TotalBytes:       job.State.FileSize,
			TransferredBytes: job.State.ReceivedBytes,
			Percentage:       float64(job.State.ReceivedBytes) / float64(job.State.FileSize) * 100,
		})
	}
}

func (r *Receiver) sendChunkAck(fileID string, chunkIndex int, success bool, reason string) {
	ack := FileChunkAckMessage{
		BaseTransferMessage: BaseTransferMessage{
			Type:             "FILE_CHUNK_ACK",
			FileID:           fileID,
			SenderInstanceID: r.myInstanceID,
			SenderPeerName:   r.myPeerName,
			Timestamp:        time.Now().UnixMilli(),
		},
		ChunkIndex: chunkIndex,
		Success:    success,
		Reason:     reason,
	}
	msgBytes, _ := framingprotocol.BuildFramedMessage(ack, nil)
	r.connManager.Write(msgBytes)
}

func (r *Receiver) handleFileEnd(endMsg FileEndMessage) {
	val, ok := r.activeTransfers.Load(endMsg.FileID)
	if !ok {
		return
	}
	state := val.(*IncomingTransferState)

	log.Printf("[Receiver] File %s transfer ended by sender. Status: %s", state.FileName, endMsg.Status)

	if state.TimeoutTimer != nil {
		state.TimeoutTimer.Stop()
	}

	if endMsg.Status == "completed" {
		r.reconstructFile(state)
	} else {
		state.OnComplete(Result{
			FileID:   endMsg.FileID,
			FileName: state.FileName,
			Status:   endMsg.Status,
			Message:  endMsg.Reason,
		})
		r.cleanupTransfer(state)
	}
}

func (r *Receiver) reconstructFile(state *IncomingTransferState) {
	log.Printf("[Receiver] Reconstructing %s from received chunks...", state.FileName)

	// TODO: Wait for write queue to drain

	// TODO: Request missing chunks

	// Assemble file
	targetPath := filepath.Join(r.downloadDir, state.Prefix)
	os.MkdirAll(targetPath, 0755)
	outputFilePath := filepath.Join(targetPath, state.FileName)

	outFile, err := os.OpenFile(outputFilePath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		state.OnError(state.FileID, fmt.Sprintf("Failed to create final file: %v", err))
		return
	}
	defer outFile.Close()

	for i := 0; i < state.TotalChunks; i++ {
		chunkInfo, ok := state.ReceivedChunkMap[i]
		if !ok {
			// This should be handled by missing chunk request logic
			state.OnError(state.FileID, fmt.Sprintf("Missing chunk %d during reconstruction", i))
			return
		}
		chunkFilePath := filepath.Join(state.ChunkStorageDir, chunkInfo.ChunkFileName)
		chunkBytes, err := os.ReadFile(chunkFilePath)
		if err != nil {
			state.OnError(state.FileID, fmt.Sprintf("Failed to read chunk %d: %v", i, err))
			return
		}
		outFile.Write(chunkBytes)
	}

	// TODO: Verify final checksum

	log.Printf("[Receiver] File %s reconstructed successfully to %s.", state.FileName, outputFilePath)
	state.OnComplete(Result{
		FileID:           state.FileID,
		FileName:         state.FileName,
		Status:           "completed",
		ReceivedFilePath: outputFilePath,
	})

	r.cleanupTransfer(state)
}

func (r *Receiver) handleTransferError(errMsg TransferErrorMessage) {
	val, ok := r.activeTransfers.Load(errMsg.FileID)
	if !ok {
		return
	}
	state := val.(*IncomingTransferState)

	state.OnComplete(Result{
		FileID:   errMsg.FileID,
		FileName: state.FileName,
		Status:   "error",
		Message:  fmt.Sprintf("Sender reported error: %s", errMsg.Message),
	})

	r.cleanupTransfer(state)
}

func (r *Receiver) cleanupTransfer(state *IncomingTransferState) {
	if state.TimeoutTimer != nil {
		state.TimeoutTimer.Stop()
	}
	r.activeTransfers.Delete(state.FileID)
	os.RemoveAll(state.ChunkStorageDir)

	// Deregister from the manager
	r.connManager.DeregisterReceiver(state.FileID)
}

func (r *Receiver) HandleConnectionError(err error) {
	// When connection dies, notify all active incoming transfers
	r.activeTransfers.Range(func(key, value interface{}) bool {
		state := value.(*IncomingTransferState)
		state.OnError(state.FileID, fmt.Sprintf("Connection error: %v", err))
		r.cleanupTransfer(state)
		return true
	})
}
