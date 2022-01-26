package obfs

import (
	"bytes"
	"crypto/hmac"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/nadoo/glider/proxy/ssr/internal/ssr"
	"github.com/nadoo/glider/proxy/ssr/internal/tools"
)

func init() {
	register("tls1.2_ticket_auth", newTLS12TicketAuth)
	register("tls1.2_ticket_fastauth", newTLS12TicketFastAuth)
}

type tlsAuthData struct {
	localClientID [32]byte
}

// tls12TicketAuth tls1.2_ticket_auth obfs encapsulate
type tls12TicketAuth struct {
	ssr.ServerInfo
	data            *tlsAuthData
	handshakeStatus int
	sendSaver       []byte
	recvBuffer      bytes.Buffer
	fastAuth        bool
}

// newTLS12TicketAuth create a tlv1.2_ticket_auth object
func newTLS12TicketAuth() IObfs {
	return &tls12TicketAuth{}
}

// newTLS12TicketFastAuth create a tlv1.2_ticket_fastauth object
func newTLS12TicketFastAuth() IObfs {
	return &tls12TicketAuth{
		fastAuth: true,
	}
}

func (t *tls12TicketAuth) SetServerInfo(s *ssr.ServerInfo) {
	t.ServerInfo = *s
}

func (t *tls12TicketAuth) GetServerInfo() (s *ssr.ServerInfo) {
	return &t.ServerInfo
}

func (t *tls12TicketAuth) SetData(data any) {
	if auth, ok := data.(*tlsAuthData); ok {
		t.data = auth
	}
}

func (t *tls12TicketAuth) GetData() any {
	if t.data == nil {
		t.data = &tlsAuthData{}
		b := make([]byte, 32)

		rand.Read(b)
		copy(t.data.localClientID[:], b)
	}
	return t.data
}

func (t *tls12TicketAuth) getHost() string {
	host := t.Host
	if len(t.Param) > 0 {
		hosts := strings.Split(t.Param, ",")
		if len(hosts) > 0 {

			host = hosts[rand.Intn(len(hosts))]
			host = strings.TrimSpace(host)
		}
	}
	if len(host) > 0 && host[len(host)-1] >= byte('0') && host[len(host)-1] <= byte('9') && len(t.Param) == 0 {
		host = ""
	}
	return host
}

func packData(prefixData []byte, suffixData []byte) (outData []byte) {
	d := []byte{0x17, 0x3, 0x3, 0, 0}
	binary.BigEndian.PutUint16(d[3:5], uint16(len(suffixData)&0xFFFF))
	outData = append(prefixData, d...)
	outData = append(outData, suffixData...)
	return
}

func (t *tls12TicketAuth) Encode(data []byte) (encodedData []byte, err error) {
	encodedData = make([]byte, 0)
	rand.Seed(time.Now().UnixNano())
	switch t.handshakeStatus {
	case 8:
		if len(data) < 1024 {
			d := []byte{0x17, 0x3, 0x3, 0, 0}
			binary.BigEndian.PutUint16(d[3:5], uint16(len(data)&0xFFFF))
			encodedData = append(d, data...)
			return
		} else {
			start := 0
			var l int
			for len(data)-start > 2048 {
				l = rand.Intn(4096) + 100
				if l > len(data)-start {
					l = len(data) - start
				}
				encodedData = packData(encodedData, data[start:start+l])
				start += l
			}
			if len(data)-start > 0 {
				l = len(data) - start
				encodedData = packData(encodedData, data[start:start+l])
			}
			return
		}
	case 1:
		if len(data) > 0 {
			if len(data) < 1024 {
				t.sendSaver = packData(t.sendSaver, data)
			} else {
				start := 0
				var l int
				for len(data)-start > 2048 {
					l = rand.Intn(4096) + 100
					if l > len(data)-start {
						l = len(data) - start
					}
					encodedData = packData(encodedData, data[start:start+l])
					start += l
				}
				if len(data)-start > 0 {
					l = len(data) - start
					encodedData = packData(encodedData, data[start:start+l])
				}
				t.sendSaver = append(t.sendSaver, encodedData...)
				encodedData = encodedData[:0]
			}
			return []byte{}, nil
		}
		hmacData := make([]byte, 43)
		handshakeFinish := []byte("\x14\x03\x03\x00\x01\x01\x16\x03\x03\x00\x20")
		copy(hmacData, handshakeFinish)
		rand.Read(hmacData[11:33])
		h := t.hmacSHA1(hmacData[:33])
		copy(hmacData[33:], h)
		encodedData = append(hmacData, t.sendSaver...)
		t.sendSaver = t.sendSaver[:0]
		t.handshakeStatus = 8
	case 0:
		tlsData0 := []byte("\x00\x1c\xc0\x2b\xc0\x2f\xcc\xa9\xcc\xa8\xcc\x14\xcc\x13\xc0\x0a\xc0\x14\xc0\x09\xc0\x13\x00\x9c\x00\x35\x00\x2f\x00\x0a\x01\x00")
		tlsData1 := []byte("\xff\x01\x00\x01\x00")
		tlsData2 := []byte("\x00\x17\x00\x00\x00\x23\x00\xd0")
		tlsData3 := []byte("\x00\x0d\x00\x16\x00\x14\x06\x01\x06\x03\x05\x01\x05\x03\x04\x01\x04\x03\x03\x01\x03\x03\x02\x01\x02\x03\x00\x05\x00\x05\x01\x00\x00\x00\x00\x00\x12\x00\x00\x75\x50\x00\x00\x00\x0b\x00\x02\x01\x00\x00\x0a\x00\x06\x00\x04\x00\x17\x00\x18\x00\x15\x00\x66\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")

		var tlsData [2048]byte
		tlsDataLen := 0
		copy(tlsData[0:], tlsData1)
		tlsDataLen += len(tlsData1)
		sni := t.sni(t.getHost())
		copy(tlsData[tlsDataLen:], sni)
		tlsDataLen += len(sni)
		copy(tlsData[tlsDataLen:], tlsData2)
		tlsDataLen += len(tlsData2)
		ticketLen := rand.Intn(164)*2 + 64
		tlsData[tlsDataLen-1] = uint8(ticketLen & 0xff)
		tlsData[tlsDataLen-2] = uint8(ticketLen >> 8)
		//ticketLen := 208
		rand.Read(tlsData[tlsDataLen : tlsDataLen+ticketLen])
		tlsDataLen += ticketLen
		copy(tlsData[tlsDataLen:], tlsData3)
		tlsDataLen += len(tlsData3)

		length := 11 + 32 + 1 + 32 + len(tlsData0) + 2 + tlsDataLen
		encodedData = make([]byte, length)
		pdata := length - tlsDataLen
		l := tlsDataLen
		copy(encodedData[pdata:], tlsData[:tlsDataLen])
		encodedData[pdata-1] = uint8(tlsDataLen)
		encodedData[pdata-2] = uint8(tlsDataLen >> 8)
		pdata -= 2
		l += 2
		copy(encodedData[pdata-len(tlsData0):], tlsData0)
		pdata -= len(tlsData0)
		l += len(tlsData0)
		copy(encodedData[pdata-32:], t.data.localClientID[:])
		pdata -= 32
		l += 32
		encodedData[pdata-1] = 0x20
		pdata -= 1
		l += 1
		copy(encodedData[pdata-32:], t.packAuthData())
		pdata -= 32
		l += 32
		encodedData[pdata-1] = 0x3
		encodedData[pdata-2] = 0x3 // tls version
		pdata -= 2
		l += 2
		encodedData[pdata-1] = uint8(l)
		encodedData[pdata-2] = uint8(l >> 8)
		encodedData[pdata-3] = 0
		encodedData[pdata-4] = 1
		pdata -= 4
		l += 4
		encodedData[pdata-1] = uint8(l)
		encodedData[pdata-2] = uint8(l >> 8)
		pdata -= 2
		// l += 2
		encodedData[pdata-1] = 0x1
		encodedData[pdata-2] = 0x3 // tls version
		pdata -= 2
		// l += 2
		encodedData[pdata-1] = 0x16 // tls handshake
		// pdata -= 1
		// l += 1

		t.sendSaver = packData(t.sendSaver, data)
		t.handshakeStatus = 1
	default:
		//log.Println(fmt.Errorf("unexpected handshake status: %d", t.handshakeStatus))
		return nil, fmt.Errorf("unexpected handshake status: %d", t.handshakeStatus)
	}
	return
}

