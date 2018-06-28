// Apache License 2.0
// @mdlayher https://github.com/mdlayher/netlink
// Ref: https://github.com/vishvananda/netlink/blob/master/nl/nl_linux.go

package main

import (
	"bytes"
	"encoding/binary"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/nadoo/glider/common/log"
)

// NFNL_SUBSYS_IPSET netfilter netlink message types
// https://github.com/torvalds/linux/blob/9e66317d3c92ddaab330c125dfe9d06eee268aff/include/uapi/linux/netfilter/nfnetlink.h#L56
const NFNL_SUBSYS_IPSET = 6

// http://git.netfilter.org/ipset/tree/include/libipset/linux_ip_set.h
// IPSET_PROTOCOL The protocol version
const IPSET_PROTOCOL = 6

// IPSET_MAXNAMELEN The max length of strings including NUL: set and type identifiers
const IPSET_MAXNAMELEN = 32

// Message types and commands
const (
	IPSET_CMD_CREATE = 2
	IPSET_CMD_FLUSH  = 4
	IPSET_CMD_ADD    = 9
	IPSET_CMD_DEL    = 10
)

// Attributes at command level
const (
	IPSET_ATTR_PROTOCOL = 1 /* 1: Protocol version */
	IPSET_ATTR_SETNAME  = 2 /* 2: Name of the set */
	IPSET_ATTR_TYPENAME = 3 /* 3: Typename */
	IPSET_ATTR_REVISION = 4 /* 4: Settype revision */
	IPSET_ATTR_FAMILY   = 5 /* 5: Settype family */
	IPSET_ATTR_DATA     = 7 /* 7: Nested attributes */
)

// CADT specific attributes
const (
	IPSET_ATTR_IP   = 1
	IPSET_ATTR_CIDR = 3
)

// IP specific attributes
const (
	IPSET_ATTR_IPADDR_IPV4 = 1
	IPSET_ATTR_IPADDR_IPV6 = 2
)

// ATTR flags
const (
	NLA_F_NESTED        = (1 << 15)
	NLA_F_NET_BYTEORDER = (1 << 14)
)

var nextSeqNr uint32
var nativeEndian binary.ByteOrder

// IPSetManager struct
type IPSetManager struct {
	fd  int
	lsa syscall.SockaddrNetlink

	mainSet   string
	domainSet sync.Map
}

// NewIPSetManager returns a IPSetManager
func NewIPSetManager(mainSet string, rules []*RuleConf) (*IPSetManager, error) {
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_RAW, syscall.NETLINK_NETFILTER)
	if err != nil {
		log.F("%s", err)
		return nil, err
	}
	// defer syscall.Close(fd)

	lsa := syscall.SockaddrNetlink{
		Family: syscall.AF_NETLINK,
	}

	if err = syscall.Bind(fd, &lsa); err != nil {
		log.F("%s", err)
		return nil, err
	}

	m := &IPSetManager{fd: fd, lsa: lsa, mainSet: mainSet}
	CreateSet(fd, lsa, mainSet)

	for _, r := range rules {
		set := r.IPSet

		if set != "" && set != m.mainSet {
			CreateSet(fd, lsa, set)
		} else {
			set = m.mainSet
		}

		for _, domain := range r.Domain {
			m.domainSet.Store(domain, set)
		}

		for _, ip := range r.IP {
			AddToSet(fd, lsa, mainSet, ip)
			AddToSet(fd, lsa, r.IPSet, ip)
		}

		for _, cidr := range r.CIDR {
			AddToSet(fd, lsa, mainSet, cidr)
			AddToSet(fd, lsa, r.IPSet, cidr)
		}

	}

	return m, nil
}

// AddDomainIP implements the DNSAnswerHandler function, used to update ipset according to domainSet rule
func (m *IPSetManager) AddDomainIP(domain, ip string) error {
	if ip != "" {
		domainParts := strings.Split(domain, ".")
		length := len(domainParts)
		for i := length - 2; i >= 0; i-- {
			domain := strings.Join(domainParts[i:length], ".")

			// find in domainMap
			if ipset, ok := m.domainSet.Load(domain); ok {
				AddToSet(m.fd, m.lsa, m.mainSet, ip)
				if ipset.(string) != m.mainSet {
					AddToSet(m.fd, m.lsa, ipset.(string), ip)
				}
			}
		}

	}
	return nil
}

