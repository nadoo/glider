package protocol

import (
	"bytes"
	"encoding/binary"

	"github.com/nadoo/glider/proxy/ssr/internal/ssr"
	"github.com/nadoo/glider/proxy/ssr/internal/tools"
)

func init() {
	register("verify_sha1", NewVerifySHA1)
	register("ota", NewVerifySHA1)
}

type verifySHA1 struct {
	ssr.ServerInfo
	hasSentHeader bool
	buffer        bytes.Buffer
	chunkId       uint32
}

const (
	oneTimeAuthMask byte = 0x10
)

func NewVerifySHA1() IProtocol {
	a := &verifySHA1{}
	return a
}

func (v *verifySHA1) otaConnectAuth(data []byte) []byte {
	return append(data, tools.HmacSHA1(append(v.IV, v.Key...), data)...)
}

func (v *verifySHA1) otaReqChunkAuth(chunkId uint32, data []byte) []byte {
	nb := make([]byte, 2)
	binary.BigEndian.PutUint16(nb, uint16(len(data)))
	chunkIdBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(chunkIdBytes, chunkId)
	header := append(nb, tools.HmacSHA1(append(v.IV, chunkIdBytes...), data)...)
	return append(header, data...)
}

func (v *verifySHA1) otaVerifyAuth(iv []byte, chunkId uint32, data []byte, expectedHmacSha1 []byte) bool {
	chunkIdBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(chunkIdBytes, chunkId)
	actualHmacSha1 := tools.HmacSHA1(append(iv, chunkIdBytes...), data)
	return bytes.Equal(expectedHmacSha1, actualHmacSha1)
}

func (v *verifySHA1) getAndIncreaseChunkId() (chunkId uint32) {
	chunkId = v.chunkId
	v.chunkId += 1
	return
}

func (v *verifySHA1) SetServerInfo(s *ssr.ServerInfo) {
	v.ServerInfo = *s
}

func (v *verifySHA1) GetServerInfo() (s *ssr.ServerInfo) {
	return &v.ServerInfo
}

func (v *verifySHA1) SetData(data any) {

}

func (v *verifySHA1) GetData() any {
	return nil
}

func (v *verifySHA1) PreEncrypt(data []byte) (encryptedData []byte, err error) {
	v.buffer.Reset()
	dataLength := len(data)
	offset := 0
	if !v.hasSentHeader {
		data[0] |= oneTimeAuthMask
		v.buffer.Write(v.otaConnectAuth(data[:v.HeadLen]))
		v.hasSentHeader = true
		dataLength -= v.HeadLen
		offset += v.HeadLen
	}
	const blockSize = 4096
	for dataLength > blockSize {
		chunkId := v.getAndIncreaseChunkId()
		v.buffer.Write(v.otaReqChunkAuth(chunkId, data[offset:offset+blockSize]))
		dataLength -= blockSize
		offset += blockSize
	}
	if dataLength > 0 {
		chunkId := v.getAndIncreaseChunkId()
		v.buffer.Write(v.otaReqChunkAuth(chunkId, data[offset:]))
	}
	return v.buffer.Bytes(), nil
}

func (v *verifySHA1) PostDecrypt(data []byte) ([]byte, int, error) {
	return data, len(data), nil
}

func (v *verifySHA1) GetOverhead() int {
	return 0
}