func (t *tls12TicketAuth) Decode(data []byte) (decodedData []byte, needSendBack bool, err error) {
	if t.handshakeStatus == -1 {
		return data, false, nil
	}

	if t.handshakeStatus == 8 {
		t.recvBuffer.Write(data)
		for t.recvBuffer.Len() > 5 {
			var h [5]byte
			_, _ = t.recvBuffer.Read(h[:])
			if !bytes.Equal(h[0:3], []byte{0x17, 0x3, 0x3}) {
				log.Println("incorrect magic number", h[0:3], ", 0x170303 is expected")
				return nil, false, ssr.ErrTLS12TicketAuthIncorrectMagicNumber
			}
			size := int(binary.BigEndian.Uint16(h[3:5]))
			if t.recvBuffer.Len() < size {
				unread := t.recvBuffer.Bytes()
				t.recvBuffer.Reset()
				t.recvBuffer.Write(h[:])
				t.recvBuffer.Write(unread)
				break
			}
			d := make([]byte, size)
			_, _ = t.recvBuffer.Read(d)
			decodedData = append(decodedData, d...)
		}
		return decodedData, false, nil
	}

	if len(data) < 11+32+1+32 {
		return nil, false, ssr.ErrTLS12TicketAuthTooShortData
	}

	hash := t.hmacSHA1(data[11 : 11+22])

	if !hmac.Equal(data[33:33+ssr.ObfsHMACSHA1Len], hash) {
		return nil, false, ssr.ErrTLS12TicketAuthHMACError
	}
	return nil, true, nil
}

func (t *tls12TicketAuth) packAuthData() (outData []byte) {
	outSize := 32
	outData = make([]byte, outSize)

	now := time.Now().Unix()
	binary.BigEndian.PutUint32(outData[0:4], uint32(now))

	rand.Read(outData[4 : 4+18])

	hash := t.hmacSHA1(outData[:outSize-ssr.ObfsHMACSHA1Len])
	copy(outData[outSize-ssr.ObfsHMACSHA1Len:], hash)

	return
}

func (t *tls12TicketAuth) hmacSHA1(data []byte) []byte {
	key := make([]byte, t.KeyLen+32)
	copy(key, t.Key)
	copy(key[t.KeyLen:], t.data.localClientID[:])

	sha1Data := tools.HmacSHA1(key, data)
	return sha1Data[:ssr.ObfsHMACSHA1Len]
}

func (t *tls12TicketAuth) sni(u string) []byte {
	bURL := []byte(u)
	length := len(bURL)
	ret := make([]byte, length+9)
	copy(ret[9:9+length], bURL)
	binary.BigEndian.PutUint16(ret[7:], uint16(length&0xFFFF))
	length += 3
	binary.BigEndian.PutUint16(ret[4:], uint16(length&0xFFFF))
	length += 2
	binary.BigEndian.PutUint16(ret[2:], uint16(length&0xFFFF))
	return ret
}

func (t *tls12TicketAuth) GetOverhead() int {
	return 5
}
