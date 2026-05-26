package session

import (
	"net"
	"sync"
)

type Manager struct {
	mu sync.RWMutex

	nextPlayerID  uint32
	nextSessionID uint64

	bySessionID map[uint64]*Session
	byPlayerID  map[uint32]*Session
}

func NewManager() *Manager {
	return &Manager{
		nextPlayerID:  1000,
		nextSessionID: 1,
		bySessionID:   make(map[uint64]*Session),
		byPlayerID:    make(map[uint32]*Session),
	}
}

func (m *Manager) Create(name string, tcpConn net.Conn) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextPlayerID++
	m.nextSessionID++

	session := New(m.nextPlayerID, m.nextSessionID, name, tcpConn)
	m.bySessionID[session.SessionID] = session
	m.byPlayerID[session.PlayerID] = session
	return session
}

func (m *Manager) Remove(sessionID uint64) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.bySessionID[sessionID]
	if !ok {
		return nil
	}
	delete(m.bySessionID, sessionID)
	delete(m.byPlayerID, session.PlayerID)
	return session
}

func (m *Manager) GetBySessionID(sessionID uint64) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.bySessionID[sessionID]
	return session, ok
}

func (m *Manager) GetByPlayerID(playerID uint32) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.byPlayerID[playerID]
	return session, ok
}

func (m *Manager) List() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make([]*Session, 0, len(m.bySessionID))
	for _, session := range m.bySessionID {
		sessions = append(sessions, session)
	}
	return sessions
}
