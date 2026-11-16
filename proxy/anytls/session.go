package anytls

import (
	"bytes"
	"container/list"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type session struct {
	conn net.Conn

	writeMu   sync.Mutex
	writes    list.List
	writeWake chan struct{}
	mu        sync.Mutex
	streams   map[uint32]*stream
	synack    map[uint32]chan synackResult
	nextID    atomic.Uint32

	incoming  chan *stream
	done      chan struct{}
	closeOnce sync.Once
	err       atomic.Value
	errOnce   sync.Once

	settingsSeen bool
}

type synackResult struct {
	data []byte
}

type frameWrite struct {
	frame    frame
	stream   *stream
	deadline <-chan struct{}
	result   chan frameWriteResult

	// Guarded by session.writeMu.
	elem    *list.Element
	started bool
}

type frameWriteResult struct {
	n   int // payload bytes, excluding the frame header
	err error
}

func newSession(conn net.Conn) *session {
	s := &session{
		conn:      conn,
		streams:   map[uint32]*stream{},
		synack:    map[uint32]chan synackResult{},
		incoming:  make(chan *stream, 32),
		done:      make(chan struct{}),
		writeWake: make(chan struct{}, 1),
	}
	s.nextID.Store(1)
	go s.writeLoop()
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
	req, err := s.queueFrame(f, nil, nil)
	if err != nil {
		return err
	}
	select {
	case result := <-req.result:
		return result.err
	case <-s.done:
		return s.Err()
	}
}

func (s *session) writeStreamFrame(st *stream, f frame) (int, error) {
	if channelClosed(st.done) {
		return 0, errStreamClosed
	}
	deadline := st.writeDeadline.wait()
	if channelClosed(deadline) {
		return 0, os.ErrDeadlineExceeded
	}
	req, err := s.queueFrame(f, st, deadline)
	if err != nil {
		return 0, err
	}
	select {
	case result := <-req.result:
		return result.n, result.err
	case <-st.done:
		err = errStreamClosed
	case <-s.done:
		err = s.Err()
	case <-deadline:
		err = os.ErrDeadlineExceeded
	}
	started := s.cancelWrite(req)
	// Prefer the transport result when both completion and cancellation are ready.
	select {
	case result := <-req.result:
		return result.n, result.err
	default:
	}
	if started {
		// The writer owns the whole frame once it starts. Its payload can
		// still reach the peer after this call returns, so count it in n.
		return len(f.data), err
	}
	return 0, err
}

func (s *session) queueFrame(f frame, st *stream, deadline <-chan struct{}) (*frameWrite, error) {
	// A stream may time out while the transport is writing. Own the payload so
	// the caller can reuse its buffer without racing with the session writer.
	f.data = bytes.Clone(f.data)
	req := &frameWrite{frame: f, stream: st, deadline: deadline, result: make(chan frameWriteResult, 1)}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if channelClosed(s.done) {
		return nil, s.Err()
	}
	req.elem = s.writes.PushBack(req)
	select {
	case s.writeWake <- struct{}{}:
	default:
	}
	return req, nil
}

// cancelWrite removes a pending request and reports whether it had already started.
func (s *session) cancelWrite(req *frameWrite) bool {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if req.elem != nil {
		s.writes.Remove(req.elem)
		req.elem = nil
	}
	return req.started
}

func (s *session) writeLoop() {
	for {
		select {
		case <-s.done:
			return
		case <-s.writeWake:
		}
		for {
			s.writeMu.Lock()
			elem := s.writes.Front()
			if elem == nil || channelClosed(s.done) {
				s.writeMu.Unlock()
				break
			}
			req := s.writes.Remove(elem).(*frameWrite)
			req.elem = nil
			if st := req.stream; st != nil {
				if channelClosed(st.done) {
					s.writeMu.Unlock()
					req.result <- frameWriteResult{err: errStreamClosed}
					continue
				}
				if channelClosed(req.deadline) {
					s.writeMu.Unlock()
					req.result <- frameWriteResult{err: os.ErrDeadlineExceeded}
					continue
				}
			}
			req.started = true
			s.writeMu.Unlock()
			// An in-flight frame must finish intact even if its stream times out.
			// Only closing the session may interrupt the shared transport write.
			n, err := writeFrame(s.conn, req.frame)
			req.result <- frameWriteResult{n: n, err: err}
			close(req.result)
			if err != nil {
				s.setErr(err)
				s.Close()
				return
			}
		}
	}
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
	if err != nil {
		s.errOnce.Do(func() { s.err.Store(err) })
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
		s.writeMu.Lock()
		for elem := s.writes.Front(); elem != nil; elem = elem.Next() {
			elem.Value.(*frameWrite).elem = nil
		}
		s.writes.Init()
		s.writeMu.Unlock()
		s.mu.Lock()
		for _, st := range s.streams {
			st.readDeadline.stop()
			st.writeDeadline.stop()
			st.closeRead()
		}
		s.streams = map[uint32]*stream{}
		for id, ch := range s.synack {
			delete(s.synack, id)
			close(ch)
		}
		s.mu.Unlock()
	})
	return nil
}
