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
type FrameParser struct {
	buffer                []byte
	expectedHeaderLength  uint32
	expectedPayloadLength uint32
	state                 string
	currentHeader         map[string]interface{}
}

// NewFrameParser creates a new FrameParser instance.
func NewFrameParser() *FrameParser {
	return &FrameParser{
		buffer: []byte{},
		state:  "WAITING_HEADER_LENGTH",
	}
}

// Feed consumes new incoming data and returns any fully parsed messages.
func (fp *FrameParser) Feed(chunk []byte) ([]FramedMessage, error) {
	fp.buffer = append(fp.buffer, chunk...)
	messages := []FramedMessage{}

	for {
		switch fp.state {
		case "WAITING_HEADER_LENGTH":
			if len(fp.buffer) < 4 {
				return messages, nil // not enough data yet
			}
			fp.expectedHeaderLength = binary.LittleEndian.Uint32(fp.buffer[:4])
			fp.buffer = fp.buffer[4:]
			fp.state = "WAITING_HEADER"

		case "WAITING_HEADER":
			if uint32(len(fp.buffer)) < fp.expectedHeaderLength {
				return messages, nil
			}
			headerBytes := fp.buffer[:fp.expectedHeaderLength]
			fp.buffer = fp.buffer[fp.expectedHeaderLength:]

			if err := json.Unmarshal(headerBytes, &fp.currentHeader); err != nil {
				fp.Reset()
				return nil, &FramingError{Message: "Failed to parse JSON header"}
			}

			fp.state = "WAITING_PAYLOAD_LENGTH"

		case "WAITING_PAYLOAD_LENGTH":
			if len(fp.buffer) < 4 {
				return messages, nil
			}
			fp.expectedPayloadLength = binary.LittleEndian.Uint32(fp.buffer[:4])
			fp.buffer = fp.buffer[4:]
			fp.state = "WAITING_PAYLOAD"

		case "WAITING_PAYLOAD":
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
			fp.state = "WAITING_HEADER_LENGTH"

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
	fp.state = "WAITING_HEADER_LENGTH"
	fp.currentHeader = nil
}
