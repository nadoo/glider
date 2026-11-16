package anytls

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	cmdWaste byte = iota
	cmdSYN
	cmdPSH
	cmdFIN
	cmdSettings
	cmdAlert
	cmdUpdatePaddingScheme
	cmdSYNACK
	cmdHeartRequest
	cmdHeartResponse
	cmdServerSettings
)

const (
	protocolVersion = 2
	maxFrameData    = 65535
)

type frame struct {
	command  byte
	streamID uint32
	data     []byte
}

func passwordHash(password string) [32]byte {
	return sha256.Sum256([]byte(password))
}

func readAuth(r io.Reader, password string) error {
	var head [34]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return err
	}
	want := passwordHash(password)
	if subtle.ConstantTimeCompare(head[:32], want[:]) != 1 {
		return errors.New("authentication failed")
	}
	paddingLen := binary.BigEndian.Uint16(head[32:34])
	if paddingLen > 0 {
		_, err := io.CopyN(io.Discard, r, int64(paddingLen))
		return err
	}
	return nil
}

func writeAuth(w io.Writer, password string, paddingLen int) error {
	if paddingLen < 0 || paddingLen > 65535 {
		return fmt.Errorf("invalid auth padding length %d", paddingLen)
	}
	hash := passwordHash(password)
	buf := make([]byte, 34+paddingLen)
	copy(buf, hash[:])
	binary.BigEndian.PutUint16(buf[32:34], uint16(paddingLen))
	_, err := w.Write(buf)
	return err
}

func readFrame(r io.Reader) (frame, error) {
	var head [7]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return frame{}, err
	}
	n := binary.BigEndian.Uint16(head[5:7])
	var data []byte
	if n > 0 {
		data = make([]byte, n)
		if _, err := io.ReadFull(r, data); err != nil {
			return frame{}, err
		}
	}
	return frame{command: head[0], streamID: binary.BigEndian.Uint32(head[1:5]), data: data}, nil
}

func writeFrame(w io.Writer, f frame) (int, error) {
	if len(f.data) > maxFrameData {
		return 0, errors.New("frame data too large")
	}
	buf := make([]byte, 7+len(f.data))
	buf[0] = f.command
	binary.BigEndian.PutUint32(buf[1:5], f.streamID)
	binary.BigEndian.PutUint16(buf[5:7], uint16(len(f.data)))
	copy(buf[7:], f.data)
	n, err := w.Write(buf)
	if n < len(buf) && err == nil {
		err = io.ErrShortWrite
	}
	return max(0, n-7), err
}
