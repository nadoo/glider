// Copyright (c) 2013 Damian Gryski <damian@gryski.com>
// Licensed under the GPLv3 or, at your option, any later version.

// Package camellia is an implementation of the CAMELLIA encryption algorithm
/*

   This is an unoptimized version based on the description in RFC 3713.

   References:
   http://en.wikipedia.org/wiki/Camellia_%28cipher%29
   https://info.isl.ntt.co.jp/crypt/eng/camellia/
*/
package camellia

import (
	"crypto/cipher"
	"encoding/binary"
	"strconv"
)

const BlockSize = 16

type KeySizeError int

func (k KeySizeError) Error() string {
	return "camellia: invalid key size " + strconv.Itoa(int(k))
}

type camelliaCipher struct {
	kw   [5]uint64
	k    [25]uint64
	ke   [7]uint64
	klen int
}

const (
	sigma1 = 0xA09E667F3BCC908B
	sigma2 = 0xB67AE8584CAA73B2
	sigma3 = 0xC6EF372FE94F82BE
	sigma4 = 0x54FF53A5F1D36F1C
	sigma5 = 0x10E527FADE682D1D
	sigma6 = 0xB05688C2B3E6C1FD
)

func init() {
	// initialize other sboxes
	for i := range sbox1 {
		sbox2[i] = rotl8(sbox1[i], 1)
		sbox3[i] = rotl8(sbox1[i], 7)
		sbox4[i] = sbox1[rotl8(uint8(i), 1)]
	}
}

func rotl128(k [2]uint64, rot uint) (hi, lo uint64) {

	if rot > 64 {
		rot -= 64
		k[0], k[1] = k[1], k[0]
	}

	t := k[0] >> (64 - rot)
	hi = (k[0] << rot) | (k[1] >> (64 - rot))
	lo = (k[1] << rot) | t
	return hi, lo
}

func rotl32(k uint32, rot uint) uint32 {
	return (k << rot) | (k >> (32 - rot))
}

func rotl8(k byte, rot uint) byte {
	return (k << rot) | (k >> (8 - rot))
}

// New creates and returns a new cipher.Block.
// The key argument should be 16, 24, or 32 bytes.
func New(key []byte) (cipher.Block, error) {

	klen := len(key)
	switch klen {
	default:
		return nil, KeySizeError(klen)
	case 16, 24, 32:
		break
	}

	var d1, d2 uint64

	var kl [2]uint64
	var kr [2]uint64
	var ka [2]uint64
	var kb [2]uint64

	kl[0] = binary.BigEndian.Uint64(key[0:])
	kl[1] = binary.BigEndian.Uint64(key[8:])

	switch klen {
	case 24:
		kr[0] = binary.BigEndian.Uint64(key[16:])
		kr[1] = ^kr[0]
	case 32:
		kr[0] = binary.BigEndian.Uint64(key[16:])
		kr[1] = binary.BigEndian.Uint64(key[24:])

	}

	d1 = (kl[0] ^ kr[0])
	d2 = (kl[1] ^ kr[1])

	d2 = d2 ^ f(d1, sigma1)
	d1 = d1 ^ f(d2, sigma2)

	d1 = d1 ^ (kl[0])
	d2 = d2 ^ (kl[1])
	d2 = d2 ^ f(d1, sigma3)
	d1 = d1 ^ f(d2, sigma4)
	ka[0] = d1
	ka[1] = d2
	d1 = (ka[0] ^ kr[0])
	d2 = (ka[1] ^ kr[1])
	d2 = d2 ^ f(d1, sigma5)
	d1 = d1 ^ f(d2, sigma6)
	kb[0] = d1
	kb[1] = d2

	// here we generate our keys
	c := new(camelliaCipher)

	c.klen = klen

	if klen == 16 {

		c.kw[1], c.kw[2] = rotl128(kl, 0)

		c.k[1], c.k[2] = rotl128(ka, 0)
		c.k[3], c.k[4] = rotl128(kl, 15)
		c.k[5], c.k[6] = rotl128(ka, 15)

		c.ke[1], c.ke[2] = rotl128(ka, 30)

		c.k[7], c.k[8] = rotl128(kl, 45)
		c.k[9], _ = rotl128(ka, 45)
		_, c.k[10] = rotl128(kl, 60)
		c.k[11], c.k[12] = rotl128(ka, 60)

		c.ke[3], c.ke[4] = rotl128(kl, 77)

		c.k[13], c.k[14] = rotl128(kl, 94)
		c.k[15], c.k[16] = rotl128(ka, 94)
		c.k[17], c.k[18] = rotl128(kl, 111)

		c.kw[3], c.kw[4] = rotl128(ka, 111)

	} else {
		// 24 or 32

		c.kw[1], c.kw[2] = rotl128(kl, 0)

		c.k[1], c.k[2] = rotl128(kb, 0)
		c.k[3], c.k[4] = rotl128(kr, 15)
		c.k[5], c.k[6] = rotl128(ka, 15)

		c.ke[1], c.ke[2] = rotl128(kr, 30)

		c.k[7], c.k[8] = rotl128(kb, 30)
		c.k[9], c.k[10] = rotl128(kl, 45)
		c.k[11], c.k[12] = rotl128(ka, 45)

		c.ke[3], c.ke[4] = rotl128(kl, 60)

		c.k[13], c.k[14] = rotl128(kr, 60)
		c.k[15], c.k[16] = rotl128(kb, 60)
		c.k[17], c.k[18] = rotl128(kl, 77)

		c.ke[5], c.ke[6] = rotl128(ka, 77)

		c.k[19], c.k[20] = rotl128(kr, 94)
		c.k[21], c.k[22] = rotl128(ka, 94)
		c.k[23], c.k[24] = rotl128(kl, 111)

		c.kw[3], c.kw[4] = rotl128(kb, 111)
	}

	return c, nil
}

