// Copyright 2022 Gregory Petrosyan <gregory.petrosyan@gmail.com>
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package rand

import (
	"encoding/binary"
	"hash/maphash"
	"io"
	"math"
	"math/bits"
)

const (
	int24Mask = 1<<24 - 1
	int31Mask = 1<<31 - 1
	int53Mask = 1<<53 - 1
	int63Mask = 1<<63 - 1
	intMask   = math.MaxInt

	randSizeof = 8*4 + 8 + 1
)

// Rand is a pseudo-random number generator based on the SFC64 algorithm by Chris Doty-Humphrey.
//
// SFC64 has a few different cycles that one might be on, depending on the seed;
// the expected period will be about 2^255. SFC64 incorporates a 64-bit counter which means that the absolute
// minimum cycle length is 2^64 and that distinct seeds will not run into each other for at least 2^64 iterations.
type Rand struct {
	sfc64
	readVal uint64
	readPos int8
}

// RandomSeed returns a pseudo-random seed value.
func RandomSeed() uint64 {
	return new(maphash.Hash).Sum64()
}

// New returns a generator seeded with the given value.
func New(seed uint64) *Rand {
	var r Rand
	r.Seed(seed)
	return &r
}

// Seed uses the provided seed value to initialize the generator to a deterministic state.
func (r *Rand) Seed(seed uint64) {
	r.init(seed, seed, seed, 1)
}

func (r *Rand) MarshalBinary() ([]byte, error) {
	var data [randSizeof]byte
	binary.LittleEndian.PutUint64(data[0:], r.a)
	binary.LittleEndian.PutUint64(data[8:], r.b)
	binary.LittleEndian.PutUint64(data[16:], r.c)
	binary.LittleEndian.PutUint64(data[24:], r.w)
	binary.LittleEndian.PutUint64(data[32:], r.readVal)
	data[40] = byte(r.readPos)
	return data[:], nil
}

func (r *Rand) UnmarshalBinary(data []byte) error {
	if len(data) < randSizeof {
		return io.ErrUnexpectedEOF
	}
	r.a = binary.LittleEndian.Uint64(data[0:])
	r.b = binary.LittleEndian.Uint64(data[8:])
	r.c = binary.LittleEndian.Uint64(data[16:])
	r.w = binary.LittleEndian.Uint64(data[24:])
	r.readVal = binary.LittleEndian.Uint64(data[32:])
	r.readPos = int8(data[40])
	return nil
}

// Float32 returns, as a float32, a pseudo-random number in the half-open interval [0.0,1.0).
func (r *Rand) Float32() float32 {
	return float32(r.next()&int24Mask) * 0x1.0p-24
}

// Float64 returns, as a float64, a pseudo-random number in the half-open interval [0.0,1.0).
func (r *Rand) Float64() float64 {
	return float64(r.next()&int53Mask) * 0x1.0p-53
}

// Int returns a non-negative pseudo-random int.
func (r *Rand) Int() int {
	return int(r.next() & intMask)
}

// Int31 returns a non-negative pseudo-random 31-bit integer as an int32.
func (r *Rand) Int31() int32 {
	return int32(r.next() & int31Mask)
}

// Int31n returns, as an int32, a non-negative pseudo-random number in the half-open interval [0,n). It panics if n <= 0.
func (r *Rand) Int31n(n int32) int32 {
	if n <= 0 {
		panic("invalid argument to Int31n")
	}
	return int32(r.Uint32n(uint32(n)))
}

// Int63 returns a non-negative pseudo-random 63-bit integer as an int64.
func (r *Rand) Int63() int64 {
	return int64(r.next() & int63Mask)
}

// Int63n returns, as an int64, a non-negative pseudo-random number in the half-open interval [0,n). It panics if n <= 0.
func (r *Rand) Int63n(n int64) int64 {
	if n <= 0 {
		panic("invalid argument to Int63n")
	}
	return int64(r.Uint64n(uint64(n)))
}

// Intn returns, as an int, a non-negative pseudo-random number in the half-open interval [0,n). It panics if n <= 0.
func (r *Rand) Intn(n int) int {
	if n <= 0 {
		panic("invalid argument to Intn")
	}
	return int(r.Uint64n(uint64(n)))
}

// Perm returns, as a slice of n ints, a pseudo-random permutation of the integers in the half-open interval [0,n).
func (r *Rand) Perm(n int) []int {
	p := make([]int, n)
	for i := 1; i < len(p); i++ {
		j := r.Uint64n(uint64(i) + 1)
		p[i] = p[j]
		p[j] = i
	}
	return p
}

// Read generates len(p) random bytes and writes them into p. It always returns len(p) and a nil error.
func (r *Rand) Read(p []byte) (n int, err error) {
	pos := r.readPos
	val := r.readVal
	for n = 0; n < len(p); n++ {
		if pos == 0 {
			val = r.next()
			pos = 8
		}
		p[n] = byte(val)
		val >>= 8
		pos--
	}
	r.readPos = pos
	r.readVal = val
	return
}

// Shuffle pseudo-randomizes the order of elements. n is the number of elements. Shuffle panics if n < 0.
// swap swaps the elements with indexes i and j.
func (r *Rand) Shuffle(n int, swap func(i, j int)) {
	if n < 0 {
		panic("invalid argument to Shuffle")
	}
	for i := n - 1; i > 0; i-- {
		j := int(r.Uint64n(uint64(i) + 1))
		swap(i, j)
	}
}

// Uint32 returns a pseudo-random 32-bit value as a uint32.
func (r *Rand) Uint32() uint32 {
	return uint32(r.next())
}

// Uint32n returns, as a uint32, a pseudo-random number in [0,n). Uint32n(0) returns 0.
func (r *Rand) Uint32n(n uint32) uint32 {
	// 32-bit version of Uint64n()
	v := r.next()
	res, frac := bits.Mul32(n, uint32(v))
	if frac < n {
		hi, _ := bits.Mul32(n, uint32(v>>32))
		_, carry := bits.Add32(frac, hi, 0)
		res += carry
	}
	return res
}

// Uint64 returns a pseudo-random 64-bit value as a uint64.
func (r *Rand) Uint64() uint64 {
	return r.next()
}

// Uint64n returns, as a uint64, a pseudo-random number in [0,n). Uint64n(0) returns 0.
func (r *Rand) Uint64n(n uint64) uint64 {
	// "An optimal algorithm for bounded random integers" by Stephen Canon, https://github.com/apple/swift/pull/39143
	res, frac := bits.Mul64(n, r.next())
	if frac < n {
		hi, _ := bits.Mul64(n, r.next())
		_, carry := bits.Add64(frac, hi, 0)
		res += carry
	}
	return res
}