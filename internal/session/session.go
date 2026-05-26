package session

import (
	"net"
	"sync"
	"sync/atomic"

	"agentDemo/internal/netproto"
)

type InputState struct {
	Seq   uint32
	MoveX int32
	MoveY int32
	Fire  bool
	AimDX float32
	AimDY float32
}

func InputStateFromProto(frame *netproto.InputFrame) InputState {
	return InputState{
		Seq:   frame.GetInputSeq(),
		MoveX: frame.GetMoveX(),
		MoveY: frame.GetMoveY(),
		Fire:  frame.GetFire(),
		AimDX: frame.GetAimDx(),
		AimDY: frame.GetAimDy(),
	}
}

type Session struct {
	PlayerID  uint32
	SessionID uint64
	Name      string
	TCPConn   net.Conn

	udpAddr atomic.Pointer[net.UDPAddr]

	inputMu   sync.RWMutex
	lastInput InputState
}

func New(playerID uint32, sessionID uint64, name string, tcpConn net.Conn) *Session {
	return &Session{
		PlayerID:  playerID,
		SessionID: sessionID,
		Name:      name,
		TCPConn:   tcpConn,
	}
}

func (s *Session) BindUDP(addr *net.UDPAddr) {
	if addr == nil {
		return
	}
	addrCopy := *addr
	s.udpAddr.Store(&addrCopy)
}

func (s *Session) UDPAddr() (*net.UDPAddr, bool) {
	addr := s.udpAddr.Load()
	if addr == nil {
		return nil, false
	}
	addrCopy := *addr
	return &addrCopy, true
}

func (s *Session) SetInput(input InputState) {
	s.inputMu.Lock()
	defer s.inputMu.Unlock()
	if input.Seq < s.lastInput.Seq {
		return
	}
	s.lastInput = input
}

func (s *Session) LastInput() InputState {
	s.inputMu.RLock()
	defer s.inputMu.RUnlock()
	return s.lastInput
}
