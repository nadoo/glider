// https://github.com/shadowsocksr-backup/shadowsocks-rss/blob/master/doc/auth_chain_a.md

package protocol

import (
	"bytes"
	"crypto/aes"
	stdCipher "crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/nadoo/glider/proxy/ssr/internal/cipher"
	"github.com/nadoo/glider/proxy/ssr/internal/ssr"
	"github.com/nadoo/glider/proxy/ssr/internal/tools"
)

func init() {
	register("auth_chain_a", NewAuthChainA)
}

type authChainA struct {
	ssr.ServerInfo
	randomClient tools.Shift128plusContext
	randomServer tools.Shift128plusContext
	recvInfo
	cipher         *cipher.StreamCipher
	hasSentHeader  bool
	lastClientHash []byte
	lastServerHash []byte
	userKey        []byte
	uid            [4]byte
	salt           string
	data           *AuthData
	hmac           hmacMethod
	hashDigest     hashDigestMethod
	rnd            rndMethod
	dataSizeList   []int
	dataSizeList2  []int
	chunkID        uint32
}

func NewAuthChainA() IProtocol {
	a := &authChainA{
		salt:       "auth_chain_a",
		hmac:       tools.HmacMD5,
		hashDigest: tools.SHA1Sum,
		rnd:        authChainAGetRandLen,
		recvInfo: recvInfo{
			recvID: 1,
			buffer: new(bytes.Buffer),
		},
	}
	return a
}

func (a *authChainA) SetServerInfo(s *ssr.ServerInfo) {
	a.ServerInfo = *s
	if a.salt == "auth_chain_b" {
		a.authChainBInitDataSize()
	}
}

func (a *authChainA) GetServerInfo() (s *ssr.ServerInfo) {
	return &a.ServerInfo
}

func (a *authChainA) SetData(data any) {
	if auth, ok := data.(*AuthData); ok {
		a.data = auth
	}
}

func (a *authChainA) GetData() any {
	if a.data == nil {
		a.data = &AuthData{}
	}
	return a.data
}

func authChainAGetRandLen(dataLength int, random *tools.Shift128plusContext, lastHash []byte, dataSizeList, dataSizeList2 []int, overhead int) int {
	if dataLength > 1440 {
		return 0
	}
	random.InitFromBinDatalen(lastHash[:16], dataLength)
	if dataLength > 1300 {
		return int(random.Next() % 31)
	}
	if dataLength > 900 {
		return int(random.Next() % 127)
	}
	if dataLength > 400 {
		return int(random.Next() % 521)
	}
	return int(random.Next() % 1021)
}

func getRandStartPos(random *tools.Shift128plusContext, randLength int) int {
	if randLength > 0 {
		return int(int64(random.Next()%8589934609) % int64(randLength))
	}
	return 0
}

func (a *authChainA) getClientRandLen(dataLength int, overhead int) int {
	return a.rnd(dataLength, &a.randomClient, a.lastClientHash, a.dataSizeList, a.dataSizeList2, overhead)
}

func (a *authChainA) getServerRandLen(dataLength int, overhead int) int {
	return a.rnd(dataLength, &a.randomServer, a.lastServerHash, a.dataSizeList, a.dataSizeList2, overhead)
}

func (a *authChainA) packedDataLen(data []byte) (chunkLength, randLength int) {
	dataLength := len(data)
	randLength = a.getClientRandLen(dataLength, a.Overhead)
	chunkLength = randLength + dataLength + 2 + 2
	return
}

func (a *authChainA) packData(outData []byte, data []byte, randLength int) {
	dataLength := len(data)
	outLength := randLength + dataLength + 2
	outData[0] = byte(dataLength) ^ a.lastClientHash[14]
	outData[1] = byte(dataLength>>8) ^ a.lastClientHash[15]

	{
		if dataLength > 0 {
			randPart1Length := getRandStartPos(&a.randomClient, randLength)
			rand.Read(outData[2 : 2+randPart1Length])
			a.cipher.Encrypt(outData[2+randPart1Length:], data)
			rand.Read(outData[2+randPart1Length+dataLength : outLength])
		} else {
			rand.Read(outData[2 : 2+randLength])
		}
	}

	userKeyLen := uint8(len(a.userKey))
	key := make([]byte, userKeyLen+4)
	copy(key, a.userKey)
	a.chunkID++
	binary.LittleEndian.PutUint32(key[userKeyLen:], a.chunkID)
	a.lastClientHash = a.hmac(key, outData[:outLength])
	copy(outData[outLength:], a.lastClientHash[:2])
	return
}

const authheadLength = 4 + 8 + 4 + 16 + 4

