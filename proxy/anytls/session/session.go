package session

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nadoo/glider/pkg/log"
)

const protocolVersion = "2"
const clientName = "anytls/0.0.11"

// Session multiplexes streams over a single connection.
type Session struct {
	conn     net.Conn
	connLock sync.Mutex

	streams    map[uint32]*Stream
	streamID   atomic.Uint32
	streamLock sync.RWMutex

	dieOnce sync.Once
	die     chan struct{}
	DieHook func()

	// pool fields
	Seq       uint64
	IdleSince time.Time

	peerVersion byte

	// buffering: buffer initial frames until first stream opens
	buffering bool
	buffer    []byte
}

// NewClientSession creates a new client-side session.
func NewClientSession(conn net.Conn) *Session {
	s := &Session{
		conn:      conn,
		buffering: true,
		die:       make(chan struct{}),
		streams:   make(map[uint32]*Stream),
	}
	return s
}

// Run starts the session. For clients, it sends settings then starts the recv loop.
func (s *Session) Run() {
	settings := fmt.Sprintf("v=%s\nclient=%s\n", protocolVersion, clientName)
	f := newFrame(cmdSettings, 0)
	f.data = []byte(settings)
	s.writeControlFrame(f)

	go s.recvLoop()
}

// IsClosed returns true if the session is closed.
func (s *Session) IsClosed() bool {
	select {
	case <-s.die:
		return true
	default:
		return false
	}
}

// Close closes the session and all its streams.
func (s *Session) Close() error {
	var once bool
	s.dieOnce.Do(func() {
		close(s.die)
		once = true
	})
	if once {
		if s.DieHook != nil {
			s.DieHook()
			s.DieHook = nil
		}
		s.streamLock.Lock()
		for _, stream := range s.streams {
			stream.closeLocally()
		}
		s.streams = make(map[uint32]*Stream)
		s.streamLock.Unlock()
		return s.conn.Close()
	}
	return io.ErrClosedPipe
}

// OpenStream opens a new stream on the session.
func (s *Session) OpenStream() (*Stream, error) {
	if s.IsClosed() {
		return nil, io.ErrClosedPipe
	}

	sid := s.streamID.Add(1)
	stream := newStream(sid, s)

	if _, err := s.writeControlFrame(newFrame(cmdSYN, sid)); err != nil {
		return nil, err
	}

	s.buffering = false

	s.streamLock.Lock()
	defer s.streamLock.Unlock()
	select {
	case <-s.die:
		return nil, io.ErrClosedPipe
	default:
		s.streams[sid] = stream
		return stream, nil
	}
}

