package transfer

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
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

// Sender handles the sending side of a file transfer.
// It manages file metadata exchange, chunking, sending, and error recovery.
type Sender struct {
	conn             net.Conn
	filePath         string
	fileId           string
	senderInstanceId string
	senderPeerName   string
	onProgress       TransferProgressCallback
	onComplete       TransferCompleteCallback
	onError          func(fileID string, message string)
	parentId         string
	prefix           string
	rootDir          string

	fileSize          int64
	totalChunks       int
	currentChunkIndex int
	outstandingChunks map[int]bool
	paused            bool
	chunkRetryCounts  map[int]int
	mutex             sync.Mutex
}

func NewSender(
	conn net.Conn,
	filePath string,
	fileId string,
	senderInstanceId string,
	senderPeerName string,
	onProgress TransferProgressCallback,
	onComplete TransferCompleteCallback,
	onError func(fileID string, message string),
	parentId string,
	prefix string,
	rootDir string,
) *Sender {
	return &Sender{
		conn:              conn,
		filePath:          filePath,
		fileId:            fileId,
		senderInstanceId:  senderInstanceId,
		senderPeerName:    senderPeerName,
		onProgress:        onProgress,
		onComplete:        onComplete,
		onError:           onError,
		parentId:          parentId,
		prefix:            prefix,
		rootDir:           rootDir,
		outstandingChunks: make(map[int]bool),
		chunkRetryCounts:  make(map[int]int),
	}
}

func (s *Sender) Start() {
	stats, err := os.Stat(s.filePath)
	if err != nil {
		s.sendError(fmt.Sprintf("Failed to get file stats: %v", err), nil)
		return
	}

	s.fileSize = stats.Size()
	s.totalChunks = int((s.fileSize + chunkSize - 1) / chunkSize)

	go s.listenForAcks()

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
	s.conn.Write(msgBytes)
}

func (s *Sender) listenForAcks() {
	frameParser := framingprotocol.NewFrameParser()
	buffer := make([]byte, 2048)

	for {
		n, err := s.conn.Read(buffer)
		if err != nil {
			s.sendError(fmt.Sprintf("Connection error: %v", err), nil)
			return
		}

		messages, err := frameParser.Feed(buffer[:n])
		if err != nil {
			log.Printf("[Sender] Error parsing frame: %v", err)
			frameParser.Reset()
			continue
		}

		for _, msg := range messages {
			s.handleMessage(msg.Header)
		}
	}
}

func (s *Sender) handleMessage(header map[string]interface{}) {
	var base BaseTransferMessage
	headerBytes, _ := json.Marshal(header)
	json.Unmarshal(headerBytes, &base)

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
	_, err = s.conn.Write(msgBytes)
	if err != nil {
		s.retryChunk(chunkIndex)
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
	s.conn.Write(msgBytes)

	s.onComplete(Result{FileID: s.fileId, FileName: filepath.Base(s.filePath), Status: "completed"})
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
	s.conn.Write(msgBytes)
	s.onError(s.fileId, msg)
}
