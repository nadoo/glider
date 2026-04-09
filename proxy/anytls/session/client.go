package session

import (
	"context"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// DialFunc is a function that creates a new outbound connection.
type DialFunc func(ctx context.Context) (net.Conn, error)

// Client manages a pool of sessions for stream multiplexing.
type Client struct {
	ctx    context.Context
	cancel context.CancelFunc

	dialOut DialFunc

	sessionCounter atomic.Uint64

	idleSessions []*Session
	idleLock     sync.Mutex

	sessions     map[uint64]*Session
	sessionsLock sync.Mutex

	idleTimeout time.Duration
}

// NewClient creates a new session pool client.
func NewClient(ctx context.Context, dialOut DialFunc, idleCheckInterval, idleTimeout time.Duration) *Client {
	if idleCheckInterval < 5*time.Second {
		idleCheckInterval = 30 * time.Second
	}
	if idleTimeout < 5*time.Second {
		idleTimeout = 30 * time.Second
	}

	c := &Client{
		dialOut:     dialOut,
		sessions:    make(map[uint64]*Session),
		idleTimeout: idleTimeout,
	}
	c.ctx, c.cancel = context.WithCancel(ctx)

	go c.idleCleanupLoop(idleCheckInterval)
	return c
}

// CreateStream opens a new stream, reusing an idle session or creating a new one.
func (c *Client) CreateStream(ctx context.Context) (net.Conn, error) {
	select {
	case <-c.ctx.Done():
		return nil, io.ErrClosedPipe
	default:
	}

	sess := c.getIdleSession()
	if sess == nil {
		var err error
		sess, err = c.createSession(ctx)
		if err != nil {
			return nil, err
		}
	}

	stream, err := sess.OpenStream()
	if err != nil {
		sess.Close()
		return nil, err
	}

	// When the stream closes, return the session to the idle pool.
	stream.CloseFunc = func() error {
		err := stream.CloseRemote()
		if !sess.IsClosed() {
			select {
			case <-c.ctx.Done():
				go sess.Close()
			default:
				c.idleLock.Lock()
				sess.IdleSince = time.Now()
				c.idleSessions = append(c.idleSessions, sess)
				c.idleLock.Unlock()
			}
		}
		return err
	}

	return stream, nil
}

func (c *Client) getIdleSession() *Session {
	c.idleLock.Lock()
	defer c.idleLock.Unlock()

	// Reuse the newest idle session (last in slice).
	for len(c.idleSessions) > 0 {
		n := len(c.idleSessions)
		sess := c.idleSessions[n-1]
		c.idleSessions = c.idleSessions[:n-1]
		if !sess.IsClosed() {
			return sess
		}
	}
	return nil
}

func (c *Client) createSession(ctx context.Context) (*Session, error) {
	conn, err := c.dialOut(ctx)
	if err != nil {
		return nil, err
	}

	sess := NewClientSession(conn)
	sess.Seq = c.sessionCounter.Add(1)
	sess.DieHook = func() {
		c.idleLock.Lock()
		for i, s := range c.idleSessions {
			if s == sess {
				c.idleSessions = append(c.idleSessions[:i], c.idleSessions[i+1:]...)
				break
			}
		}
		c.idleLock.Unlock()

		c.sessionsLock.Lock()
		delete(c.sessions, sess.Seq)
		c.sessionsLock.Unlock()
	}

	c.sessionsLock.Lock()
	c.sessions[sess.Seq] = sess
	c.sessionsLock.Unlock()

	sess.Run()
	return sess, nil
}

// Close shuts down the client and all sessions.
func (c *Client) Close() error {
	c.cancel()

	c.sessionsLock.Lock()
	toClose := make([]*Session, 0, len(c.sessions))
	for _, sess := range c.sessions {
		toClose = append(toClose, sess)
	}
	c.sessions = make(map[uint64]*Session)
	c.sessionsLock.Unlock()

	for _, sess := range toClose {
		sess.Close()
	}
	return nil
}

func (c *Client) idleCleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.idleCleanup()
		}
	}
}

func (c *Client) idleCleanup() {
	expTime := time.Now().Add(-c.idleTimeout)
	var toClose []*Session

	c.idleLock.Lock()
	remaining := c.idleSessions[:0]
	for _, sess := range c.idleSessions {
		if sess.IsClosed() {
			continue
		}
		if sess.IdleSince.Before(expTime) {
			toClose = append(toClose, sess)
		} else {
			remaining = append(remaining, sess)
		}
	}
	c.idleSessions = remaining
	c.idleLock.Unlock()

	for _, sess := range toClose {
		sess.Close()
	}
}
