package dns

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math/rand"
	"net"
	"strings"
)

// UDPMaxLen is the max size of udp dns request.
// https://tools.ietf.org/html/rfc1035#section-4.2.1
// Messages carried by UDP are restricted to 512 bytes (not counting the IP
// or UDP headers).  Longer messages are truncated and the TC bit is set in
// the header.
const UDPMaxLen = 512

// HeaderLen is the length of dns msg header.
const HeaderLen = 12

// Message types
const (
	Query    = 0
	Response = 1
)

// Query types
const (
	QTypeA    uint16 = 1  //ipv4
	QTypeAAAA uint16 = 28 ///ipv6
)

// ClassINET .
const ClassINET uint16 = 1

// Message format
// https://tools.ietf.org/html/rfc1035#section-4.1
// All communications inside of the domain protocol are carried in a single
// format called a message.  The top level format of message is divided
// into 5 sections (some of which are empty in certain cases) shown below:
//
//     +---------------------+
//     |        Header       |
//     +---------------------+
//     |       Question      | the question for the name server
//     +---------------------+
//     |        Answer       | RRs answering the question
//     +---------------------+
//     |      Authority      | RRs pointing toward an authority
//     +---------------------+
//     |      Additional     | RRs holding additional information
type Message struct {
	Header
	// most dns implementation only support 1 question
	Question   *Question
	Answers    []*RR
	Authority  []*RR
	Additional []*RR

	// used in UnmarshalMessage
	unMarshaled []byte
}

// NewMessage returns a new message.
func NewMessage(id uint16, msgType int) *Message {
	if id == 0 {
		id = uint16(rand.Uint32())
	}

	m := &Message{Header: Header{ID: id}}
	m.SetMsgType(msgType)

	return m
}

// SetQuestion sets a question to dns message.
func (m *Message) SetQuestion(q *Question) error {
	m.Question = q
	m.Header.SetQdcount(1)
	return nil
}

// AddAnswer adds an answer to dns message.
func (m *Message) AddAnswer(rr *RR) error {
	m.Answers = append(m.Answers, rr)
	return nil
}

// Marshal marshals message struct to []byte.
func (m *Message) Marshal() ([]byte, error) {
	var buf bytes.Buffer

	m.Header.SetQdcount(1)
	m.Header.SetAncount(len(m.Answers))

	b, err := m.Header.Marshal()
	if err != nil {
		return nil, err
	}
	buf.Write(b)

	b, err = m.Question.Marshal()
	if err != nil {
		return nil, err
	}
	buf.Write(b)

	for _, answer := range m.Answers {
		b, err := answer.Marshal()
		if err != nil {
			return nil, err
		}
		buf.Write(b)
	}

	return buf.Bytes(), nil
}

// UnmarshalMessage unmarshals []bytes to Message.
func UnmarshalMessage(b []byte) (*Message, error) {
	if len(b) < HeaderLen {
		return nil, errors.New("UnmarshalMessage: not enough data")
	}

	m := &Message{unMarshaled: b}
	err := UnmarshalHeader(b[:HeaderLen], &m.Header)
	if err != nil {
		return nil, err
	}

	q := &Question{}
	qLen, err := m.UnmarshalQuestion(b[HeaderLen:], q)
	if err != nil {
		return nil, err
	}
	m.SetQuestion(q)

	// resp answers
	rrIdx := HeaderLen + qLen
	for i := 0; i < int(m.Header.ANCOUNT); i++ {
		rr := &RR{}
		rrLen, err := m.UnmarshalRR(rrIdx, rr)
		if err != nil {
			return nil, err
		}
		m.AddAnswer(rr)

		rrIdx += rrLen
	}

	m.Header.SetAncount(len(m.Answers))

	return m, nil
}

// Header format
// https://tools.ietf.org/html/rfc1035#section-4.1.1
// The header contains the following fields:
//
//                                     1  1  1  1  1  1
//       0  1  2  3  4  5  6  7  8  9  0  1  2  3  4  5
//     +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//     |                      ID                       |
//     +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//     |QR|   Opcode  |AA|TC|RD|RA|   Z    |   RCODE   |
//     +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//     |                    QDCOUNT                    |
//     +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//     |                    ANCOUNT                    |
//     +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//     |                    NSCOUNT                    |
//     +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//     |                    ARCOUNT                    |
//     +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//
type Header struct {
	ID      uint16
	Bits    uint16
	QDCOUNT uint16
	ANCOUNT uint16
	NSCOUNT uint16
	ARCOUNT uint16
}

