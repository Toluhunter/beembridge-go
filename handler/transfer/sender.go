package transfer

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"beembridge-go/handler/framingprotocol"
)

const (
	chunkSize            = 1024 * 1024 // 1MB
	maxRetries           = 5
	retryDelay           = 1 * time.Second
	maxOutstandingChunks = 16
)

// ConnectionManager defines the interface needed by a Sender or Receiver
// to interact with the underlying connection.
type ConnectionManager interface {
	Write(data []byte) (int, error)
	RegisterSender(sender *Sender)
	DeregisterSender(fileID string)
	DeregisterReceiver(fileID string)
}

// Sender handles the sending side of a file transfer.
// It manages file metadata exchange, chunking, sending, and error recovery.
type Sender struct {
	connManager       ConnectionManager
	filePath          string
	fileId            string
	senderInstanceId  string
	senderPeerName    string
	onProgress        TransferProgressCallback
	onComplete        TransferCompleteCallback
	onError           func(fileID string, message string)
	onHashingProgress func(progress map[string]interface{})
	parentId          string
	prefix            string
	rootDir           string

	fileSize          int64
	totalChunks       int
	currentChunkIndex int
	outstandingChunks map[int]bool
	paused            bool
	chunkRetryCounts  map[int]int
	mutex             sync.Mutex
}

func NewSender(
	connManager ConnectionManager,
	filePath string,
	senderInstanceId string,
	senderPeerName string,
	onProgress TransferProgressCallback,
	onComplete TransferCompleteCallback,
	onError func(fileID string, message string),
	onHashingProgress func(progress map[string]interface{}),
	parentId string,
	prefix string,
	rootDir string,
) Sender {
	return Sender{
		connManager:       connManager,
		filePath:          filePath,
		senderInstanceId:  senderInstanceId,
		senderPeerName:    senderPeerName,
		onProgress:        onProgress,
		onComplete:        onComplete,
		onError:           onError,
		onHashingProgress: onHashingProgress,
		parentId:          parentId,
		prefix:            prefix,
		rootDir:           rootDir,
		outstandingChunks: make(map[int]bool),
		chunkRetryCounts:  make(map[int]int),
	}
}

func (s *Sender) Start() {
	// Hashing file before transfer
	s.onHashingProgress(map[string]interface{}{
		"filePath":   s.filePath,
		"percentage": 0,
	})
	fileId, err := calculateFileHash(s.filePath, func(percentage int) {
		s.onHashingProgress(map[string]interface{}{
			"filePath":   s.filePath,
			"percentage": percentage,
		})
	})
	if err != nil {
		s.sendError(fmt.Sprintf("Failed to hash file: %v", err), nil)
		s.onHashingProgress(map[string]interface{}{
			"filePath":   s.filePath,
			"percentage": 100, // Clear from UI
		})
		return
	}
	s.fileId = fileId
	s.onHashingProgress(map[string]interface{}{
		"filePath":   s.filePath,
		"percentage": 100, // Clear from UI
	})

	stats, err := os.Stat(s.filePath)
	if err != nil {
		s.sendError(fmt.Sprintf("Failed to get file stats: %v", err), nil)
		return
	}

	s.fileSize = stats.Size()
	s.totalChunks = int((s.fileSize + chunkSize - 1) / chunkSize)

	// Register self with the connection manager.
	// The listener goroutine is no longer started here.
	s.connManager.RegisterSender(s)

	s.sendMetadata()
}

func (s *Sender) sendMetadata() {
	metadata := FileMetadataMessage{
		BaseTransferMessage: BaseTransferMessage{
			Type:             "FILE_METADATA",
			FileID:           s.fileId,
			SenderInstanceID: s.senderInstanceId,
			SenderPeerName:   s.senderPeerName,
			Timestamp:        time.Now().UnixMilli(),
		},
		FileName:    filepath.Base(s.filePath),
		FileSize:    s.fileSize,
		TotalChunks: s.totalChunks,
		ChunkSize:   chunkSize,
		ParentID:    s.parentId,
		Prefix:      s.prefix,
	}

	log.Printf("[Sender] Sending metadata for file %s...", metadata.FileName)
	msgBytes, _ := framingprotocol.BuildFramedMessage(metadata, nil)
	s.connManager.Write(msgBytes)
}

