package dns

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math/rand"
	"net/netip"
	"strings"
)

// UDPMaxLen is the max size of udp dns request.
// https://www.rfc-editor.org/rfc/rfc1035#section-4.2.1
// Messages carried by UDP are restricted to 512 bytes (not counting the IP
// or UDP headers).  Longer messages are truncated and the TC bit is set in
// the header.
const UDPMaxLen = 512

// HeaderLen is the length of dns msg header.
const HeaderLen = 12

// MsgType is the dns Message type.
type MsgType byte

// Message types.
const (
	QueryMsg    MsgType = 0
	ResponseMsg MsgType = 1
)

// Query types.
const (
	QTypeA    uint16 = 1  //ipv4
	QTypeAAAA uint16 = 28 ///ipv6
)

// ClassINET .
const ClassINET uint16 = 1

// Message format:
// https://www.rfc-editor.org/rfc/rfc1035#section-4.1
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
func NewMessage(id uint16, msgType MsgType) *Message {
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
	buf := &bytes.Buffer{}
	if _, err := m.MarshalTo(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// MarshalTo marshals message struct to []byte and write to w.
func (m *Message) MarshalTo(w io.Writer) (n int, err error) {
	m.Header.SetQdcount(1)
	m.Header.SetAncount(len(m.Answers))

	nn := 0
	nn, err = m.Header.MarshalTo(w)
	if err != nil {
		return
	}
	n += nn

	nn, err = m.Question.MarshalTo(w)
	if err != nil {
		return
	}
	n += nn

	for _, answer := range m.Answers {
		nn, err = answer.MarshalTo(w)
		if err != nil {
			return
		}
		n += nn
	}

	return
}

// UnmarshalMessage unmarshals []bytes to Message.
func UnmarshalMessage(b []byte) (*Message, error) {
	if len(b) < HeaderLen {
		return nil, errors.New("UnmarshalMessage: not enough data")
	}

	m := &Message{unMarshaled: b}
	if err := UnmarshalHeader(b[:HeaderLen], &m.Header); err != nil {
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

// Header format:
// https://www.rfc-editor.org/rfc/rfc1035#section-4.1.1
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
func (h *Header) SetMsgType(qr MsgType) {
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

// MarshalTo marshals header struct to []byte and write to w.
func (h *Header) MarshalTo(w io.Writer) (int, error) {
	return HeaderLen, binary.Write(w, binary.BigEndian, h)
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

// Question format:
// https://www.rfc-editor.org/rfc/rfc1035#section-4.1.2
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

// MarshalTo marshals Question struct to []byte and write to w.
func (q *Question) MarshalTo(w io.Writer) (n int, err error) {
	n, err = MarshalDomainTo(w, q.QNAME)
	if err != nil {
		return
	}

	if err = binary.Write(w, binary.BigEndian, q.QTYPE); err != nil {
		return
	}
	n += 2

	if err = binary.Write(w, binary.BigEndian, q.QCLASS); err != nil {
		return
	}
	n += 2

	return
}

// UnmarshalQuestion unmarshals []bytes to Question.
func (m *Message) UnmarshalQuestion(b []byte, q *Question) (n int, err error) {
	if q == nil {
		return 0, errors.New("unmarshal question must not be nil")
	}

	if len(b) <= 5 {
		return 0, errors.New("UnmarshalQuestion: not enough data")
	}

	sb := new(strings.Builder)
	sb.Grow(32)
	idx, err := m.UnmarshalDomainTo(sb, b)
	if err != nil {
		return 0, err
	}

	q.QNAME = sb.String()
	q.QTYPE = binary.BigEndian.Uint16(b[idx : idx+2])
	q.QCLASS = binary.BigEndian.Uint16(b[idx+2 : idx+4])

	return idx + 3 + 1, nil
}

// RR format:
// https://www.rfc-editor.org/rfc/rfc1035#section-3.2.1
// https://www.rfc-editor.org/rfc/rfc1035#section-4.1.3
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

	IP netip.Addr
}

// NewRR returns a new dns rr.
func NewRR() *RR {
	return &RR{}
}

// MarshalTo marshals RR struct to []byte and write to w.
func (rr *RR) MarshalTo(w io.Writer) (n int, err error) {
	n, err = MarshalDomainTo(w, rr.NAME)
	if err != nil {
		return
	}

	if err = binary.Write(w, binary.BigEndian, rr.TYPE); err != nil {
		return
	}
	n += 2

	if err = binary.Write(w, binary.BigEndian, rr.CLASS); err != nil {
		return
	}
	n += 2

	if err = binary.Write(w, binary.BigEndian, rr.TTL); err != nil {
		return
	}
	n += 4

	err = binary.Write(w, binary.BigEndian, rr.RDLENGTH)
	if err != nil {
		return
	}
	n += 2

	if _, err = w.Write(rr.RDATA); err != nil {
		return
	}
	n += len(rr.RDATA)

	return
}

// UnmarshalRR unmarshals []bytes to RR.
func (m *Message) UnmarshalRR(start int, rr *RR) (n int, err error) {
	if rr == nil {
		return 0, errors.New("unmarshal rr must not be nil")
	}

	p := m.unMarshaled[start:]

	sb := new(strings.Builder)
	sb.Grow(32)

	n, err = m.UnmarshalDomainTo(sb, p)
	if err != nil {
		return 0, err
	}
	rr.NAME = sb.String()

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
		rr.IP = netip.AddrFrom4(*(*[4]byte)(rr.RDATA[:4]))
	} else if rr.TYPE == QTypeAAAA {
		rr.IP = netip.AddrFrom16(*(*[16]byte)(rr.RDATA[:16]))
	}

	n = n + 10 + int(rr.RDLENGTH)

	return n, nil
}

// MarshalDomainTo marshals domain string struct to []byte and write to w.
func MarshalDomainTo(w io.Writer, domain string) (n int, err error) {
	nn := 0
	for _, seg := range strings.Split(domain, ".") {
		nn, err = w.Write([]byte{byte(len(seg))})
		if err != nil {
			return
		}
		n += nn

		nn, err = io.WriteString(w, seg)
		if err != nil {
			return
		}
		n += nn
	}

	nn, err = w.Write([]byte{0x00})
	if err != nil {
		return
	}
	n += nn

	return
}

// UnmarshalDomainTo gets domain from bytes to string builder.
func (m *Message) UnmarshalDomainTo(sb *strings.Builder, b []byte) (int, error) {
	var idx, size int

	for len(b[idx:]) != 0 {
		// https://www.rfc-editor.org/rfc/rfc1035#section-4.1.4
		// "Message compression",
		// +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
		// | 1  1|                OFFSET                   |
		// +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
		if b[idx]&0xC0 == 0xC0 {
			if len(b[idx:]) < 2 {
				return 0, errors.New("UnmarshalDomainTo: not enough size for compressed domain")
			}

			offset := binary.BigEndian.Uint16(b[idx : idx+2])
			if err := m.UnmarshalDomainPointTo(sb, int(offset&0x3FFF)); err != nil {
				return 0, err
			}

			idx += 2
			break
		}

		size = int(b[idx])
		idx++

		// root domain name
		if size == 0 {
			break
		}

		if size > 63 {
			return 0, errors.New("UnmarshalDomainTo: label size larger than 63")
		}

		if idx+size > len(b) {
			return 0, errors.New("UnmarshalDomainTo: label size larger than msg length")
		}

		if sb.Len() > 0 {
			sb.WriteByte('.')
		}
		sb.Write(b[idx : idx+size])

		idx += size
	}

	return idx, nil
}

// UnmarshalDomainPointTo gets domain from offset point to string builder.
func (m *Message) UnmarshalDomainPointTo(sb *strings.Builder, offset int) error {
	if offset > len(m.unMarshaled) {
		return errors.New("UnmarshalDomainPointTo: offset larger than msg length")
	}
	_, err := m.UnmarshalDomainTo(sb, m.unMarshaled[offset:])
	return err
}