// SetMsgType sets the message type.
func (h *Header) SetMsgType(qr int) {
	h.Bits |= uint16(qr) << 15
}

// SetTC sets the tc flag.
func (h *Header) SetTC(tc int) {
	h.Bits |= uint16(tc) << 9
}

// SetQdcount sets query count, most dns servers only support 1 query per request.
func (h *Header) SetQdcount(qdcount int) {
	h.QDCOUNT = uint16(qdcount)
}

// SetAncount sets answers count.
func (h *Header) SetAncount(ancount int) {
	h.ANCOUNT = uint16(ancount)
}

func (h *Header) setFlag(QR uint16, Opcode uint16, AA uint16,
	TC uint16, RD uint16, RA uint16, RCODE uint16) {
	h.Bits = QR<<15 + Opcode<<11 + AA<<10 + TC<<9 + RD<<8 + RA<<7 + RCODE
}

// Marshal marshals header struct to []byte.
func (h *Header) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	err := binary.Write(&buf, binary.BigEndian, h)
	return buf.Bytes(), err
}

// UnmarshalHeader unmarshals []bytes to Header.
func UnmarshalHeader(b []byte, h *Header) error {
	if h == nil {
		return errors.New("unmarshal header must not be nil")
	}

	if len(b) != HeaderLen {
		return errors.New("unmarshal header bytes has an unexpected size")
	}

	h.ID = binary.BigEndian.Uint16(b[:2])
	h.Bits = binary.BigEndian.Uint16(b[2:4])
	h.QDCOUNT = binary.BigEndian.Uint16(b[4:6])
	h.ANCOUNT = binary.BigEndian.Uint16(b[6:8])
	h.NSCOUNT = binary.BigEndian.Uint16(b[8:10])
	h.ARCOUNT = binary.BigEndian.Uint16(b[10:])

	return nil
}

// Question format
// https://tools.ietf.org/html/rfc1035#section-4.1.2
// The question section is used to carry the "question" in most queries,
// i.e., the parameters that define what is being asked.  The section
// contains QDCOUNT (usually 1) entries, each of the following format:
//
//                                     1  1  1  1  1  1
//       0  1  2  3  4  5  6  7  8  9  0  1  2  3  4  5
//     +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//     |                                               |
//     /                     QNAME                     /
//     /                                               /
//     +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//     |                     QTYPE                     |
//     +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//     |                     QCLASS                    |
//     +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
type Question struct {
	QNAME  string
	QTYPE  uint16
	QCLASS uint16
}

// NewQuestion returns a new dns question.
func NewQuestion(qtype uint16, domain string) *Question {
	return &Question{
		QNAME:  domain,
		QTYPE:  qtype,
		QCLASS: ClassINET,
	}
}

// Marshal marshals Question struct to []byte.
func (q *Question) Marshal() ([]byte, error) {
	var buf bytes.Buffer

	buf.Write(MarshalDomain(q.QNAME))
	binary.Write(&buf, binary.BigEndian, q.QTYPE)
	binary.Write(&buf, binary.BigEndian, q.QCLASS)

	return buf.Bytes(), nil
}

// UnmarshalQuestion unmarshals []bytes to Question.
func (m *Message) UnmarshalQuestion(b []byte, q *Question) (n int, err error) {
	if q == nil {
		return 0, errors.New("unmarshal question must not be nil")
	}

	if len(b) <= 5 {
		return 0, errors.New("UnmarshalQuestion: not enough data")
	}

	domain, idx, err := m.UnmarshalDomain(b)
	if err != nil {
		return 0, err
	}

	q.QNAME = domain
	q.QTYPE = binary.BigEndian.Uint16(b[idx : idx+2])
	q.QCLASS = binary.BigEndian.Uint16(b[idx+2 : idx+4])

	return idx + 3 + 1, nil
}

// RR format
// https://tools.ietf.org/html/rfc1035#section-3.2.1
// https://tools.ietf.org/html/rfc1035#section-4.1.3
// The answer, authority, and additional sections all share the same
// format: a variable number of resource records, where the number of
// records is specified in the corresponding count field in the header.
// Each resource record has the following format:
//
//                                     1  1  1  1  1  1
//       0  1  2  3  4  5  6  7  8  9  0  1  2  3  4  5
//     +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//     |                                               |
//     /                                               /
//     /                      NAME                     /
//     |                                               |
//     +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//     |                      TYPE                     |
//     +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//     |                     CLASS                     |
//     +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//     |                      TTL                      |
//     |                                               |
//     +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//     |                   RDLENGTH                    |
//     +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--|
//     /                     RDATA                     /
//     /                                               /
//     +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
type RR struct {
	NAME     string
	TYPE     uint16
	CLASS    uint16
	TTL      uint32
	RDLENGTH uint16
	RDATA    []byte

	IP string
}