// HandleMessage is called by the central connection manager's reader loop.
func (s *Sender) HandleMessage(header map[string]interface{}) {
	var base BaseTransferMessage
	headerBytes, _ := json.Marshal(header)
	json.Unmarshal(headerBytes, &base)

	// This check is still relevant in case of a logic error in the dispatcher
	if base.FileID != s.fileId {
		log.Printf("[Sender] Received message for unexpected fileId %s. Ignoring.", base.FileID)
		return
	}

	switch base.Type {
	case "FILE_METADATA_ACK":
		var ack FileMetadataAckMessage
		json.Unmarshal(headerBytes, &ack)
		s.handleMetadataAck(ack)
	case "FILE_CHUNK_ACK":
		var ack FileChunkAckMessage
		json.Unmarshal(headerBytes, &ack)
		s.handleChunkAck(ack)
	case "QUEUE_FULL":
		s.mutex.Lock()
		s.paused = true
		s.mutex.Unlock()
		log.Printf("[Sender] Receiver queue full. Pausing transfer.")
	case "QUEUE_FREE":
		s.mutex.Lock()
		s.paused = false
		s.mutex.Unlock()
		log.Printf("[Sender] Receiver queue free. Resuming transfer.")
		s.sendAvailableChunks()
	case "TRANSFER_ERROR":
		log.Printf("[Sender] Received error message from receiver: %v", header["message"])
		s.onError(s.fileId, fmt.Sprintf("Receiver error: %v", header["message"]))
		s.connManager.DeregisterSender(s.fileId)
	}
}

func (s *Sender) handleMetadataAck(ack FileMetadataAckMessage) {
	if ack.Accepted {
		log.Printf("[Sender] Receiver accepted metadata. Starting transfer...")
		if ack.ExistingTransfer && ack.ContinueIndex != nil {
			s.currentChunkIndex = *ack.ContinueIndex + 1
		}
		s.sendAvailableChunks()
	} else {
		s.sendError(fmt.Sprintf("Receiver rejected transfer: %s", ack.Reason), nil)
	}
}

func (s *Sender) handleChunkAck(ack FileChunkAckMessage) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.outstandingChunks[ack.ChunkIndex] {
		return // Ignore duplicate or unexpected ACKs
	}

	if ack.Success {
		delete(s.outstandingChunks, ack.ChunkIndex)
		delete(s.chunkRetryCounts, ack.ChunkIndex)

		if s.currentChunkIndex >= s.totalChunks && len(s.outstandingChunks) == 0 {
			s.finalizeTransfer()
		} else {
			go s.sendAvailableChunks()
		}
	} else {
		log.Printf("[Sender] Receiver requested resend for chunk %d: %s", ack.ChunkIndex, ack.Reason)
		delete(s.outstandingChunks, ack.ChunkIndex)
		s.retryChunk(ack.ChunkIndex)
	}
}

func (s *Sender) sendAvailableChunks() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.paused {
		return
	}

	for len(s.outstandingChunks) < maxOutstandingChunks && s.currentChunkIndex < s.totalChunks {
		chunkIndex := s.currentChunkIndex
		s.outstandingChunks[chunkIndex] = true
		s.currentChunkIndex++

		go s.readAndSendChunk(chunkIndex)
	}
}