func CreateSet(fd int, lsa syscall.SockaddrNetlink, setName string) {
	if setName == "" {
		return
	}

	if len(setName) > IPSET_MAXNAMELEN {
		log.Fatal("ipset: name too long")
	}

	log.F("ipset create %s hash:net", setName)

	req := NewNetlinkRequest(IPSET_CMD_CREATE|(NFNL_SUBSYS_IPSET<<8), syscall.NLM_F_REQUEST)

	// TODO: support AF_INET6
	nfgenMsg := NewNfGenMsg(syscall.AF_INET, 0, 0)
	req.AddData(nfgenMsg)

	attrProto := NewRtAttr(IPSET_ATTR_PROTOCOL, Uint8Attr(IPSET_PROTOCOL))
	req.AddData(attrProto)

	attrSiteName := NewRtAttr(IPSET_ATTR_SETNAME, ZeroTerminated(setName))
	req.AddData(attrSiteName)

	attrSiteType := NewRtAttr(IPSET_ATTR_TYPENAME, ZeroTerminated("hash:net"))
	req.AddData(attrSiteType)

	attrRev := NewRtAttr(IPSET_ATTR_REVISION, Uint8Attr(1))
	req.AddData(attrRev)

	attrFamily := NewRtAttr(IPSET_ATTR_FAMILY, Uint8Attr(2))
	req.AddData(attrFamily)

	attrData := NewRtAttr(IPSET_ATTR_DATA|NLA_F_NESTED, nil)
	req.AddData(attrData)

	err := syscall.Sendto(fd, req.Serialize(), 0, &lsa)
	if err != nil {
		log.F("%s", err)
	}

	FlushSet(fd, lsa, setName)
}

func FlushSet(fd int, lsa syscall.SockaddrNetlink, setName string) {
	log.F("ipset flush %s", setName)

	req := NewNetlinkRequest(IPSET_CMD_FLUSH|(NFNL_SUBSYS_IPSET<<8), syscall.NLM_F_REQUEST)

	// TODO: support AF_INET6
	req.AddData(NewNfGenMsg(syscall.AF_INET, 0, 0))
	req.AddData(NewRtAttr(IPSET_ATTR_PROTOCOL, Uint8Attr(IPSET_PROTOCOL)))
	req.AddData(NewRtAttr(IPSET_ATTR_SETNAME, ZeroTerminated(setName)))

	err := syscall.Sendto(fd, req.Serialize(), 0, &lsa)
	if err != nil {
		log.F("%s", err)
	}

}

func AddToSet(fd int, lsa syscall.SockaddrNetlink, setName, entry string) {
	if setName == "" {
		return
	}

	if len(setName) > IPSET_MAXNAMELEN {
		log.F("ipset: name too long")
	}

	log.F("ipset add %s %s", setName, entry)

	var ip net.IP
	var cidr *net.IPNet

	ip, cidr, err := net.ParseCIDR(entry)
	if err != nil {
		ip = net.ParseIP(entry)
	}

	if ip == nil {
		log.F("ipset: parse %s error", entry)
		return
	}

	req := NewNetlinkRequest(IPSET_CMD_ADD|(NFNL_SUBSYS_IPSET<<8), syscall.NLM_F_REQUEST)

	// TODO: support AF_INET6
	nfgenMsg := NewNfGenMsg(syscall.AF_INET, 0, 0)
	req.AddData(nfgenMsg)

	attrProto := NewRtAttr(IPSET_ATTR_PROTOCOL, Uint8Attr(IPSET_PROTOCOL))
	req.AddData(attrProto)

	attrSiteName := NewRtAttr(IPSET_ATTR_SETNAME, ZeroTerminated(setName))
	req.AddData(attrSiteName)

	attrNested := NewRtAttr(IPSET_ATTR_DATA|NLA_F_NESTED, nil)
	attrIP := NewRtAttrChild(attrNested, IPSET_ATTR_IP|NLA_F_NESTED, nil)

	// TODO: support ipV6
	NewRtAttrChild(attrIP, IPSET_ATTR_IPADDR_IPV4|NLA_F_NET_BYTEORDER, ip.To4())

	// for cidr prefix
	if cidr != nil {
		cidrPrefix, _ := cidr.Mask.Size()
		NewRtAttrChild(attrNested, IPSET_ATTR_CIDR, Uint8Attr(uint8(cidrPrefix)))
	}

	NewRtAttrChild(attrNested, 9|NLA_F_NET_BYTEORDER, Uint32Attr(0))
	req.AddData(attrNested)

	err = syscall.Sendto(fd, req.Serialize(), 0, &lsa)
	if err != nil {
		log.F("%s", err)
	}
}

// Get native endianness for the system
func NativeEndian() binary.ByteOrder {
	if nativeEndian == nil {
		var x uint32 = 0x01020304
		if *(*byte)(unsafe.Pointer(&x)) == 0x01 {
			nativeEndian = binary.BigEndian
		} else {
			nativeEndian = binary.LittleEndian
		}
	}
	return nativeEndian
}

func rtaAlignOf(attrlen int) int {
	return (attrlen + syscall.RTA_ALIGNTO - 1) & ^(syscall.RTA_ALIGNTO - 1)
}

type NetlinkRequestData interface {
	Len() int
	Serialize() []byte
}

type NfGenMsg struct {
	nfgenFamily uint8
	version     uint8
	resID       uint16
}

func NewNfGenMsg(nfgenFamily, version, resID int) *NfGenMsg {
	return &NfGenMsg{
		nfgenFamily: uint8(nfgenFamily),
		version:     uint8(version),
		resID:       uint16(resID),
	}
}

func (m *NfGenMsg) Len() int {
	return rtaAlignOf(4)
}

