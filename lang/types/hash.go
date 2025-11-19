// Mgmt
// Copyright (C) James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//
// Additional permission under GNU GPL version 3 section 7
//
// If you modify this program, or any covered work, by linking or combining it
// with embedded mcl code and modules (and that the embedded mcl code and
// modules which link with this program, contain a copy of their source code in
// the authoritative form) containing parts covered by the terms of any other
// license, the licensors of this program grant you additional permission to
// convey the resulting work. Furthermore, the licensors of this program grant
// the original author, James Shubin, additional permission to update this
// additional permission if he deems it necessary to achieve the goals of this
// additional permission.

package types

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"hash/maphash"
	"math"
)

// IsHashableType returns true if you pass it a hashable type. These have a Hash
// method and as a result they can be used for map keys. Notably List, Map, Func
// and any other special types are not present in this list.
func IsHashableType(typ *Type) bool {
	_, ok := typ.New().(Hashable)
	if ok != isHashableKind(typ.Kind) { // bonus safety / consistency check!
		panic("inconsistent hashable implementation")
	}

	if typ.Kind != KindStruct {
		return ok
	}

	// recurse on struct fields
	if typ.Map == nil {
		panic("malformed struct type")
	}
	if len(typ.Map) != len(typ.Ord) {
		panic("malformed struct length")
	}

	for _, x := range typ.Ord {
		t, ok := typ.Map[x]
		if !ok {
			panic("malformed struct order")
		}
		if t == nil {
			panic("malformed struct field")
		}
		if !IsHashableType(t) {
			return false
		}
	}

	return true
}

// isHashableKind returns true if you pass it a hashable kind. These have a Hash
// method and as a result they can be used for map keys. Notably KindList,
// KindMap, KindFunc and any other special kinds are not present in this list.
// If you want to know if a struct is Hashable, you need to use IsHashableType
// which will also recurse into the struct fields, since they must be hashable.
func isHashableKind(kind Kind) bool {
	switch kind {
	case KindNil:
		return false // TODO: should they be? not sure why they would be
	case KindBool:
		return true
	case KindStr:
		return true
	case KindInt:
		return true
	case KindFloat:
		return true
	case KindList:
		return false
	case KindMap:
		return false
	case KindStruct:
		return true // make sure the also check the fields!
	case KindFunc:
		return false // not hashable!
	}
	return false // others
}

// Comparable takes a seed and a value and hashes it.
func Comparable[T comparable](seed Seed, v T) Hash {
	//return NewHasher(seed).Hash(v) // pre golang 1.24
	return Hash(maphash.Comparable(maphash.Seed(seed), v)) // golang 1.24+
}

// Hashable specifies that a particular Value can be hashed.
type Hashable interface {

	// Hash runs the hashing of the Value.
	Hash(Seed) Hash
}

// Hash is the type of our hash. This is taken from the golang
// hash/maphash.Sum64() return type. You should likely be able to easily change
// this without anything breaking.
type Hash uint64

// Seed is the type of our hashing seed. This is taken from the golang
// hash/maphash.MakeSeed() return type. You should likely be able to easily
// change this without anything breaking.
type Seed maphash.Seed

// MakeSeed generates a random seed for our hashing purposes.
func MakeSeed() Seed {
	return Seed(maphash.MakeSeed())
}

// Hasher is an implementation of a thing that can hash data for you.
type Hasher maphash.Hash

// NewHasher builds a new hasher! This is not currently safe for concurrent use.
func NewHasher(seed Seed) *Hasher {
	var h maphash.Hash
	h.SetSeed(maphash.Seed(seed))
	out := Hasher(h)
	return &out
}

// Hash returns the hash of an input value.
func (obj *Hasher) Hash(value Value) Hash {
	h := maphash.Hash(*obj)

	appendT(&h, value) // causes subsequent h.Write( ... )

	out := Hash(h.Sum64())
	h.Reset() // get it ready to use again
	return out
}

// similar to appendT in: src/hash/maphash/maphash_purego.go
func appendT(h *maphash.Hash, val Value) {
	h.WriteString(val.Type().String())

	switch val.Type().Kind {
	case KindBool:
		btoi := func(b bool) byte {
			if b {
				return 1
			}
			return 0
		}
		h.WriteByte(btoi(val.Bool()))

	case KindStr:
		h.WriteString(val.Str())

	case KindInt:
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], uint64(val.Int()))
		h.Write(buf[:])

	case KindFloat:
		float64Hash(h, val.Float())

	case KindStruct:
		obj, ok := val.(*StructValue)
		if !ok {
			panic("malformed struct value")
		}

		if obj.V == nil {
			panic("malformed struct")
		}
		if obj.T == nil {
			panic("malformed struct type")
		}
		if len(obj.V) != len(obj.T.Ord) {
			panic("malformed struct length")
		}
		if len(obj.T.Map) != len(obj.T.Ord) {
			panic("malformed struct length")
		}

		var buf [8]byte
		for i, x := range obj.T.Ord {
			v, exists := obj.V[x]
			if !exists {
				panic("malformed struct order")
			}
			if v == nil {
				panic("malformed struct field")
			}

			// x is a string
			// v is a types.Value
			// TODO: do we want to hash `x` (the field name) too?

			//byteorder.LEPutUint64(buf[:], uint64(i)) // internal
			binary.LittleEndian.PutUint64(buf[:], uint64(i))
			// do not want to hash to the same value,
			// struct{a,b string}{"foo",""} and
			// struct{a,b string}{"","foo"}.
			h.Write(buf[:])

			appendT(h, v) // causes subsequent h.Write( ... )
		}
	default:
		panic(fmt.Errorf("hash of unhashable type: %s", val.Type().String()))
	}
}

// similar to float64 in: src/hash/maphash/maphash_purego.go
func float64Hash(h *maphash.Hash, f float64) {
	if f == 0 {
		h.WriteByte(0)
		return
	}
	var buf [8]byte
	if f != f {
		binary.LittleEndian.PutUint64(buf[:], randUint64())
		h.Write(buf[:])
		return
	}
	binary.LittleEndian.PutUint64(buf[:], math.Float64bits(f))
	h.Write(buf[:])
}

// similar to randUint64 in: src/hash/maphash/maphash_purego.go
func randUint64() uint64 {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return binary.LittleEndian.Uint64(buf)
}
