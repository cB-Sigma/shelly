// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package bthome

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/subtle"
	"encoding/binary"
	"errors"
)

// ErrAuth means an encrypted packet failed its integrity check: a wrong
// key, a wrong address, or a damaged or forged packet.
var ErrAuth = errors.New("bthome: message integrity check failed")

// ccm is AES in counter with CBC-MAC mode (RFC 3610) for one key, a
// fixed nonce length and a fixed tag length. The standard library has no
// CCM, and BTHome needs only this small part of it.
type ccm struct {
	b      cipher.Block
	tagLen int
}

func newCCM(key []byte, tagLen int) (*ccm, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if tagLen < 4 || tagLen > 16 || tagLen%2 != 0 {
		return nil, errors.New("bthome: bad CCM tag length")
	}
	return &ccm{b: b, tagLen: tagLen}, nil
}

// lenField is L, the size of the message-length field: 15 minus the nonce
// length.
func lenField(nonce []byte) (int, error) {
	l := 15 - len(nonce)
	if l < 2 || l > 8 {
		return 0, errors.New("bthome: bad CCM nonce length")
	}
	return l, nil
}

// mac computes the CBC-MAC of the message and the additional data.
func (c *ccm) mac(nonce, msg, aad []byte) ([]byte, error) {
	l, err := lenField(nonce)
	if err != nil {
		return nil, err
	}
	if l < 8 && uint64(len(msg)) >= 1<<(8*uint(l)) {
		return nil, errors.New("bthome: message too long for the nonce")
	}
	var b0 [16]byte
	if len(aad) > 0 {
		b0[0] |= 0x40
	}
	b0[0] |= byte(((c.tagLen - 2) / 2) << 3)
	b0[0] |= byte(l - 1)
	copy(b0[1:], nonce)
	n := uint64(len(msg))
	for i := 15; i > 15-l; i-- {
		b0[i] = byte(n)
		n >>= 8
	}
	x := make([]byte, 16)
	c.b.Encrypt(x, b0[:])
	feed := func(data []byte) {
		for len(data) > 0 {
			var blk [16]byte
			k := copy(blk[:], data)
			data = data[k:]
			for i := range blk {
				x[i] ^= blk[i]
			}
			c.b.Encrypt(x, x)
		}
	}
	if len(aad) > 0 {
		var head []byte
		if len(aad) < 0xFF00 {
			head = binary.BigEndian.AppendUint16(nil, uint16(len(aad)))
		} else {
			head = append([]byte{0xFF, 0xFE}, binary.BigEndian.AppendUint32(nil, uint32(len(aad)))...)
		}
		feed(append(head, aad...))
	}
	feed(msg)
	return x[:c.tagLen], nil
}

// ctr XORs data with the key stream starting at counter block i.
func (c *ccm) ctr(nonce []byte, i uint64, data []byte) []byte {
	l, _ := lenField(nonce)
	out := make([]byte, len(data))
	var a, s [16]byte
	a[0] = byte(l - 1)
	copy(a[1:], nonce)
	for off := 0; off < len(data); off += 16 {
		n := i
		for j := 15; j > 15-l; j-- {
			a[j] = byte(n)
			n >>= 8
		}
		c.b.Encrypt(s[:], a[:])
		end := off + 16
		if end > len(data) {
			end = len(data)
		}
		for j := off; j < end; j++ {
			out[j] = data[j] ^ s[j-off]
		}
		i++
	}
	return out
}

// seal encrypts msg and returns the ciphertext and the tag.
func (c *ccm) seal(nonce, msg, aad []byte) (ct, tag []byte, err error) {
	t, err := c.mac(nonce, msg, aad)
	if err != nil {
		return nil, nil, err
	}
	s0 := c.ctr(nonce, 0, make([]byte, c.tagLen))
	tag = make([]byte, c.tagLen)
	for i := range tag {
		tag[i] = t[i] ^ s0[i]
	}
	return c.ctr(nonce, 1, msg), tag, nil
}

// open decrypts ct and checks tag.
func (c *ccm) open(nonce, ct, tag, aad []byte) ([]byte, error) {
	if len(tag) != c.tagLen {
		return nil, ErrAuth
	}
	if _, err := lenField(nonce); err != nil {
		return nil, err
	}
	msg := c.ctr(nonce, 1, ct)
	t, err := c.mac(nonce, msg, aad)
	if err != nil {
		return nil, err
	}
	s0 := c.ctr(nonce, 0, make([]byte, c.tagLen))
	want := make([]byte, c.tagLen)
	for i := range want {
		want[i] = t[i] ^ s0[i]
	}
	if subtle.ConstantTimeCompare(want, tag) != 1 {
		return nil, ErrAuth
	}
	return msg, nil
}
