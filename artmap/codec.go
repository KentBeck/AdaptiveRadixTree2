package artmap

import (
	"encoding/binary"
	"reflect"
	"unsafe"
)

// keyKind categorizes an [OrderedKey] type for fast inline encoding.
// The value is set once at [New] time from the key's [reflect.Kind]
// and platform word size, and thereafter used by (*Ordered).encode
// via a direct switch — avoiding any indirect call or heap allocation
// on the Put/Get hot paths for numeric keys.
type keyKind uint8

const (
	kindString keyKind = iota
	kindUint8
	kindUint16
	kindUint32
	kindUint64
	kindInt8
	kindInt16
	kindInt32
	kindInt64
	kindFloat32
	kindFloat64
)

// maxFixedKey is the largest fixed-width numeric encoding produced by
// (*Ordered).encode, so callers can stack-allocate a buffer of this
// size.
const maxFixedKey = 8

// pickKind returns the keyKind for K, dispatched from K's
// reflect.Kind, plus the matching decode closure used by iterators
// and Min/Max-style readers.
func pickKind[K OrderedKey]() (keyKind, func([]byte) K) {
	t := reflect.TypeOf((*K)(nil)).Elem()
	switch t.Kind() {
	case reflect.String:
		return kindString, decString[K]
	case reflect.Uint8:
		return kindUint8, decUint8[K]
	case reflect.Uint16:
		return kindUint16, decUint16[K]
	case reflect.Uint32:
		return kindUint32, decUint32[K]
	case reflect.Uint64:
		return kindUint64, decUint64[K]
	case reflect.Uint, reflect.Uintptr:
		if t.Size() == 8 {
			return kindUint64, decUint64[K]
		}
		return kindUint32, decUint32[K]
	case reflect.Int8:
		return kindInt8, decInt8[K]
	case reflect.Int16:
		return kindInt16, decInt16[K]
	case reflect.Int32:
		return kindInt32, decInt32[K]
	case reflect.Int64:
		return kindInt64, decInt64[K]
	case reflect.Int:
		if t.Size() == 8 {
			return kindInt64, decInt64[K]
		}
		return kindInt32, decInt32[K]
	case reflect.Float32:
		return kindFloat32, decFloat32[K]
	case reflect.Float64:
		return kindFloat64, decFloat64[K]
	}
	panic("artmap: unsupported OrderedKey kind " + t.Kind().String())
}

// encode writes the byte-order-preserving encoding of key into buf
// (or, for strings, allocates a fresh []byte) and returns the encoded
// slice. buf must have len >= maxFixedKey; the returned slice may
// alias buf for numeric kinds, so callers must treat the result as
// read-only until the next call. The encoding rules are the ones
// documented on [OrderedKey].
func (o *Ordered[K, V]) encode(key K, buf []byte) []byte {
	switch o.kind {
	case kindString:
		return []byte(*(*string)(unsafe.Pointer(&key)))
	case kindUint8:
		buf[0] = *(*uint8)(unsafe.Pointer(&key))
		return buf[:1]
	case kindUint16:
		binary.BigEndian.PutUint16(buf[:2], *(*uint16)(unsafe.Pointer(&key)))
		return buf[:2]
	case kindUint32:
		binary.BigEndian.PutUint32(buf[:4], *(*uint32)(unsafe.Pointer(&key)))
		return buf[:4]
	case kindUint64:
		binary.BigEndian.PutUint64(buf[:8], *(*uint64)(unsafe.Pointer(&key)))
		return buf[:8]
	case kindInt8:
		buf[0] = *(*uint8)(unsafe.Pointer(&key)) ^ 0x80
		return buf[:1]
	case kindInt16:
		binary.BigEndian.PutUint16(buf[:2], *(*uint16)(unsafe.Pointer(&key))^0x8000)
		return buf[:2]
	case kindInt32:
		binary.BigEndian.PutUint32(buf[:4], *(*uint32)(unsafe.Pointer(&key))^0x80000000)
		return buf[:4]
	case kindInt64:
		binary.BigEndian.PutUint64(buf[:8], *(*uint64)(unsafe.Pointer(&key))^0x8000000000000000)
		return buf[:8]
	case kindFloat32:
		bits := *(*uint32)(unsafe.Pointer(&key))
		if bits&0x80000000 != 0 {
			bits = ^bits
		} else {
			bits ^= 0x80000000
		}
		binary.BigEndian.PutUint32(buf[:4], bits)
		return buf[:4]
	case kindFloat64:
		bits := *(*uint64)(unsafe.Pointer(&key))
		if bits&0x8000000000000000 != 0 {
			bits = ^bits
		} else {
			bits ^= 0x8000000000000000
		}
		binary.BigEndian.PutUint64(buf[:8], bits)
		return buf[:8]
	}
	panic("artmap: unreachable")
}

// String decoder: a fresh copy of b so the returned K owns its bytes.
func decString[K OrderedKey](b []byte) K {
	var k K
	s := string(b)
	*(*string)(unsafe.Pointer(&k)) = s
	return k
}

// Unsigned decoders: big-endian fixed width.

func decUint8[K OrderedKey](b []byte) K {
	var k K
	*(*uint8)(unsafe.Pointer(&k)) = b[0]
	return k
}

func decUint16[K OrderedKey](b []byte) K {
	var k K
	*(*uint16)(unsafe.Pointer(&k)) = binary.BigEndian.Uint16(b)
	return k
}

func decUint32[K OrderedKey](b []byte) K {
	var k K
	*(*uint32)(unsafe.Pointer(&k)) = binary.BigEndian.Uint32(b)
	return k
}

func decUint64[K OrderedKey](b []byte) K {
	var k K
	*(*uint64)(unsafe.Pointer(&k)) = binary.BigEndian.Uint64(b)
	return k
}

// Signed decoders: undo the sign-bit flip.

func decInt8[K OrderedKey](b []byte) K {
	var k K
	*(*uint8)(unsafe.Pointer(&k)) = b[0] ^ 0x80
	return k
}

func decInt16[K OrderedKey](b []byte) K {
	var k K
	*(*uint16)(unsafe.Pointer(&k)) = binary.BigEndian.Uint16(b) ^ 0x8000
	return k
}

func decInt32[K OrderedKey](b []byte) K {
	var k K
	*(*uint32)(unsafe.Pointer(&k)) = binary.BigEndian.Uint32(b) ^ 0x80000000
	return k
}

func decInt64[K OrderedKey](b []byte) K {
	var k K
	*(*uint64)(unsafe.Pointer(&k)) = binary.BigEndian.Uint64(b) ^ 0x8000000000000000
	return k
}

// Float decoders: undo the IEEE sign-or-all-bits flip.

func decFloat32[K OrderedKey](b []byte) K {
	bits := binary.BigEndian.Uint32(b)
	if bits&0x80000000 != 0 {
		bits ^= 0x80000000
	} else {
		bits = ^bits
	}
	var k K
	*(*uint32)(unsafe.Pointer(&k)) = bits
	return k
}

func decFloat64[K OrderedKey](b []byte) K {
	bits := binary.BigEndian.Uint64(b)
	if bits&0x8000000000000000 != 0 {
		bits ^= 0x8000000000000000
	} else {
		bits = ^bits
	}
	var k K
	*(*uint64)(unsafe.Pointer(&k)) = bits
	return k
}
