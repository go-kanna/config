package gen

import (
	"go/types"

	"github.com/go-kanna/internal/enumtype"
	"github.com/go-kanna/internal/valuetype"
)

// readKind is how the text of a setting becomes a value of a field's type.
type readKind int

const (
	// readText takes the text as it is.
	readText readKind = iota
	// readBool reads it with strconv.ParseBool.
	readBool
	// readInt reads it with strconv.ParseInt, and readUint with ParseUint.
	readInt
	readUint
	// readFloat reads it with strconv.ParseFloat.
	readFloat
	// readDuration reads it with time.ParseDuration.
	readDuration
	// readEnum passes it to the constructor of an enum of kanna-value.
	readEnum
	// readValue reads what a value type of kanna-value holds, and passes that
	// to its constructor.
	readValue
	// readText unmarshals it with the UnmarshalText of the type's pointer.
	readUnmarshal
)

// reader is how a setting is read into a value of typ.
type reader struct {
	kind readKind
	typ  types.Type

	// bits is the size of a number.
	bits int

	// ctor is the constructor of an enum or a value type, and held the reader
	// of what a value type holds.
	ctor *types.Func
	held *reader
}

// readerOf returns how a setting is read into a value of t, the first of these
// that fits: time.Duration, a value type or an enum of kanna-value, a type
// whose pointer has UnmarshalText, and a type whose underlying type is a
// string, a bool, or a number. It reports false for any other type.
func readerOf(t types.Type) (reader, bool) {
	if isDuration(t) {
		return reader{kind: readDuration, typ: t}, true
	}
	if vt, ok := valuetype.Of(t); ok && vt.Constructor != nil {
		held, ok := readerOf(vt.Held)
		if !ok || held.kind == readValue || held.kind == readEnum {
			return reader{}, false
		}
		return reader{kind: readValue, typ: t, ctor: vt.Constructor, held: &held}, true
	}
	if et, ok := enumtype.Of(t); ok && et.Constructor != nil {
		return reader{kind: readEnum, typ: t, ctor: et.Constructor}, true
	}
	if unmarshals(t) {
		return reader{kind: readUnmarshal, typ: t}, true
	}

	b, ok := t.Underlying().(*types.Basic)
	if !ok {
		return reader{}, false
	}
	switch info := b.Info(); {
	case info&types.IsString != 0:
		return reader{kind: readText, typ: t}, true
	case info&types.IsBoolean != 0:
		return reader{kind: readBool, typ: t}, true
	case info&types.IsInteger != 0 && b.Kind() != types.Uintptr:
		kind := readInt
		if info&types.IsUnsigned != 0 {
			kind = readUint
		}
		return reader{kind: kind, typ: t, bits: bitsOf(b)}, true
	case info&types.IsFloat != 0:
		return reader{kind: readFloat, typ: t, bits: bitsOf(b)}, true
	}
	return reader{}, false
}

// isDuration reports whether t is time.Duration, which reads as 30s rather
// than as the number of nanoseconds it holds.
func isDuration(t types.Type) bool {
	n, ok := types.Unalias(t).(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == "time" && n.Obj().Name() == "Duration"
}

// unmarshals reports whether a pointer to t has UnmarshalText([]byte) error.
func unmarshals(t types.Type) bool {
	obj, _, _ := types.LookupFieldOrMethod(types.NewPointer(t), true, nil, "UnmarshalText")
	fn, ok := obj.(*types.Func)
	if !ok {
		return false
	}
	sig := fn.Signature()
	return sig.Params().Len() == 1 && sig.Results().Len() == 1 &&
		types.Identical(sig.Params().At(0).Type(), types.NewSlice(types.Typ[types.Byte])) &&
		types.Identical(sig.Results().At(0).Type(), types.Universe.Lookup("error").Type())
}

// bitsOf returns the size strconv reads a number of b in: 0 for an int and a
// uint, which strconv reads at their size.
func bitsOf(b *types.Basic) int {
	return map[types.BasicKind]int{
		types.Int8: 8, types.Uint8: 8, types.Int16: 16, types.Uint16: 16,
		types.Int32: 32, types.Uint32: 32, types.Float32: 32,
		types.Int64: 64, types.Uint64: 64, types.Float64: 64,
	}[b.Kind()]
}

// staticDefault reports whether the default of a field read by r is read when
// generating, and written as a Go value: a value type and a type with
// UnmarshalText read it when the program runs, as they read a setting.
func (r reader) staticDefault() bool {
	return r.kind != readValue && r.kind != readUnmarshal
}
