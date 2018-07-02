package vmess

import (
	"encoding/hex"
	"errors"
	"strings"
)

// User of vmess client
type User struct {
	UUID [16]byte
}

// NewUser .
func NewUser(uuidStr string) (*User, error) {
	uuid, err := StrToUUID(uuidStr)
	if err != nil {
		return nil, err
	}

	u := &User{
		UUID: uuid,
	}

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