func (s *Session) recvLoop() {
	defer s.Close()

	var hdr rawHeader
	for {
		if s.IsClosed() {
			return
		}

		if _, err := io.ReadFull(s.conn, hdr[:]); err != nil {
			return
		}

		sid := hdr.StreamID()
		dataLen := int(hdr.Length())

		switch hdr.Cmd() {
		case cmdPSH:
			if dataLen > 0 {
				buf := make([]byte, dataLen)
				if _, err := io.ReadFull(s.conn, buf); err != nil {
					return
				}
				s.streamLock.RLock()
				stream, ok := s.streams[sid]
				s.streamLock.RUnlock()
				if ok {
					stream.pw.Write(buf)
				}
			}

		case cmdFIN:
			s.streamLock.Lock()
			stream, ok := s.streams[sid]
			delete(s.streams, sid)
			s.streamLock.Unlock()
			if ok {
				stream.closeLocally()
			}

		case cmdSYNACK:
			if dataLen > 0 {
				buf := make([]byte, dataLen)
				if _, err := io.ReadFull(s.conn, buf); err != nil {
					return
				}
				// non-empty SYNACK means handshake failure
				s.streamLock.RLock()
				stream, ok := s.streams[sid]
				s.streamLock.RUnlock()
				if ok {
					stream.dieErr = fmt.Errorf("remote: %s", string(buf))
					stream.pr.CloseWithError(stream.dieErr)
				}
			}

		case cmdWaste:
			if dataLen > 0 {
				buf := make([]byte, dataLen)
				if _, err := io.ReadFull(s.conn, buf); err != nil {
					return
				}
				// discard
			}

		case cmdAlert:
			if dataLen > 0 {
				buf := make([]byte, dataLen)
				if _, err := io.ReadFull(s.conn, buf); err != nil {
					return
				}
				log.F("[anytls] alert from server: %s", string(buf))
				return
			}

		case cmdServerSettings:
			if dataLen > 0 {
				buf := make([]byte, dataLen)
				if _, err := io.ReadFull(s.conn, buf); err != nil {
					return
				}
				m := parseStringMap(string(buf))
				if v, err := strconv.Atoi(m["v"]); err == nil {
					s.peerVersion = byte(v)
				}
			}

		case cmdUpdatePaddingScheme:
			// We don't implement dynamic padding updates for simplicity;
			// just consume the data.
			if dataLen > 0 {
				buf := make([]byte, dataLen)
				if _, err := io.ReadFull(s.conn, buf); err != nil {
					return
				}
			}

		case cmdHeartRequest:
			s.writeControlFrame(newFrame(cmdHeartResponse, sid))

		case cmdHeartResponse:
			// no-op

		case cmdSettings:
			// Server shouldn't send this to client, but consume anyway
			if dataLen > 0 {
				buf := make([]byte, dataLen)
				if _, err := io.ReadFull(s.conn, buf); err != nil {
					return
				}
			}

		default:
			// Unknown command: consume data
			if dataLen > 0 {
				buf := make([]byte, dataLen)
				if _, err := io.ReadFull(s.conn, buf); err != nil {
					return
				}
			}
		}
	}
}

func (s *Session) streamClosed(sid uint32) error {
	if s.IsClosed() {
		return io.ErrClosedPipe
	}
	_, err := s.writeControlFrame(newFrame(cmdFIN, sid))
	s.streamLock.Lock()
	delete(s.streams, sid)
	s.streamLock.Unlock()
	return err
}

func (s *Session) writeDataFrame(sid uint32, data []byte) (int, error) {
	dataLen := len(data)
	buf := make([]byte, headerSize+dataLen)
	buf[0] = cmdPSH
	binary.BigEndian.PutUint32(buf[1:5], sid)
	binary.BigEndian.PutUint16(buf[5:7], uint16(dataLen))
	copy(buf[headerSize:], data)

	_, err := s.writeConn(buf)
	if err != nil {
		return 0, err
	}
	return dataLen, nil
}

func (s *Session) writeControlFrame(f frame) (int, error) {
	dataLen := len(f.data)
	buf := make([]byte, headerSize+dataLen)
	buf[0] = f.cmd
	binary.BigEndian.PutUint32(buf[1:5], f.sid)
	binary.BigEndian.PutUint16(buf[5:7], uint16(dataLen))
	copy(buf[headerSize:], f.data)

	s.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := s.writeConn(buf)
	if err != nil {
		s.Close()
		return 0, err
	}
	s.conn.SetWriteDeadline(time.Time{})
	return dataLen, nil
}

func (s *Session) writeConn(b []byte) (int, error) {
	s.connLock.Lock()
	defer s.connLock.Unlock()

	if s.buffering {
		s.buffer = append(s.buffer, b...)
		return len(b), nil
	}

	if len(s.buffer) > 0 {
		b = append(s.buffer, b...)
		s.buffer = nil
	}

	return s.conn.Write(b)
}

// NumStreams returns the number of active streams.
func (s *Session) NumStreams() int {
	s.streamLock.RLock()
	defer s.streamLock.RUnlock()
	return len(s.streams)
}

// parseStringMap parses newline-separated key=value pairs.
func parseStringMap(s string) map[string]string {
	m := make(map[string]string)
	for _, line := range strings.Split(s, "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return m
}
