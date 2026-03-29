package rpc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Encoder writes newline-delimited JSON messages to a writer.
type Encoder struct {
	w  io.Writer
	mu sync.Mutex
}

// NewEncoder creates an Encoder that writes to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

// Send marshals msg as JSON and writes it followed by a newline.
func (e *Encoder) Send(msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling message: %w", err)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	data = append(data, '\n')
	if _, err := e.w.Write(data); err != nil {
		return fmt.Errorf("writing message: %w", err)
	}

	return nil
}

// Decoder reads newline-delimited JSON messages from a reader.
type Decoder struct {
	scanner *bufio.Scanner
}

// NewDecoder creates a Decoder that reads from r.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{scanner: bufio.NewScanner(r)}
}

// Receive reads one line and unmarshals the JSON into msg.
func (d *Decoder) Receive(msg interface{}) error {
	if !d.scanner.Scan() {
		if err := d.scanner.Err(); err != nil {
			return fmt.Errorf("reading message: %w", err)
		}
		return io.EOF
	}

	line := d.scanner.Bytes()
	if err := json.Unmarshal(line, msg); err != nil {
		return fmt.Errorf("unmarshaling message: %w", err)
	}

	return nil
}

// Envelope is used to peek at the Type field before full deserialization.
type Envelope struct {
	Type MessageType `json:"type"`
}

// ReceiveEnvelope reads one line, peeks at the type, and returns raw JSON bytes
// along with the message type. Use json.Unmarshal on the returned bytes to
// deserialize into the correct concrete type.
func (d *Decoder) ReceiveEnvelope() (MessageType, []byte, error) {
	if !d.scanner.Scan() {
		if err := d.scanner.Err(); err != nil {
			return "", nil, fmt.Errorf("reading message: %w", err)
		}
		return "", nil, io.EOF
	}

	line := d.scanner.Bytes()

	// Copy because scanner reuses the buffer
	raw := make([]byte, len(line))
	copy(raw, line)

	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", nil, fmt.Errorf("peeking message type: %w", err)
	}

	return env.Type, raw, nil
}
