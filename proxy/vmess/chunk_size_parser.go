package vmess

import (
	"encoding/binary"
	"golang.org/x/crypto/sha3"
)

// ChunkSizeEncoder is a utility class to encode size value into bytes.
type ChunkSizeEncoder interface {
	SizeBytes() int32
	Encode(uint16, []byte) []byte
}

// ChunkSizeDecoder is a utility class to decode size value from bytes.
type ChunkSizeDecoder interface {
	SizeBytes() int32
	Decode([]byte) (uint16, error)
}

type ShakeSizeParser struct {
	shake  sha3.ShakeHash
	buffer [2]byte
}

func NewShakeSizeParser(nonce []byte) *ShakeSizeParser {
	shake := sha3.NewShake128()
	shake.Write(nonce)
	return &ShakeSizeParser{
		shake: shake,
	}
}

func (*ShakeSizeParser) SizeBytes() int32 {
	return 2
}

func (s *ShakeSizeParser) next() uint16 {
	s.shake.Read(s.buffer[:])
	return binary.BigEndian.Uint16(s.buffer[:])
}

func (s *ShakeSizeParser) Decode(b []byte) (uint16, error) {
	mask := s.next()
	size := binary.BigEndian.Uint16(b)
	return mask ^ size, nil
}

func (s *ShakeSizeParser) Encode(size uint16, b []byte) []byte {
	mask := s.next()
	binary.BigEndian.PutUint16(b, mask^size)
	return b[:2]
}

func (s *ShakeSizeParser) NextPaddingLen() uint16 {
	return s.next() % 64
}

func (s *ShakeSizeParser) MaxPaddingLen() uint16 {
	return 64
}
