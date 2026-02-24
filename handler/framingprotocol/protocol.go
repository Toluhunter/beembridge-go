package framingprotocol

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
)

// FramedMessage represents a parsed message with a JSON header and optional binary payload.
type FramedMessage struct {
	Header  map[string]interface{}
	Payload []byte
}

// FramingError represents custom errors for framing issues.
type FramingError struct {
	Message string
}

func (e *FramingError) Error() string {
	return fmt.Sprintf("FramingError: %s", e.Message)
}

const (
	MaxHeaderSize  = 1 << 20 // e.g., 1 MiB
	MaxPayloadSize = 1 << 30 // e.g., 1 GiB — tune this for your app
)

// BuildFramedMessage constructs a framed message as bytes.
// Format: [4-byte header length][JSON header][4-byte payload length][binary payload]
func BuildFramedMessage(header interface{}, payload []byte) ([]byte, error) {
	headerBytes, err := json.Marshal(header)
	if err != nil {
		return nil, err
	}

	headerLen := uint32(len(headerBytes))
	payloadLen := uint32(0)
	if payload != nil {
		payloadLen = uint32(len(payload))
	}

	totalLen := 4 + headerLen + 4 + payloadLen
	buf := bytes.NewBuffer(make([]byte, 0, totalLen))

	// Write header length
	if err := binary.Write(buf, binary.LittleEndian, headerLen); err != nil {
		return nil, err
	}

	// Write JSON header
	if _, err := buf.Write(headerBytes); err != nil {
		return nil, err
	}

	// Write payload length
	if err := binary.Write(buf, binary.LittleEndian, payloadLen); err != nil {
		return nil, err
	}

	// Write payload (if any)
	if payload != nil {
		if _, err := buf.Write(payload); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

// FrameParser incrementally parses incoming framed messages from a TCP stream.
type parserState uint8

const (
	waitingHeaderLength parserState = iota
	waitingHeader
	waitingPayloadLength
	waitingPayload
)

type FrameParser struct {
	buffer                []byte
	expectedHeaderLength  uint32
	expectedPayloadLength uint32
	state                 parserState
	currentHeader         map[string]interface{}
}

// NewFrameParser creates a new FrameParser instance.
func NewFrameParser() *FrameParser {
	return &FrameParser{
		buffer: []byte{},
		state:  waitingHeaderLength,
	}
}

// Feed consumes new incoming data and returns any fully parsed messages.
func (fp *FrameParser) Feed(chunk []byte) ([]FramedMessage, error) {
	fp.buffer = append(fp.buffer, chunk...)
	messages := []FramedMessage{}

	for {
		switch fp.state {
		case waitingHeaderLength:
			if len(fp.buffer) < 4 {
				return messages, nil // not enough data yet
			}
			fp.expectedHeaderLength = binary.LittleEndian.Uint32(fp.buffer[:4])
			fp.buffer = fp.buffer[4:]
			fp.state = waitingHeader

		case waitingHeader:
			if uint32(len(fp.buffer)) < fp.expectedHeaderLength {
				return messages, nil
			}
			headerBytes := fp.buffer[:fp.expectedHeaderLength]
			fp.buffer = fp.buffer[fp.expectedHeaderLength:]

			if err := json.Unmarshal(headerBytes, &fp.currentHeader); err != nil {
				fp.ResetOnError()
				return nil, &FramingError{Message: "Failed to parse JSON header"}
			}

			fp.state = waitingPayloadLength

		case waitingPayloadLength:
			if len(fp.buffer) < 4 {
				return messages, nil
			}
			fp.expectedPayloadLength = binary.LittleEndian.Uint32(fp.buffer[:4])
			fp.buffer = fp.buffer[4:]
			fp.state = waitingPayload

		case waitingPayload:
			if uint32(len(fp.buffer)) < fp.expectedPayloadLength {
				return messages, nil
			}

			var payload []byte
			if fp.expectedPayloadLength > 0 {
				payload = fp.buffer[:fp.expectedPayloadLength]
				fp.buffer = fp.buffer[fp.expectedPayloadLength:]
			} else {
				payload = nil
			}

			msg := FramedMessage{
				Header:  fp.currentHeader,
				Payload: payload,
			}
			messages = append(messages, msg)

			// Reset for next message
			fp.expectedHeaderLength = 0
			fp.expectedPayloadLength = 0
			fp.currentHeader = nil
			fp.state = waitingHeaderLength

			if len(fp.buffer) == 0 {
				return messages, nil
			}
		default:
			return messages, &FramingError{Message: "Invalid parser state"}
		}
	}
}

// Reset clears the parser’s internal state.
func (fp *FrameParser) Reset() {
	fp.buffer = []byte{}
	fp.expectedHeaderLength = 0
	fp.expectedPayloadLength = 0
	fp.state = waitingHeaderLength
	fp.currentHeader = nil
}

// ResetOnError tries to resynchronize the parser buffer when a header is invalid.
// It searches for the next plausible header-length field and a valid JSON header right after it.
// If nothing looks plausible it does a hard Reset.
func (fp *FrameParser) ResetOnError() {
	buf := fp.buffer
	n := len(buf)

	// minimal bytes to hold a headerLen + at least 0 header bytes
	if n < 4 {
		// not enough to find anything useful — just clear
		fp.Reset()
		return
	}

	// scan for a possible header-length field
	for i := 0; i+4 <= n; i++ {
		candidateLen := binary.LittleEndian.Uint32(buf[i : i+4])

		// sanity checks on candidate header length
		if candidateLen == 0 || candidateLen > MaxHeaderSize {
			continue
		}

		// ensure we actually have header bytes available to validate
		headerStart := i + 4
		headerEnd := headerStart + int(candidateLen)
		if headerEnd > n {
			// not enough bytes yet — we should keep the tail bytes for next read
			// keep everything from i (potential start) onwards and reset state ready to parse header-length
			fp.buffer = buf[i:]
			fp.state = waitingHeaderLength
			fp.expectedHeaderLength = 0
			fp.expectedPayloadLength = 0
			fp.currentHeader = nil
			return
		}

		// attempt to unmarshal the candidate header
		var tmp map[string]interface{}
		if err := json.Unmarshal(buf[headerStart:headerEnd], &tmp); err == nil {
			// Found a plausible start — keep everything from this frame's header-length onward
			fp.buffer = buf[i:]
			fp.state = waitingHeaderLength
			fp.expectedHeaderLength = 0
			fp.expectedPayloadLength = 0
			fp.currentHeader = nil
			return
		}
		// else keep scanning
	}

	// If we get here, no plausible boundary found — hard reset (safe fallback)
	fp.Reset()
}
