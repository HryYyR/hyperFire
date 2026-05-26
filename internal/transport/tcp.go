package transport

import (
	"encoding/binary"
	"fmt"
	"io"

	"agentDemo/internal/netproto"

	"google.golang.org/protobuf/proto"
)

const MaxTCPMessageSize = 1 << 20

func ReadTCPEnvelope(r io.Reader) (*netproto.TcpEnvelope, error) {
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, err
	}
	if length == 0 || length > MaxTCPMessageSize {
		return nil, fmt.Errorf("invalid tcp message size: %d", length)
	}

	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}

	var envelope netproto.TcpEnvelope
	if err := proto.Unmarshal(buf, &envelope); err != nil {
		return nil, err
	}
	return &envelope, nil
}

func WriteTCPEnvelope(w io.Writer, envelope *netproto.TcpEnvelope) error {
	payload, err := proto.Marshal(envelope)
	if err != nil {
		return err
	}
	if len(payload) == 0 || len(payload) > MaxTCPMessageSize {
		return fmt.Errorf("invalid tcp message size: %d", len(payload))
	}

	if err := binary.Write(w, binary.BigEndian, uint32(len(payload))); err != nil {
		return err
	}
	_, err = w.Write(payload)
	return err
}