func (c *camelliaCipher) Encrypt(dst, src []byte) {

	d1 := binary.BigEndian.Uint64(src[0:])
	d2 := binary.BigEndian.Uint64(src[8:])

	d1 ^= c.kw[1]
	d2 ^= c.kw[2]

	d2 = d2 ^ f(d1, c.k[1])
	d1 = d1 ^ f(d2, c.k[2])
	d2 = d2 ^ f(d1, c.k[3])
	d1 = d1 ^ f(d2, c.k[4])
	d2 = d2 ^ f(d1, c.k[5])
	d1 = d1 ^ f(d2, c.k[6])

	d1 = fl(d1, c.ke[1])
	d2 = flinv(d2, c.ke[2])

	d2 = d2 ^ f(d1, c.k[7])
	d1 = d1 ^ f(d2, c.k[8])
	d2 = d2 ^ f(d1, c.k[9])
	d1 = d1 ^ f(d2, c.k[10])
	d2 = d2 ^ f(d1, c.k[11])
	d1 = d1 ^ f(d2, c.k[12])

	d1 = fl(d1, c.ke[3])
	d2 = flinv(d2, c.ke[4])

	d2 = d2 ^ f(d1, c.k[13])
	d1 = d1 ^ f(d2, c.k[14])
	d2 = d2 ^ f(d1, c.k[15])
	d1 = d1 ^ f(d2, c.k[16])
	d2 = d2 ^ f(d1, c.k[17])
	d1 = d1 ^ f(d2, c.k[18])

	if c.klen > 16 {
		// 24 or 32

		d1 = fl(d1, c.ke[5])
		d2 = flinv(d2, c.ke[6])

		d2 = d2 ^ f(d1, c.k[19])
		d1 = d1 ^ f(d2, c.k[20])
		d2 = d2 ^ f(d1, c.k[21])
		d1 = d1 ^ f(d2, c.k[22])
		d2 = d2 ^ f(d1, c.k[23])
		d1 = d1 ^ f(d2, c.k[24])
	}

	d2 = d2 ^ c.kw[3]
	d1 = d1 ^ c.kw[4]

	binary.BigEndian.PutUint64(dst[0:], d2)
	binary.BigEndian.PutUint64(dst[8:], d1)
}

