package property

import (
	"go.mongodb.org/mongo-driver/v2/bson"
)

// DecodeFunc extracts a value of type T from bson.Raw by key name.
type DecodeFunc[T any] func(raw bson.Raw, key string) T

// Primitive[T] is a lazy-decoded scalar property.
type Primitive[T any] struct {
	propertyBase
	decode DecodeFunc[T]
	raw    bson.Raw
	val    T
	loaded bool
}

func NewPrimitive[T any](name string, decode DecodeFunc[T]) *Primitive[T] {
	return &Primitive[T]{propertyBase: propertyBase{name: name}, decode: decode}
}

func (p *Primitive[T]) Init(raw bson.Raw) { p.raw = raw }

func (p *Primitive[T]) Get() T {
	if !p.loaded {
		if p.raw != nil {
			p.val = p.decode(p.raw, p.name)
		}
		p.loaded = true
	}
	return p.val
}

func (p *Primitive[T]) Set(v T) {
	p.val = v
	p.loaded = true
	p.markDirty()
}

// BSONValue returns the current value for BSON serialization.
func (p *Primitive[T]) BSONValue() any { return p.Get() }

// --- Decode functions for common types ---

func DecodeString(raw bson.Raw, key string) string {
	val, err := raw.LookupErr(key)
	if err != nil {
		return ""
	}
	s, _ := val.StringValueOK()
	return s
}

func DecodeBool(raw bson.Raw, key string) bool {
	val, err := raw.LookupErr(key)
	if err != nil {
		return false
	}
	b, _ := val.BooleanOK()
	return b
}

// DecodeInt32 reads an int32-typed gen field. Mendix/Studio Pro stores small
// integers (connection indices, canvas positions, …) as BSON int64 — and
// occasionally as double — not int32, so accept all three numeric encodings.
// Reading only Int32OK silently returns 0 for Studio-Pro-authored content, which
// surfaced as bogus @anchor(from: top, …) lines (index 0 = AnchorTop) on every
// flow whose real connection index (right=1 / left=3) was stored as int64.
func DecodeInt32(raw bson.Raw, key string) int32 {
	val, err := raw.LookupErr(key)
	if err != nil {
		return 0
	}
	if i, ok := val.Int32OK(); ok {
		return i
	}
	if i, ok := val.Int64OK(); ok {
		return int32(i)
	}
	if d, ok := val.DoubleOK(); ok {
		return int32(d)
	}
	return 0
}

func DecodeFloat64(raw bson.Raw, key string) float64 {
	val, err := raw.LookupErr(key)
	if err != nil {
		return 0
	}
	f, _ := val.DoubleOK()
	return f
}
