package vmess

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"hash/crc32"
	"io"
	"time"
)

// https://github.com/v2fly/v2ray-core/tree/327cbdc5cd10ee1aec6ef4dce1af027b3e277f33/proxy/vmess/aead
const (
	kdfSaltConstAuthIDEncryptionKey             = "AES Auth ID Encryption"
	kdfSaltConstAEADRespHeaderLenKey            = "AEAD Resp Header Len Key"
	kdfSaltConstAEADRespHeaderLenIV             = "AEAD Resp Header Len IV"
	kdfSaltConstAEADRespHeaderPayloadKey        = "AEAD Resp Header Key"
	kdfSaltConstAEADRespHeaderPayloadIV         = "AEAD Resp Header IV"
	kdfSaltConstVMessAEADKDF                    = "VMess AEAD KDF"
	kdfSaltConstVMessHeaderPayloadAEADKey       = "VMess Header AEAD Key"
	kdfSaltConstVMessHeaderPayloadAEADIV        = "VMess Header AEAD Nonce"
	kdfSaltConstVMessHeaderPayloadLengthAEADKey = "VMess Header AEAD Key_Length"
	kdfSaltConstVMessHeaderPayloadLengthAEADIV  = "VMess Header AEAD Nonce_Length"
)

func sealVMessAEADHeader(key [16]byte, data []byte) []byte {
	generatedAuthID := createAuthID(key[:], time.Now().Unix())
	connectionNonce := make([]byte, 8)
	rand.Read(connectionNonce)

	aeadPayloadLengthSerializedByte := make([]byte, 2)
	binary.BigEndian.PutUint16(aeadPayloadLengthSerializedByte, uint16(len(data)))

	var payloadHeaderLengthAEADEncrypted []byte

	{
		payloadHeaderLengthAEADKey := kdf(key[:], kdfSaltConstVMessHeaderPayloadLengthAEADKey, string(generatedAuthID[:]), string(connectionNonce))[:16]
		payloadHeaderLengthAEADNonce := kdf(key[:], kdfSaltConstVMessHeaderPayloadLengthAEADIV, string(generatedAuthID[:]), string(connectionNonce))[:12]
		payloadHeaderLengthAEADAESBlock, _ := aes.NewCipher(payloadHeaderLengthAEADKey)
		payloadHeaderAEAD, _ := cipher.NewGCM(payloadHeaderLengthAEADAESBlock)
		payloadHeaderLengthAEADEncrypted = payloadHeaderAEAD.Seal(nil, payloadHeaderLengthAEADNonce, aeadPayloadLengthSerializedByte, generatedAuthID[:])
	}

	var payloadHeaderAEADEncrypted []byte

	{
		payloadHeaderAEADKey := kdf(key[:], kdfSaltConstVMessHeaderPayloadAEADKey, string(generatedAuthID[:]), string(connectionNonce))[:16]
		payloadHeaderAEADNonce := kdf(key[:], kdfSaltConstVMessHeaderPayloadAEADIV, string(generatedAuthID[:]), string(connectionNonce))[:12]
		payloadHeaderAEADAESBlock, _ := aes.NewCipher(payloadHeaderAEADKey)
		payloadHeaderAEAD, _ := cipher.NewGCM(payloadHeaderAEADAESBlock)
		payloadHeaderAEADEncrypted = payloadHeaderAEAD.Seal(nil, payloadHeaderAEADNonce, data, generatedAuthID[:])
	}

	var outputBuffer = &bytes.Buffer{}

	outputBuffer.Write(generatedAuthID[:])
	outputBuffer.Write(payloadHeaderLengthAEADEncrypted)
	outputBuffer.Write(connectionNonce)
	outputBuffer.Write(payloadHeaderAEADEncrypted)

	return outputBuffer.Bytes()
}

func openVMessAEADHeader(key [16]byte, iv [16]byte, r io.Reader) ([]byte, error) {
	aeadResponseHeaderLengthEncryptionKey := kdf(key[:], kdfSaltConstAEADRespHeaderLenKey)[:16]
	aeadResponseHeaderLengthEncryptionIV := kdf(iv[:], kdfSaltConstAEADRespHeaderLenIV)[:12]

	aeadResponseHeaderLengthEncryptionKeyAESBlock, _ := aes.NewCipher(aeadResponseHeaderLengthEncryptionKey)
	aeadResponseHeaderLengthEncryptionAEAD, _ := cipher.NewGCM(aeadResponseHeaderLengthEncryptionKeyAESBlock)

	aeadEncryptedResponseHeaderLength := make([]byte, 18)
	if _, err := io.ReadFull(r, aeadEncryptedResponseHeaderLength); err != nil {
		return nil, err
	}

	decryptedResponseHeaderLengthBinaryBuffer, err := aeadResponseHeaderLengthEncryptionAEAD.Open(nil, aeadResponseHeaderLengthEncryptionIV, aeadEncryptedResponseHeaderLength[:], nil)
	if err != nil {
		return nil, err
	}

	decryptedResponseHeaderLength := binary.BigEndian.Uint16(decryptedResponseHeaderLengthBinaryBuffer)
	aeadResponseHeaderPayloadEncryptionKey := kdf(key[:], kdfSaltConstAEADRespHeaderPayloadKey)[:16]
	aeadResponseHeaderPayloadEncryptionIV := kdf(iv[:], kdfSaltConstAEADRespHeaderPayloadIV)[:12]
	aeadResponseHeaderPayloadEncryptionKeyAESBlock, _ := aes.NewCipher(aeadResponseHeaderPayloadEncryptionKey)
	aeadResponseHeaderPayloadEncryptionAEAD, _ := cipher.NewGCM(aeadResponseHeaderPayloadEncryptionKeyAESBlock)

	encryptedResponseHeaderBuffer := make([]byte, decryptedResponseHeaderLength+16)
	if _, err := io.ReadFull(r, encryptedResponseHeaderBuffer); err != nil {
		return nil, err
	}

	return aeadResponseHeaderPayloadEncryptionAEAD.Open(nil, aeadResponseHeaderPayloadEncryptionIV, encryptedResponseHeaderBuffer, nil)
}

func kdf(key []byte, path ...string) []byte {
	hmacCreator := &hMacCreator{value: []byte(kdfSaltConstVMessAEADKDF)}
	for _, v := range path {
		hmacCreator = &hMacCreator{value: []byte(v), parent: hmacCreator}
	}
	hmacf := hmacCreator.Create()
	hmacf.Write(key)
	return hmacf.Sum(nil)
}

type hMacCreator struct {
	parent *hMacCreator
	value  []byte
}

func (h *hMacCreator) Create() hash.Hash {
	if h.parent == nil {
		return hmac.New(sha256.New, h.value)
	}
	return hmac.New(h.parent.Create, h.value)
}

func createAuthID(cmdKey []byte, time int64) [16]byte {
	buf := &bytes.Buffer{}
	binary.Write(buf, binary.BigEndian, time)

	random := make([]byte, 4)
	rand.Read(random)
	buf.Write(random)
	zero := crc32.ChecksumIEEE(buf.Bytes())
	binary.Write(buf, binary.BigEndian, zero)

	aesBlock, _ := aes.NewCipher(kdf(cmdKey[:], kdfSaltConstAuthIDEncryptionKey)[:16])
	var result [16]byte
	aesBlock.Encrypt(result[:], buf.Bytes())
	return result
}