func (s *Sender) readAndSendChunk(chunkIndex int) {
	file, err := os.Open(s.filePath)
	if err != nil {
		s.sendError(fmt.Sprintf("Failed to open file: %v", err), nil)
		return
	}
	defer file.Close()

	buffer := make([]byte, chunkSize)
	n, err := file.ReadAt(buffer, int64(chunkIndex)*chunkSize)
	if err != nil && err.Error() != "EOF" {
		s.sendError(fmt.Sprintf("Failed to read chunk %d: %v", chunkIndex, err), nil)
		return
	}

	chunkData := buffer[:n]
	checksum := md5.Sum(chunkData)

	chunkMessage := FileChunkMessage{
		BaseTransferMessage: BaseTransferMessage{
			Type:             "FILE_CHUNK",
			FileID:           s.fileId,
			SenderInstanceID: s.senderInstanceId,
			SenderPeerName:   s.senderPeerName,
			Timestamp:        time.Now().UnixMilli(),
		},
		ChunkIndex:      chunkIndex,
		ActualChunkSize: len(chunkData),
		Checksum:        hex.EncodeToString(checksum[:]),
	}

	msgBytes, _ := framingprotocol.BuildFramedMessage(chunkMessage, chunkData)
	_, err = s.connManager.Write(msgBytes)
	if err != nil {
		// The write error will be handled by the connection manager's reader loop
		// which will terminate and call HandleConnectionError.
		log.Printf("[Sender] Write error: %v. The connection may be closed.", err)
	}

	s.onProgress(Progress{
		FileID:           s.fileId,
		FileName:         filepath.Base(s.filePath),
		TotalBytes:       s.fileSize,
		TransferredBytes: int64(s.currentChunkIndex) * chunkSize,
		Percentage:       float64(s.currentChunkIndex*chunkSize) / float64(s.fileSize) * 100,
		ParentID:         s.parentId,
		RootDir:          s.rootDir,
	})
}

func (s *Sender) retryChunk(chunkIndex int) {
	retries := s.chunkRetryCounts[chunkIndex]
	if retries >= maxRetries {
		s.sendError(fmt.Sprintf("Max retries reached for chunk %d", chunkIndex), nil)
		return
	}
	s.chunkRetryCounts[chunkIndex] = retries + 1

	time.AfterFunc(retryDelay, func() {
		s.mutex.Lock()
		s.outstandingChunks[chunkIndex] = true
		s.mutex.Unlock()
		log.Printf("[Sender] Retrying chunk %d (attempt %d/%d)...", chunkIndex, retries+1, maxRetries)
		s.readAndSendChunk(chunkIndex)
	})
}

func (s *Sender) finalizeTransfer() {
	finalMessage := FileEndMessage{
		BaseTransferMessage: BaseTransferMessage{
			Type:             "FILE_END",
			FileID:           s.fileId,
			SenderInstanceID: s.senderInstanceId,
			SenderPeerName:   s.senderPeerName,
			Timestamp:        time.Now().UnixMilli(),
		},
		Status: "completed",
	}
	msgBytes, _ := framingprotocol.BuildFramedMessage(finalMessage, nil)
	s.connManager.Write(msgBytes)

	s.onComplete(Result{FileID: s.fileId, FileName: filepath.Base(s.filePath), Status: "completed"})

	// Deregister self from the connection manager
	s.connManager.DeregisterSender(s.fileId)
}

func (s *Sender) sendError(msg string, details interface{}) {
	errorMessage := TransferErrorMessage{
		BaseTransferMessage: BaseTransferMessage{
			Type:             "TRANSFER_ERROR",
			FileID:           s.fileId,
			SenderInstanceID: s.senderInstanceId,
			SenderPeerName:   s.senderPeerName,
			Timestamp:        time.Now().UnixMilli(),
		},
		Message: msg,
		Details: details,
	}
	msgBytes, _ := framingprotocol.BuildFramedMessage(errorMessage, nil)
	s.connManager.Write(msgBytes)
	s.onError(s.fileId, msg)
}

func (s *Sender) GetFileID() string {
	return s.fileId
}

func (s *Sender) HandleConnectionError(err error) {
	s.onError(s.fileId, fmt.Sprintf("Connection error: %v", err))
}

func calculateFileHash(filePath string, onProgress func(percentage int)) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	fileInfo, _ := file.Stat()
	fileSize := fileInfo.Size()
	bytesRead := int64(0)

	hash := md5.New()
	buffer := make([]byte, 1024*1024)

	for {
		n, err := file.Read(buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		hash.Write(buffer[:n])
		bytesRead += int64(n)
		if onProgress != nil {
			percentage := int(float64(bytesRead) / float64(fileSize) * 100)
			onProgress(percentage)
		}
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}