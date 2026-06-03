package session

import (
	"net"
	"sync"
	"sync/atomic"

	"agentDemo/internal/netproto"
)

type InputState struct {
	Seq               uint32
	MoveX             int32
	MoveY             int32
	Fire              bool
	Roll              bool
	AimDX             float32
	AimDY             float32
	AckedSnapshotTick uint32
}

func InputStateFromProto(frame *netproto.InputFrame) InputState {
	return InputState{
		Seq:               frame.GetInputSeq(),
		MoveX:             frame.GetMoveX(),
		MoveY:             frame.GetMoveY(),
		Fire:              frame.GetFire(),
		Roll:              frame.GetRoll(),
		AimDX:             frame.GetAimDx(),
		AimDY:             frame.GetAimDy(),
		AckedSnapshotTick: frame.GetAckedSnapshotTick(),
	}
}

type SnapshotState struct {
	Tick     uint32
	Entities map[uint32]*netproto.EntityState
}

type Session struct {
	PlayerID  uint32
	SessionID uint64
	Name      string
	TCPConn   net.Conn

	udpAddr atomic.Pointer[net.UDPAddr]

	inputMu   sync.RWMutex
	lastInput InputState

	syncMu               sync.Mutex
	ackedSnapshotTick    uint32
	lastFullSnapshotTick uint32
	forceFullSnapshot    bool
	sentSnapshots        map[uint32]SnapshotState
}

func New(playerID uint32, sessionID uint64, name string, tcpConn net.Conn) *Session {
	return &Session{
		PlayerID:          playerID,
		SessionID:         sessionID,
		Name:              name,
		TCPConn:           tcpConn,
		forceFullSnapshot: true,
		sentSnapshots:     make(map[uint32]SnapshotState),
	}
}

func (s *Session) BindUDP(addr *net.UDPAddr) {
	if addr == nil {
		return
	}
	addrCopy := *addr
	s.udpAddr.Store(&addrCopy)
	s.ResetSync()
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
	if input.Seq < s.lastInput.Seq {
		s.inputMu.Unlock()
		s.noteAckedSnapshotTick(input.AckedSnapshotTick)
		return
	}
	s.lastInput = input
	s.inputMu.Unlock()
	s.noteAckedSnapshotTick(input.AckedSnapshotTick)
}

func (s *Session) LastInput() InputState {
	s.inputMu.RLock()
	defer s.inputMu.RUnlock()
	return s.lastInput
}

func (s *Session) ResetSync() {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	s.ackedSnapshotTick = 0
	s.lastFullSnapshotTick = 0
	s.forceFullSnapshot = true
	s.sentSnapshots = make(map[uint32]SnapshotState)
}

func (s *Session) SyncBaseline() (ackedTick uint32, lastFullTick uint32, forceFull bool, state SnapshotState, ok bool) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	ackedTick = s.ackedSnapshotTick
	lastFullTick = s.lastFullSnapshotTick
	forceFull = s.forceFullSnapshot
	if ackedTick == 0 {
		return ackedTick, lastFullTick, forceFull, SnapshotState{}, false
	}

	baseState, exists := s.sentSnapshots[ackedTick]
	if !exists {
		return ackedTick, lastFullTick, forceFull, SnapshotState{}, false
	}
	return ackedTick, lastFullTick, forceFull, SnapshotState{
		Tick:     baseState.Tick,
		Entities: cloneEntityStateMap(baseState.Entities),
	}, true
}

func (s *Session) RecordSentSnapshot(tick uint32, entities map[uint32]*netproto.EntityState, wasFull bool, maxHistoryTicks uint32) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	s.sentSnapshots[tick] = SnapshotState{
		Tick:     tick,
		Entities: cloneEntityStateMap(entities),
	}
	if wasFull {
		s.lastFullSnapshotTick = tick
		s.forceFullSnapshot = false
	}
	s.pruneSnapshotHistoryLocked(tick, maxHistoryTicks)
}

func (s *Session) noteAckedSnapshotTick(tick uint32) {
	if tick == 0 {
		return
	}

	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	if tick <= s.ackedSnapshotTick {
		return
	}
	s.ackedSnapshotTick = tick
}

func (s *Session) pruneSnapshotHistoryLocked(currentTick uint32, maxHistoryTicks uint32) {
	if len(s.sentSnapshots) == 0 {
		return
	}

	var minTickToKeep uint32
	if maxHistoryTicks > 0 && currentTick > maxHistoryTicks {
		minTickToKeep = currentTick - maxHistoryTicks
	}

	for tick := range s.sentSnapshots {
		if tick == s.ackedSnapshotTick {
			continue
		}
		if s.ackedSnapshotTick > 0 && tick < s.ackedSnapshotTick {
			delete(s.sentSnapshots, tick)
			continue
		}
		if maxHistoryTicks > 0 && tick < minTickToKeep {
			delete(s.sentSnapshots, tick)
		}
	}
}

func cloneEntityStateMap(src map[uint32]*netproto.EntityState) map[uint32]*netproto.EntityState {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[uint32]*netproto.EntityState, len(src))
	for netID, entity := range src {
		dst[netID] = entity
	}
	return dst
}
