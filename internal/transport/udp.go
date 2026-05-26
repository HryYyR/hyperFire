package transport

import (
	"fmt"

	"agentDemo/internal/netproto"

	"google.golang.org/protobuf/proto"
)

const MaxUDPPacketSize = 130000

func MarshalUDPEnvelope(envelope *netproto.UdpEnvelope) ([]byte, error) {
	payload, err := proto.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	if len(payload) > MaxUDPPacketSize {
		return nil, fmt.Errorf("udp packet too large: %d", len(payload))
	}
	return payload, nil
}

func UnmarshalUDPEnvelope(payload []byte) (*netproto.UdpEnvelope, error) {
	var envelope netproto.UdpEnvelope
	if err := proto.Unmarshal(payload, &envelope); err != nil {
		return nil, err
	}
	return &envelope, nil
}