// NewRR returns a new dns rr.
func NewRR() *RR {
	rr := &RR{}
	return rr
}

// Marshal marshals RR struct to []byte.
func (rr *RR) Marshal() ([]byte, error) {
	var buf bytes.Buffer

	buf.Write(MarshalDomain(rr.NAME))
	binary.Write(&buf, binary.BigEndian, rr.TYPE)
	binary.Write(&buf, binary.BigEndian, rr.CLASS)
	binary.Write(&buf, binary.BigEndian, rr.TTL)
	binary.Write(&buf, binary.BigEndian, rr.RDLENGTH)
	buf.Write(rr.RDATA)

	return buf.Bytes(), nil
}

// UnmarshalRR unmarshals []bytes to RR.
func (m *Message) UnmarshalRR(start int, rr *RR) (n int, err error) {
	if rr == nil {
		return 0, errors.New("unmarshal rr must not be nil")
	}

	p := m.unMarshaled[start:]

	domain, n, err := m.UnmarshalDomain(p)
	if err != nil {
		return 0, err
	}
	rr.NAME = domain

	if len(p) <= n+10 {
		return 0, errors.New("UnmarshalRR: not enough data")
	}

	rr.TYPE = binary.BigEndian.Uint16(p[n:])
	rr.CLASS = binary.BigEndian.Uint16(p[n+2:])
	rr.TTL = binary.BigEndian.Uint32(p[n+4:])
	rr.RDLENGTH = binary.BigEndian.Uint16(p[n+8:])

	if len(p) < n+10+int(rr.RDLENGTH) {
		return 0, errors.New("UnmarshalRR: not enough data for RDATA")
	}

	rr.RDATA = p[n+10 : n+10+int(rr.RDLENGTH)]

	if rr.TYPE == QTypeA {
		rr.IP = net.IP(rr.RDATA[:net.IPv4len]).String()
	} else if rr.TYPE == QTypeAAAA {
		rr.IP = net.IP(rr.RDATA[:net.IPv6len]).String()
	}

	n = n + 10 + int(rr.RDLENGTH)

	return n, nil
}

// MarshalDomain marshals domain string struct to []byte.
func MarshalDomain(domain string) []byte {
	var buf bytes.Buffer

	for _, seg := range strings.Split(domain, ".") {
		binary.Write(&buf, binary.BigEndian, byte(len(seg)))
		binary.Write(&buf, binary.BigEndian, []byte(seg))
	}
	binary.Write(&buf, binary.BigEndian, byte(0x00))

	return buf.Bytes()
}

// UnmarshalDomain gets domain from bytes.
func (m *Message) UnmarshalDomain(b []byte) (string, int, error) {
	var idx, size int
	var labels = []string{}

	for {
		// https://tools.ietf.org/html/rfc1035#section-4.1.4
		// "Message compression",
		// +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
		// | 1  1|                OFFSET                   |
		// +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
		if b[idx]&0xC0 == 0xC0 {
			offset := binary.BigEndian.Uint16(b[idx : idx+2])
			label, err := m.UnmarshalDomainPoint(int(offset & 0x3FFF))
			if err != nil {
				return "", 0, err
			}

			labels = append(labels, label)
			idx += 2
			break
		} else {
			size = int(b[idx])
			if size == 0 {
				idx++
				break
			}

			if size > 63 {
				return "", 0, errors.New("UnmarshalDomain: label size larger than 63")
			}

			if idx+size+1 > len(b) {
				return "", 0, errors.New("UnmarshalDomain: label size larger than msg length")
			}

			labels = append(labels, string(b[idx+1:idx+size+1]))
			idx += (size + 1)
		}
	}

	domain := strings.Join(labels, ".")
	return domain, idx, nil
}

// UnmarshalDomainPoint gets domain from offset point.
func (m *Message) UnmarshalDomainPoint(offset int) (string, error) {
	if offset > len(m.unMarshaled) {
		return "", errors.New("UnmarshalDomainPoint: offset larger than msg length")
	}
	domain, _, err := m.UnmarshalDomain(m.unMarshaled[offset:])
	return domain, err
}
