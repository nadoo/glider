package anytls

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type session struct {
	conn net.Conn

	writeMu sync.Mutex
	mu      sync.Mutex
	streams map[uint32]*stream
	synack  map[uint32]chan synackResult
	nextID  atomic.Uint32

	incoming  chan *stream
	done      chan struct{}
	closeOnce sync.Once
	err       atomic.Value

	settingsSeen bool
}

type synackResult struct {
	data []byte
}

func newSession(conn net.Conn) *session {
	s := &session{
		conn:     conn,
		streams:  map[uint32]*stream{},
		synack:   map[uint32]chan synackResult{},
		incoming: make(chan *stream, 32),
		done:     make(chan struct{}),
	}
	s.nextID.Store(1)
	return s
}

func (s *session) start() {
	go s.readLoop()
}

func (s *session) acceptStream() (*stream, error) {
	select {
	case st, ok := <-s.incoming:
		if !ok {
			return nil, s.Err()
		}
		return st, nil
	case <-s.done:
		return nil, s.Err()
	}
}

func (s *session) openStream() (*stream, error) {
	id := s.nextID.Add(1) - 1
	st := newStream(id, s)
	s.mu.Lock()
	s.streams[id] = st
	s.synack[id] = make(chan synackResult, 1)
	s.mu.Unlock()
	if err := s.writeFrame(frame{command: cmdSYN, streamID: id}); err != nil {
		s.removeStream(id)
		return nil, err
	}
	return st, nil
}

func (s *session) waitSYNACK(id uint32, timeout time.Duration) error {
	s.mu.Lock()
	ch := s.synack[id]
	s.mu.Unlock()
	if ch == nil {
		return nil
	}
	var timer <-chan time.Time
	if timeout > 0 {
		t := time.NewTimer(timeout)
		defer t.Stop()
		timer = t.C
	}
	select {
	case r, ok := <-ch:
		if !ok {
			return s.Err()
		}
		if len(r.data) > 0 {
			return fmt.Errorf("stream open failed: %s", string(r.data))
		}
		return nil
	case <-timer:
		return errors.New("timeout waiting for SYNACK")
	case <-s.done:
		return s.Err()
	}
}

func (s *session) writeFrame(f frame) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return writeFrame(s.conn, f)
}

func (s *session) readLoop() {
	for {
		f, err := readFrame(s.conn)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				s.setErr(err)
			}
			s.Close()
			return
		}
		if err := s.handleFrame(f); err != nil {
			s.setErr(err)
			s.Close()
			return
		}
	}
}

func (s *session) handleFrame(f frame) error {
	switch f.command {
	case cmdWaste:
		return nil
	case cmdHeartRequest:
		return s.writeFrame(frame{command: cmdHeartResponse, streamID: f.streamID})
	case cmdHeartResponse:
		return nil
	case cmdSettings:
		m := parseSettings(f.data)
		s.settingsSeen = true
		if settingsVersion(m) >= 2 {
			return s.writeFrame(frame{command: cmdServerSettings, data: serverSettings()})
		}
	case cmdServerSettings:
		return nil
	case cmdAlert:
		return errors.New("alert: " + string(f.data))
	case cmdUpdatePaddingScheme:
		return nil
	case cmdSYN:
		if !s.settingsSeen {
			_ = s.writeFrame(frame{command: cmdAlert, data: []byte("cmdSYN received before cmdSettings")})
			return errors.New("cmdSYN received before cmdSettings")
		}
		st := newStream(f.streamID, s)
		s.mu.Lock()
		s.streams[f.streamID] = st
		s.mu.Unlock()
		select {
		case s.incoming <- st:
		case <-s.done:
		}
	case cmdSYNACK:
		s.mu.Lock()
		ch := s.synack[f.streamID]
		delete(s.synack, f.streamID)
		s.mu.Unlock()
		if ch != nil {
			ch <- synackResult{data: f.data}
			close(ch)
		}
	case cmdPSH:
		st := s.getStream(f.streamID)
		if st != nil {
			st.push(f.data)
		}
	case cmdFIN:
		st := s.getStream(f.streamID)
		if st != nil {
			st.closeRead()
			s.removeStream(f.streamID)
		}
	default:
		return errors.New("unknown command")
	}
	return nil
}

func (s *session) getStream(id uint32) *stream {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.streams[id]
}

func (s *session) removeStream(id uint32) {
	s.mu.Lock()
	delete(s.streams, id)
	if ch := s.synack[id]; ch != nil {
		delete(s.synack, id)
		close(ch)
	}
	s.mu.Unlock()
}

func (s *session) setErr(err error) {
	if err != nil && s.err.Load() == nil {
		s.err.Store(err)
	}
}

func (s *session) Err() error {
	if v := s.err.Load(); v != nil {
		return v.(error)
	}
	return net.ErrClosed
}

func (s *session) Close() error {
	s.closeOnce.Do(func() {
		close(s.done)
		_ = s.conn.Close()
		s.mu.Lock()
		for _, st := range s.streams {
			st.closeRead()
		}
		s.streams = map[uint32]*stream{}
		for id, ch := range s.synack {
			delete(s.synack, id)
			close(ch)
		}
		close(s.incoming)
		s.mu.Unlock()
	})
	return nil
}