func (a *authChainA) packAuthData(data []byte) (outData []byte) {
	outData = make([]byte, authheadLength, authheadLength+1500)
	a.data.connectionID++
	if a.data.connectionID > 0xFF000000 {
		rand.Read(a.data.clientID)
		b := make([]byte, 4)
		rand.Read(b)
		a.data.connectionID = binary.LittleEndian.Uint32(b) & 0xFFFFFF
	}
	var key = make([]byte, a.IVLen+a.KeyLen)
	copy(key, a.IV)
	copy(key[a.IVLen:], a.Key)

	encrypt := make([]byte, 20)
	t := time.Now().Unix()
	binary.LittleEndian.PutUint32(encrypt[:4], uint32(t))
	copy(encrypt[4:8], a.data.clientID)
	binary.LittleEndian.PutUint32(encrypt[8:], a.data.connectionID)
	binary.LittleEndian.PutUint16(encrypt[12:], uint16(a.Overhead))
	//binary.LittleEndian.PutUint16(encrypt[14:], 0)

	// first 12 bytes
	{
		rand.Read(outData[:4])
		a.lastClientHash = a.hmac(key, outData[:4])
		copy(outData[4:], a.lastClientHash[:8])
	}
	var base64UserKey string
	// uid & 16 bytes auth data
	{
		uid := make([]byte, 4)
		if a.userKey == nil {
			params := strings.Split(a.ServerInfo.Param, ":")
			if len(params) >= 2 {
				if userID, err := strconv.ParseUint(params[0], 10, 32); err == nil {
					binary.LittleEndian.PutUint32(a.uid[:], uint32(userID))
					a.userKey = a.hashDigest([]byte(params[1]))
				}
			}
			if a.userKey == nil {
				rand.Read(a.uid[:])
				a.userKey = make([]byte, a.KeyLen)
				copy(a.userKey, a.Key)
			}
		}
		for i := 0; i < 4; i++ {
			uid[i] = a.uid[i] ^ a.lastClientHash[8+i]
		}
		base64UserKey = base64.StdEncoding.EncodeToString(a.userKey)
		aesCipherKey := tools.EVPBytesToKey(base64UserKey+a.salt, 16)
		block, err := aes.NewCipher(aesCipherKey)
		if err != nil {
			return
		}
		encryptData := make([]byte, 16)
		iv := make([]byte, aes.BlockSize)
		cbc := stdCipher.NewCBCEncrypter(block, iv)
		cbc.CryptBlocks(encryptData, encrypt[:16])
		copy(encrypt[:4], uid[:])
		copy(encrypt[4:4+16], encryptData)
	}
	// final HMAC
	{
		a.lastServerHash = a.hmac(a.userKey, encrypt[0:20])

		copy(outData[12:], encrypt)
		copy(outData[12+20:], a.lastServerHash[:4])
	}

	// init cipher
	password := make([]byte, len(base64UserKey)+base64.StdEncoding.EncodedLen(16))
	copy(password, base64UserKey)
	base64.StdEncoding.Encode(password[len(base64UserKey):], a.lastClientHash[:16])
	a.cipher, _ = cipher.NewStreamCipher("rc4", string(password))
	_, _ = a.cipher.InitEncrypt()
	_ = a.cipher.InitDecrypt(nil)

	// data
	chunkLength, randLength := a.packedDataLen(data)
	if chunkLength <= 1500 {
		outData = outData[:authheadLength+chunkLength]
	} else {
		newOutData := make([]byte, authheadLength+chunkLength)
		copy(newOutData, outData[:authheadLength])
		outData = newOutData
	}
	a.packData(outData[authheadLength:], data, randLength)
	return
}

func (a *authChainA) PreEncrypt(plainData []byte) (outData []byte, err error) {
	a.buffer.Reset()
	dataLength := len(plainData)
	length := dataLength
	offset := 0
	if length > 0 && !a.hasSentHeader {
		headSize := 1200
		if headSize > dataLength {
			headSize = dataLength
		}
		a.buffer.Write(a.packAuthData(plainData[:headSize]))
		offset += headSize
		dataLength -= headSize
		a.hasSentHeader = true
	}
	var unitSize = a.TcpMss - a.Overhead
	for dataLength > unitSize {
		dataLen, randLength := a.packedDataLen(plainData[offset : offset+unitSize])
		b := make([]byte, dataLen)
		a.packData(b, plainData[offset:offset+unitSize], randLength)
		a.buffer.Write(b)
		dataLength -= unitSize
		offset += unitSize
	}
	if dataLength > 0 {
		dataLen, randLength := a.packedDataLen(plainData[offset:])
		b := make([]byte, dataLen)
		a.packData(b, plainData[offset:], randLength)
		a.buffer.Write(b)
	}
	return a.buffer.Bytes(), nil
}

func (a *authChainA) PostDecrypt(plainData []byte) (outData []byte, n int, err error) {
	a.buffer.Reset()
	key := make([]byte, len(a.userKey)+4)
	readlenth := 0
	copy(key, a.userKey)
	for len(plainData) > 4 {
		binary.LittleEndian.PutUint32(key[len(a.userKey):], a.recvID)
		dataLen := (int)((uint(plainData[1]^a.lastServerHash[15]) << 8) + uint(plainData[0]^a.lastServerHash[14]))
		randLen := a.getServerRandLen(dataLen, a.Overhead)
		length := randLen + dataLen
		if length >= 4096 {
			return nil, 0, ssr.ErrAuthChainDataLengthError
		}
		length += 4
		if length > len(plainData) {
			break
		}

		hash := a.hmac(key, plainData[:length-2])
		if !bytes.Equal(hash[:2], plainData[length-2:length]) {
			return nil, 0, ssr.ErrAuthChainIncorrectHMAC
		}
		var dataPos int
		if dataLen > 0 && randLen > 0 {
			dataPos = 2 + getRandStartPos(&a.randomServer, randLen)
		} else {
			dataPos = 2
		}
		b := make([]byte, dataLen)
		a.cipher.Decrypt(b, plainData[dataPos:dataPos+dataLen])
		a.buffer.Write(b)
		if a.recvID == 1 {
			a.TcpMss = int(binary.LittleEndian.Uint16(a.buffer.Next(2)))
		}
		a.lastServerHash = hash
		a.recvID++
		plainData = plainData[length:]
		readlenth += length

	}
	return a.buffer.Bytes(), readlenth, nil
}

func (a *authChainA) GetOverhead() int {
	return 4
}