func (c *camelliaCipher) Decrypt(dst, src []byte) {

	d2 := binary.BigEndian.Uint64(src[0:])
	d1 := binary.BigEndian.Uint64(src[8:])

	d1 = d1 ^ c.kw[4]
	d2 = d2 ^ c.kw[3]

	if c.klen > 16 {
		// 24 or 32

		d1 = d1 ^ f(d2, c.k[24])
		d2 = d2 ^ f(d1, c.k[23])
		d1 = d1 ^ f(d2, c.k[22])
		d2 = d2 ^ f(d1, c.k[21])
		d1 = d1 ^ f(d2, c.k[20])
		d2 = d2 ^ f(d1, c.k[19])

		d2 = fl(d2, c.ke[6])
		d1 = flinv(d1, c.ke[5])
	}

	d1 = d1 ^ f(d2, c.k[18])
	d2 = d2 ^ f(d1, c.k[17])
	d1 = d1 ^ f(d2, c.k[16])
	d2 = d2 ^ f(d1, c.k[15])
	d1 = d1 ^ f(d2, c.k[14])
	d2 = d2 ^ f(d1, c.k[13])

	d2 = fl(d2, c.ke[4])
	d1 = flinv(d1, c.ke[3])

	d1 = d1 ^ f(d2, c.k[12])
	d2 = d2 ^ f(d1, c.k[11])
	d1 = d1 ^ f(d2, c.k[10])
	d2 = d2 ^ f(d1, c.k[9])
	d1 = d1 ^ f(d2, c.k[8])
	d2 = d2 ^ f(d1, c.k[7])

	d2 = fl(d2, c.ke[2])
	d1 = flinv(d1, c.ke[1])

	d1 = d1 ^ f(d2, c.k[6])
	d2 = d2 ^ f(d1, c.k[5])
	d1 = d1 ^ f(d2, c.k[4])
	d2 = d2 ^ f(d1, c.k[3])
	d1 = d1 ^ f(d2, c.k[2])
	d2 = d2 ^ f(d1, c.k[1])

	d2 ^= c.kw[2]
	d1 ^= c.kw[1]

	binary.BigEndian.PutUint64(dst[0:], d1)
	binary.BigEndian.PutUint64(dst[8:], d2)
}

func (c *camelliaCipher) BlockSize() int {
	return BlockSize
}

func f(fin, ke uint64) uint64 {
	var x uint64
	x = fin ^ ke
	t1 := sbox1[uint8(x>>56)]
	t2 := sbox2[uint8(x>>48)]
	t3 := sbox3[uint8(x>>40)]
	t4 := sbox4[uint8(x>>32)]
	t5 := sbox2[uint8(x>>24)]
	t6 := sbox3[uint8(x>>16)]
	t7 := sbox4[uint8(x>>8)]
	t8 := sbox1[uint8(x)]
	y1 := t1 ^ t3 ^ t4 ^ t6 ^ t7 ^ t8
	y2 := t1 ^ t2 ^ t4 ^ t5 ^ t7 ^ t8
	y3 := t1 ^ t2 ^ t3 ^ t5 ^ t6 ^ t8
	y4 := t2 ^ t3 ^ t4 ^ t5 ^ t6 ^ t7
	y5 := t1 ^ t2 ^ t6 ^ t7 ^ t8
	y6 := t2 ^ t3 ^ t5 ^ t7 ^ t8
	y7 := t3 ^ t4 ^ t5 ^ t6 ^ t8
	y8 := t1 ^ t4 ^ t5 ^ t6 ^ t7
	return uint64(y1)<<56 | uint64(y2)<<48 | uint64(y3)<<40 | uint64(y4)<<32 | uint64(y5)<<24 | uint64(y6)<<16 | uint64(y7)<<8 | uint64(y8)
}

func fl(flin, ke uint64) uint64 {
	x1 := uint32(flin >> 32)
	x2 := uint32(flin & 0xffffffff)
	k1 := uint32(ke >> 32)
	k2 := uint32(ke & 0xffffffff)
	x2 = x2 ^ rotl32(x1&k1, 1)
	x1 = x1 ^ (x2 | k2)
	return uint64(x1)<<32 | uint64(x2)
}

