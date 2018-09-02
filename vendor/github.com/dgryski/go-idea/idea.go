// Package idea implements the IDEA block cipher
/*
For more information, please see https://en.wikipedia.org/wiki/International_Data_Encryption_Algorithm

This implementation derived from Public Domain code by Colin Plumb available at https://www.schneier.com/book-applied-source.html
*/
package idea

import (
	"crypto/cipher"
	"encoding/binary"
	"strconv"
)

const rounds = 8
const keyLen = (6*rounds + 4)

// KeySizeError is returned for incorrect key sizes
type KeySizeError int

func (k KeySizeError) Error() string {
	return "idea: invalid key size " + strconv.Itoa(int(k))
}

type ideaCipher struct {
	ek [keyLen]uint16
	dk [keyLen]uint16
}

// NewCipher returns a cipher.Block implementing the IDEA block cipher.  The key argument should be 16 bytes.
func NewCipher(key []byte) (cipher.Block, error) {

	if l := len(key); l != 16 {
		return nil, KeySizeError(l)
	}

	cipher := &ideaCipher{}

	expandKey(key, cipher.ek[:])

	// key inversion is expensive, we could do this lazily
	invertKey(cipher.ek[:], cipher.dk[:])

	return cipher, nil
}

func (c *ideaCipher) BlockSize() int          { return 8 }
func (c *ideaCipher) Encrypt(dst, src []byte) { crypt(src, dst, c.ek[:]) }
func (c *ideaCipher) Decrypt(dst, src []byte) { crypt(src, dst, c.dk[:]) }

// mulInv computes the multiplicative inverse of x mod 2^16+1
func mulInv(x uint16) (ret uint16) {

	if x <= 1 {
		return x // 0 and 1 are self-inverse
	}

	t1 := uint16(0x10001 / uint32(x)) // Since x >= 2, this fits into 16 bits
	y := uint16(0x10001 % uint32(x))

	if y == 1 {
		return 1 - t1
	}

	var t0 uint16 = 1
	var q uint16

	for y != 1 {
		q = x / y
		x = x % y
		t0 += q * t1
		if x == 1 {
			return t0
		}
		q = y / x
		y = y % x
		t1 += q * t0
	}
	return 1 - t1
}

// mul computes x*y mod 2^16+1
func mul(x, y uint16) uint16 {

	if y == 0 {
		return 1 - x
	}

	if x == 0 {
		return 1 - y
	}

	t32 := uint32(x) * uint32(y)
	x = uint16(t32)
	y = uint16(t32 >> 16)

	if x < y {
		return x - y + 1
	}

	return x - y
}

// expandKey computes encryption round-keys from a user-supplied key
func expandKey(key []byte, EK []uint16) {
	var i, j int

	for j = 0; j < 8; j++ {
		EK[j] = (uint16(key[0]) << 8) + uint16(key[1])
		key = key[2:]
	}
	for i = 0; j < keyLen; j++ {
		i++
		EK[i+7] = EK[i&7]<<9 | EK[(i+1)&7]>>7
		EK = EK[i&8:]
		i &= 7
	}
}

// invertKey computes the decryption round-keys from a set of encryption round-keys
func invertKey(EK []uint16, DK []uint16) {

	var t1, t2, t3 uint16
	var p [keyLen]uint16
	pidx := keyLen
	ekidx := 0

	t1 = mulInv(EK[ekidx])
	ekidx++
	t2 = -EK[ekidx]
	ekidx++
	t3 = -EK[ekidx]
	ekidx++
	pidx--
	p[pidx] = mulInv(EK[ekidx])
	ekidx++
	pidx--
	p[pidx] = t3
	pidx--
	p[pidx] = t2
	pidx--
	p[pidx] = t1

	for i := 0; i < rounds-1; i++ {
		t1 = EK[ekidx]
		ekidx++
		pidx--
		p[pidx] = EK[ekidx]
		ekidx++
		pidx--
		p[pidx] = t1

		t1 = mulInv(EK[ekidx])
		ekidx++
		t2 = -EK[ekidx]
		ekidx++
		t3 = -EK[ekidx]
		ekidx++
		pidx--
		p[pidx] = mulInv(EK[ekidx])
		ekidx++
		pidx--
		p[pidx] = t2
		pidx--
		p[pidx] = t3
		pidx--
		p[pidx] = t1
	}

	t1 = EK[ekidx]
	ekidx++
	pidx--
	p[pidx] = EK[ekidx]
	ekidx++
	pidx--
	p[pidx] = t1

	t1 = mulInv(EK[ekidx])
	ekidx++
	t2 = -EK[ekidx]
	ekidx++
	t3 = -EK[ekidx]
	ekidx++
	pidx--
	p[pidx] = mulInv(EK[ekidx])
	pidx--
	p[pidx] = t3
	pidx--
	p[pidx] = t2
	pidx--
	p[pidx] = t1

	copy(DK, p[:])
}

// crypt performs IDEA encryption given input/output buffers and a set of round-keys
func crypt(inbuf, outbuf []byte, key []uint16) {

	var x1, x2, x3, x4, s2, s3 uint16

	x1 = binary.BigEndian.Uint16(inbuf[0:])
	x2 = binary.BigEndian.Uint16(inbuf[2:])
	x3 = binary.BigEndian.Uint16(inbuf[4:])
	x4 = binary.BigEndian.Uint16(inbuf[6:])

	for r := rounds; r > 0; r-- {

		x1 = mul(x1, key[0])
		key = key[1:]
		x2 += key[0]
		key = key[1:]
		x3 += key[0]
		key = key[1:]

		x4 = mul(x4, key[0])
		key = key[1:]

		s3 = x3
		x3 ^= x1
		x3 = mul(x3, key[0])
		key = key[1:]
		s2 = x2

		x2 ^= x4
		x2 += x3
		x2 = mul(x2, key[0])
		key = key[1:]
		x3 += x2

		x1 ^= x2
		x4 ^= x3

		x2 ^= s3
		x3 ^= s2

	}
	x1 = mul(x1, key[0])
	key = key[1:]

	x3 += key[0]
	key = key[1:]
	x2 += key[0]
	key = key[1:]
	x4 = mul(x4, key[0])

	binary.BigEndian.PutUint16(outbuf[0:], x1)
	binary.BigEndian.PutUint16(outbuf[2:], x3)
	binary.BigEndian.PutUint16(outbuf[4:], x2)
	binary.BigEndian.PutUint16(outbuf[6:], x4)
}
