package protocol

import (
	"bytes"
	"encoding/binary"

	"github.com/sun8911879/shadowsocksR/ssr"
	"github.com/sun8911879/shadowsocksR/tools"
)

func init() {
	register("verify_sha1", NewVerifySHA1)
	register("ota", NewVerifySHA1)
}

type verifySHA1 struct {
	ssr.ServerInfoForObfs
	hasSentHeader bool
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

func (v *verifySHA1) SetServerInfo(s *ssr.ServerInfoForObfs) {
	v.ServerInfoForObfs = *s
}

func (v *verifySHA1) GetServerInfo() (s *ssr.ServerInfoForObfs) {
	return &v.ServerInfoForObfs
}

func (v *verifySHA1) SetData(data interface{}) {

}

func (v *verifySHA1) GetData() interface{} {
	return nil
}

func (v *verifySHA1) PreEncrypt(data []byte) (encryptedData []byte, err error) {
	dataLength := len(data)
	offset := 0
	if !v.hasSentHeader {
		data[0] |= oneTimeAuthMask
		encryptedData = v.otaConnectAuth(data[:v.HeadLen])
		v.hasSentHeader = true
		dataLength -= v.HeadLen
		offset += v.HeadLen
	}
	const blockSize = 4096
	for dataLength > blockSize {
		chunkId := v.getAndIncreaseChunkId()
		b := v.otaReqChunkAuth(chunkId, data[offset:offset+blockSize])
		encryptedData = append(encryptedData, b...)
		dataLength -= blockSize
		offset += blockSize
	}
	if dataLength > 0 {
		chunkId := v.getAndIncreaseChunkId()
		b := v.otaReqChunkAuth(chunkId, data[offset:])
		encryptedData = append(encryptedData, b...)
	}
	return
}

func (v *verifySHA1) PostDecrypt(data []byte) ([]byte, int, error) {
	return data, 0, nil
}