func flinv(flin, ke uint64) uint64 {
	y1 := uint32(flin >> 32)
	y2 := uint32(flin & 0xffffffff)
	k1 := uint32(ke >> 32)
	k2 := uint32(ke & 0xffffffff)
	y1 = y1 ^ (y2 | k2)
	y2 = y2 ^ rotl32(y1&k1, 1)
	return uint64(y1)<<32 | uint64(y2)
}

var sbox1 = [...]byte{
	0x70, 0x82, 0x2c, 0xec, 0xb3, 0x27, 0xc0, 0xe5, 0xe4, 0x85, 0x57, 0x35, 0xea, 0x0c, 0xae, 0x41,
	0x23, 0xef, 0x6b, 0x93, 0x45, 0x19, 0xa5, 0x21, 0xed, 0x0e, 0x4f, 0x4e, 0x1d, 0x65, 0x92, 0xbd,
	0x86, 0xb8, 0xaf, 0x8f, 0x7c, 0xeb, 0x1f, 0xce, 0x3e, 0x30, 0xdc, 0x5f, 0x5e, 0xc5, 0x0b, 0x1a,
	0xa6, 0xe1, 0x39, 0xca, 0xd5, 0x47, 0x5d, 0x3d, 0xd9, 0x01, 0x5a, 0xd6, 0x51, 0x56, 0x6c, 0x4d,
	0x8b, 0x0d, 0x9a, 0x66, 0xfb, 0xcc, 0xb0, 0x2d, 0x74, 0x12, 0x2b, 0x20, 0xf0, 0xb1, 0x84, 0x99,
	0xdf, 0x4c, 0xcb, 0xc2, 0x34, 0x7e, 0x76, 0x05, 0x6d, 0xb7, 0xa9, 0x31, 0xd1, 0x17, 0x04, 0xd7,
	0x14, 0x58, 0x3a, 0x61, 0xde, 0x1b, 0x11, 0x1c, 0x32, 0x0f, 0x9c, 0x16, 0x53, 0x18, 0xf2, 0x22,
	0xfe, 0x44, 0xcf, 0xb2, 0xc3, 0xb5, 0x7a, 0x91, 0x24, 0x08, 0xe8, 0xa8, 0x60, 0xfc, 0x69, 0x50,
	0xaa, 0xd0, 0xa0, 0x7d, 0xa1, 0x89, 0x62, 0x97, 0x54, 0x5b, 0x1e, 0x95, 0xe0, 0xff, 0x64, 0xd2,
	0x10, 0xc4, 0x00, 0x48, 0xa3, 0xf7, 0x75, 0xdb, 0x8a, 0x03, 0xe6, 0xda, 0x09, 0x3f, 0xdd, 0x94,
	0x87, 0x5c, 0x83, 0x02, 0xcd, 0x4a, 0x90, 0x33, 0x73, 0x67, 0xf6, 0xf3, 0x9d, 0x7f, 0xbf, 0xe2,
	0x52, 0x9b, 0xd8, 0x26, 0xc8, 0x37, 0xc6, 0x3b, 0x81, 0x96, 0x6f, 0x4b, 0x13, 0xbe, 0x63, 0x2e,
	0xe9, 0x79, 0xa7, 0x8c, 0x9f, 0x6e, 0xbc, 0x8e, 0x29, 0xf5, 0xf9, 0xb6, 0x2f, 0xfd, 0xb4, 0x59,
	0x78, 0x98, 0x06, 0x6a, 0xe7, 0x46, 0x71, 0xba, 0xd4, 0x25, 0xab, 0x42, 0x88, 0xa2, 0x8d, 0xfa,
	0x72, 0x07, 0xb9, 0x55, 0xf8, 0xee, 0xac, 0x0a, 0x36, 0x49, 0x2a, 0x68, 0x3c, 0x38, 0xf1, 0xa4,
	0x40, 0x28, 0xd3, 0x7b, 0xbb, 0xc9, 0x43, 0xc1, 0x15, 0xe3, 0xad, 0xf4, 0x77, 0xc7, 0x80, 0x9e,
}

var sbox2 [256]byte
var sbox3 [256]byte
var sbox4 [256]byte

var _ cipher.Block = &camelliaCipher{}
