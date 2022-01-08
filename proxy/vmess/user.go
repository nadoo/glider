package vmess

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/nadoo/glider/pkg/pool"
)

// User of vmess client.
type User struct {
	UUID   [16]byte
	CmdKey [16]byte
}

// NewUser returns a new user.
func NewUser(uuid [16]byte) *User {
	u := &User{UUID: uuid}
	copy(u.CmdKey[:], GetKey(uuid))
	return u
}

func nextID(oldID [16]byte) (newID [16]byte) {
	md5hash := md5.New()
	md5hash.Write(oldID[:])
	md5hash.Write([]byte("16167dc8-16b6-4e6d-b8bb-65dd68113a81"))
	for {
		md5hash.Sum(newID[:0])
		if !bytes.Equal(oldID[:], newID[:]) {
			return
		}
		md5hash.Write([]byte("533eff8a-4113-4b10-b5ce-0f5d76b98cd2"))
	}
}

// GenAlterIDUsers generates users according to primary user's id and alterID.
func (u *User) GenAlterIDUsers(alterID int) []*User {
	users := make([]*User, alterID)
	preID := u.UUID
	for i := 0; i < alterID; i++ {
		newID := nextID(preID)
		// NOTE: alterID user is a user which have a different uuid but a same cmdkey with the primary user.
		users[i] = &User{UUID: newID, CmdKey: u.CmdKey}
		preID = newID
	}

	return users
}

// StrToUUID converts string to uuid.
func StrToUUID(s string) (uuid [16]byte, err error) {
	if len(s) >= 1 && len(s) <= 30 {
		h := sha1.New()
		h.Write(uuid[:])
		h.Write([]byte(s))
		u := h.Sum(nil)[:16]
		u[6] = (u[6] & 0x0f) | (5 << 4)
		u[8] = (u[8]&(0xff>>2) | (0x02 << 6))
		copy(uuid[:], u)
		return
	}
	b := []byte(strings.Replace(s, "-", "", -1))
	if len(b) != 32 {
		return uuid, errors.New("invalid UUID: " + s)
	}
	_, err = hex.Decode(uuid[:], b)
	return
}

// GetKey returns the key of AES-128-CFB encrypter.
// Key：MD5(UUID + []byte('c48619fe-8f02-49e0-b9e9-edf763e17e21'))
func GetKey(uuid [16]byte) []byte {
	md5hash := md5.New()
	md5hash.Write(uuid[:])
	md5hash.Write([]byte("c48619fe-8f02-49e0-b9e9-edf763e17e21"))
	return md5hash.Sum(nil)
}

// TimestampHash returns the iv of AES-128-CFB encrypter.
// IV：MD5(X + X + X + X)，X = []byte(timestamp.now) (8 bytes, Big Endian)
func TimestampHash(t time.Time) []byte {
	ts := pool.GetBuffer(8)
	defer pool.PutBuffer(ts)

	binary.BigEndian.PutUint64(ts, uint64(t.UTC().Unix()))
	md5hash := md5.New()
	md5hash.Write(ts)
	md5hash.Write(ts)
	md5hash.Write(ts)
	md5hash.Write(ts)
	return md5hash.Sum(nil)
}