func (m *NfGenMsg) Serialize() []byte {
	native := NativeEndian()

	length := m.Len()
	buf := make([]byte, rtaAlignOf(length))
	buf[0] = m.nfgenFamily
	buf[1] = m.version
	native.PutUint16(buf[2:4], m.resID)
	return buf
}

// Extend RtAttr to handle data and children
type RtAttr struct {
	syscall.RtAttr
	Data     []byte
	children []NetlinkRequestData
}

// Create a new Extended RtAttr object
func NewRtAttr(attrType int, data []byte) *RtAttr {
	return &RtAttr{
		RtAttr: syscall.RtAttr{
			Type: uint16(attrType),
		},
		children: []NetlinkRequestData{},
		Data:     data,
	}
}

// Create a new RtAttr obj anc add it as a child of an existing object
func NewRtAttrChild(parent *RtAttr, attrType int, data []byte) *RtAttr {
	attr := NewRtAttr(attrType, data)
	parent.children = append(parent.children, attr)
	return attr
}

func (a *RtAttr) Len() int {
	if len(a.children) == 0 {
		return (syscall.SizeofRtAttr + len(a.Data))
	}

	l := 0
	for _, child := range a.children {
		l += rtaAlignOf(child.Len())
	}
	l += syscall.SizeofRtAttr
	return rtaAlignOf(l + len(a.Data))
}

// Serialize the RtAttr into a byte array
// This can't just unsafe.cast because it must iterate through children.
func (a *RtAttr) Serialize() []byte {
	native := NativeEndian()

	length := a.Len()
	buf := make([]byte, rtaAlignOf(length))

	next := 4
	if a.Data != nil {
		copy(buf[next:], a.Data)
		next += rtaAlignOf(len(a.Data))
	}
	if len(a.children) > 0 {
		for _, child := range a.children {
			childBuf := child.Serialize()
			copy(buf[next:], childBuf)
			next += rtaAlignOf(len(childBuf))
		}
	}

	if l := uint16(length); l != 0 {
		native.PutUint16(buf[0:2], l)
	}
	native.PutUint16(buf[2:4], a.Type)
	return buf
}

type NetlinkRequest struct {
	syscall.NlMsghdr
	Data    []NetlinkRequestData
	RawData []byte
}

// Create a new netlink request from proto and flags
// Note the Len value will be inaccurate once data is added until
// the message is serialized
func NewNetlinkRequest(proto, flags int) *NetlinkRequest {
	return &NetlinkRequest{
		NlMsghdr: syscall.NlMsghdr{
			Len:   uint32(syscall.SizeofNlMsghdr),
			Type:  uint16(proto),
			Flags: syscall.NLM_F_REQUEST | uint16(flags),
			Seq:   atomic.AddUint32(&nextSeqNr, 1),
			// Pid:   uint32(os.Getpid()),
		},
	}
}

// Serialize the Netlink Request into a byte array
func (req *NetlinkRequest) Serialize() []byte {
	length := syscall.SizeofNlMsghdr
	dataBytes := make([][]byte, len(req.Data))
	for i, data := range req.Data {
		dataBytes[i] = data.Serialize()
		length = length + len(dataBytes[i])
	}
	length += len(req.RawData)

	req.Len = uint32(length)
	b := make([]byte, length)
	hdr := (*(*[syscall.SizeofNlMsghdr]byte)(unsafe.Pointer(req)))[:]
	next := syscall.SizeofNlMsghdr
	copy(b[0:next], hdr)
	for _, data := range dataBytes {
		for _, dataByte := range data {
			b[next] = dataByte
			next = next + 1
		}
	}
	// Add the raw data if any
	if len(req.RawData) > 0 {
		copy(b[next:length], req.RawData)
	}
	return b
}

func (req *NetlinkRequest) AddData(data NetlinkRequestData) {
	if data != nil {
		req.Data = append(req.Data, data)
	}
}

// AddRawData adds raw bytes to the end of the NetlinkRequest object during serialization
func (req *NetlinkRequest) AddRawData(data []byte) {
	if data != nil {
		req.RawData = append(req.RawData, data...)
	}
}

func Uint8Attr(v uint8) []byte {
	return []byte{byte(v)}
}

func Uint16Attr(v uint16) []byte {
	native := NativeEndian()
	bytes := make([]byte, 2)
	native.PutUint16(bytes, v)
	return bytes
}

func Uint32Attr(v uint32) []byte {
	native := NativeEndian()
	bytes := make([]byte, 4)
	native.PutUint32(bytes, v)
	return bytes
}

func ZeroTerminated(s string) []byte {
	bytes := make([]byte, len(s)+1)
	for i := 0; i < len(s); i++ {
		bytes[i] = s[i]
	}
	bytes[len(s)] = 0
	return bytes
}

func NonZeroTerminated(s string) []byte {
	bytes := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		bytes[i] = s[i]
	}
	return bytes
}

func BytesToString(b []byte) string {
	n := bytes.Index(b, []byte{0})
	return string(b[:n])
}
