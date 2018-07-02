package vmess

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// User of vmess client
type User struct {
	UUID   [16]byte
	CmdKey [16]byte
}

// NewUser .
func NewUser(uuidStr string) (*User, error) {
	uuid, err := StrToUUID(uuidStr)
	if err != nil {
		return nil, err
	}

	u := &User{UUID: uuid}
	copy(u.CmdKey[:], GetKey(uuid))

	return u, nil
}

// StrToUUID converts string to uuid.
// s fomat: "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
func StrToUUID(s string) (uuid [16]byte, err error) {
	b := []byte(strings.Replace(s, "-", "", -1))
	if len(b) != 32 {
		return uuid, errors.New("invalid UUID: " + s)
	}
	_, err = hex.Decode(uuid[:], b)
	return
}

// GetKey returns the key of AES-128-CFB encrypter
// Key：MD5(UUID + []byte('c48619fe-8f02-49e0-b9e9-edf763e17e21'))
func GetKey(uuid [16]byte) []byte {
	md5hash := md5.New()
	md5hash.Write(uuid[:])
	md5hash.Write([]byte("c48619fe-8f02-49e0-b9e9-edf763e17e21"))
	return md5hash.Sum(nil)
}

// TimestampHash returns the iv of AES-128-CFB encrypter
// IV：MD5(X + X + X + X)，X = []byte(timestamp.now) (8 bytes, Big Endian)
func TimestampHash(t time.Time) []byte {
	md5hash := md5.New()

	ts := make([]byte, 8)
	binary.BigEndian.PutUint64(ts, uint64(t.UTC().Unix()))
	md5hash.Write(ts)
	md5hash.Write(ts)
	md5hash.Write(ts)
	md5hash.Write(ts)
	return md5hash.Sum(nil)
}
