package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync/atomic"
	"time"

	"agentDemo/internal/gameplay"
	"agentDemo/internal/netproto"
	"agentDemo/internal/session"
	"agentDemo/internal/transport"
)

type Config struct {
	TCPAddr string
	UDPAddr string
	TickHz  uint32
}

type Server struct {
	cfg Config

	tcpListener net.Listener
	udpConn     *net.UDPConn

	sessions *session.Manager
	world    *gameplay.Runtime

	logger *log.Logger

	gameOverSent atomic.Bool
}

func New(cfg Config, logger *log.Logger) *Server {
	if cfg.TickHz == 0 {
		cfg.TickHz = 60
	}
	if logger == nil {
		logger = log.Default()
	}

	return &Server{
		cfg:      cfg,
		sessions: session.NewManager(),
		world:    gameplay.NewRuntime(cfg.TickHz),
		logger:   logger,
	}
}

func (s *Server) Run(ctx context.Context) error {
	tcpListener, err := net.Listen("tcp", s.cfg.TCPAddr)
	if err != nil {
		return err
	}
	s.tcpListener = tcpListener

	udpAddr, err := net.ResolveUDPAddr("udp", s.cfg.UDPAddr)
	if err != nil {
		return err
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	s.udpConn = udpConn

	s.logger.Printf("server listening tcp=%s udp=%s tick_hz=%d", tcpListener.Addr(), udpConn.LocalAddr(), s.cfg.TickHz)

	errCh := make(chan error, 3)
	go s.acceptLoop(errCh)
	go s.udpLoop(errCh)
	go s.tickLoop(ctx, errCh)

	select {
	case <-ctx.Done():
		_ = s.Close()
		return nil
	case err := <-errCh:
		_ = s.Close()
		return err
	}
}

func (s *Server) Close() error {
	var errs []error
	if s.tcpListener != nil {
		if err := s.tcpListener.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.udpConn != nil {
		if err := s.udpConn.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	for _, session := range s.sessions.List() {
		if session.TCPConn != nil {
			_ = session.TCPConn.Close()
		}
	}
	return errors.Join(errs...)
}

func (s *Server) acceptLoop(errCh chan<- error) {
	for {
		conn, err := s.tcpListener.Accept()
		if err != nil {
			if isClosedError(err) {
				return
			}
			errCh <- err
			return
		}
		go s.handleTCPConn(conn)
	}
}

func (s *Server) handleTCPConn(conn net.Conn) {
	defer conn.Close()

	envelope, err := transport.ReadTCPEnvelope(conn)
	if err != nil {
		s.logger.Printf("tcp read failed: %v", err)
		return
	}

	joinReq := envelope.GetJoinReq()
	if joinReq == nil {
		_ = transport.WriteTCPEnvelope(conn, &netproto.TcpEnvelope{
			Msg: &netproto.TcpEnvelope_Error{
				Error: &netproto.ErrorMsg{Code: 400, Message: "expected join_req as first tcp message"},
			},
		})
		return
	}

	sess := s.sessions.Create(joinReq.GetName(), conn)
	s.world.AddPlayer(sess.PlayerID)

	resp := &netproto.TcpEnvelope{
		Msg: &netproto.TcpEnvelope_JoinResp{
			JoinResp: &netproto.JoinResp{
				PlayerId:  sess.PlayerID,
				SessionId: sess.SessionID,
				UdpPort:   uint32(s.udpConn.LocalAddr().(*net.UDPAddr).Port),
				TickHz:    s.cfg.TickHz,
			},
		},
	}
	if err := transport.WriteTCPEnvelope(conn, resp); err != nil {
		s.logger.Printf("tcp join response failed: %v", err)
		s.removeSession(sess)
		return
	}

	s.logger.Printf("player joined player_id=%d session_id=%d name=%q", sess.PlayerID, sess.SessionID, sess.Name)

	for {
		envelope, err := transport.ReadTCPEnvelope(conn)
		if err != nil {
			s.logger.Printf("tcp session closed player_id=%d err=%v", sess.PlayerID, err)
			s.removeSession(sess)
			return
		}
		if envelope.GetJoinReq() != nil {
			_ = transport.WriteTCPEnvelope(conn, &netproto.TcpEnvelope{
				Msg: &netproto.TcpEnvelope_Error{
					Error: &netproto.ErrorMsg{Code: 409, Message: "join already completed"},
				},
			})
		}
	}
}

func (s *Server) removeSession(sess *session.Session) {
	s.sessions.Remove(sess.SessionID)
	s.world.RemovePlayer(sess.PlayerID)
}

func (s *Server) udpLoop(errCh chan<- error) {
	buf := make([]byte, transport.MaxUDPPacketSize)
	for {
		n, addr, err := s.udpConn.ReadFromUDP(buf)
		if err != nil {
			if isClosedError(err) {
				return
			}
			errCh <- err
			return
		}

		envelope, err := transport.UnmarshalUDPEnvelope(buf[:n])
		if err != nil {
			s.logger.Printf("udp decode failed addr=%s err=%v", addr, err)
			continue
		}
		s.handleUDPEnvelope(addr, envelope)
	}
}

func (s *Server) handleUDPEnvelope(addr *net.UDPAddr, envelope *netproto.UdpEnvelope) {
	sess, ok := s.sessions.GetBySessionID(envelope.GetSessionId())
	if !ok {
		s.logger.Printf("udp unknown session_id=%d from=%s", envelope.GetSessionId(), addr)
		return
	}

	switch msg := envelope.Msg.(type) {
	case *netproto.UdpEnvelope_Hello:
		sess.BindUDP(addr)
		s.logger.Printf("udp bound player_id=%d addr=%s", sess.PlayerID, addr)
	case *netproto.UdpEnvelope_Input:
		input := session.InputStateFromProto(msg.Input)
		sess.SetInput(input)
		s.world.SetInput(sess.PlayerID, input)
	default:
		s.logger.Printf("udp unsupported message player_id=%d from=%s", sess.PlayerID, addr)
	}
}

func (s *Server) tickLoop(ctx context.Context, errCh chan<- error) {
	interval := time.Second / time.Duration(s.cfg.TickHz)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.world.Tick()
			s.broadcastSnapshots()
			if s.world.Running() {
				s.gameOverSent.Store(false)
				continue
			}
			if !s.gameOverSent.Swap(true) {
				s.broadcastGameOver()
			}
		}
	}
}

func (s *Server) broadcastSnapshots() {
	for _, sess := range s.sessions.List() {
		addr, ok := sess.UDPAddr()
		if !ok {
			continue
		}
		snapshot := s.world.BuildSnapshotFor(sess.PlayerID)
		envelope := &netproto.UdpEnvelope{
			SessionId: sess.SessionID,
			Msg: &netproto.UdpEnvelope_Snapshot{
				Snapshot: snapshot,
			},
		}
		payload, err := transport.MarshalUDPEnvelope(envelope)
		if err != nil {
			s.logger.Printf("snapshot marshal failed player_id=%d err=%v", sess.PlayerID, err)
			continue
		}
		if _, err := s.udpConn.WriteToUDP(payload, addr); err != nil {
			s.logger.Printf("snapshot send failed player_id=%d err=%v", sess.PlayerID, err)
		}
	}
}

func (s *Server) broadcastGameOver() {
	payload := s.world.GameOver()
	for _, sess := range s.sessions.List() {
		if sess.TCPConn == nil {
			continue
		}
		err := transport.WriteTCPEnvelope(sess.TCPConn, &netproto.TcpEnvelope{
			Msg: &netproto.TcpEnvelope_GameOver{
				GameOver: payload,
			},
		})
		if err != nil {
			s.logger.Printf("game over send failed player_id=%d err=%v", sess.PlayerID, err)
		}
	}
}

func isClosedError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	return false
}

func (s *Server) String() string {
	return fmt.Sprintf("Server{tcp=%s, udp=%s, tick_hz=%d}", s.cfg.TCPAddr, s.cfg.UDPAddr, s.cfg.TickHz)
}
