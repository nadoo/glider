package ssr

import "errors"

const ObfsHMACSHA1Len = 10

var (
	ErrAuthSHA1v4CRC32Error                = errors.New("auth_sha1_v4 post decrypt data crc32 error")
	ErrAuthSHA1v4DataLengthError           = errors.New("auth_sha1_v4 post decrypt data length error")
	ErrAuthSHA1v4IncorrectChecksum         = errors.New("auth_sha1_v4 post decrypt incorrect checksum")
	ErrAuthAES128HMACError                 = errors.New("auth_aes128_* post decrypt hmac error")
	ErrAuthAES128DataLengthError           = errors.New("auth_aes128_* post decrypt length mismatch")
	ErrAuthAES128IncorrectChecksum         = errors.New("auth_aes128_* post decrypt incorrect checksum")
	ErrTLS12TicketAuthTooShortData         = errors.New("tls1.2_ticket_auth too short data")
	ErrTLS12TicketAuthHMACError            = errors.New("tls1.2_ticket_auth hmac verifying failed")
	ErrTLS12TicketAuthIncorrectMagicNumber = errors.New("tls1.2_ticket_auth incorrect magic number")
)

type ServerInfoForObfs struct {
	Host      string
	Port      uint16
	Param     string
	IV        []byte
	IVLen     int
	RecvIV    []byte
	RecvIVLen int
	Key       []byte
	KeyLen    int
	HeadLen   int
	TcpMss    int
}

func GetHeadSize(data []byte, defaultValue int) int {
	if data == nil || len(data) < 2 {
		return defaultValue
	}
	headType := data[0] & 0x07
	switch headType {
	case 1:
		// IPv4 1+4+2
		return 7
	case 4:
		// IPv6 1+16+2
		return 19
	case 3:
		// domain name, variant length
		return 4 + int(data[1])
	}

	return defaultValue
}

func (s *ServerInfoForObfs) SetHeadLen(data []byte, defaultValue int) {
	s.HeadLen = GetHeadSize(data, defaultValue)
}
