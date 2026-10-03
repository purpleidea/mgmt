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

// Package json encodes and decodes golang values as json, keeping distinctions
// that the gob encoding loses, such as a nil pointer versus a pointer to a zero
// value. It's what we use to encode resources, but it works for any value.
package json

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"

	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
)

// jsonOptions are what we use to encode and decode values as json. A nil slice
// or map is encoded as null so that it stays distinct from an empty one.
// Complex numbers, which json has no type for, are encoded as strings, which
// also lets them hold the NaN and Inf values that json numbers can't. Floats
// which are NaN or Inf are encoded as strings for the same reason, but all the
// other floats are left as json numbers. A value in an interface is encoded
// with its lang type, so that it can be decoded back into the same golang type.
var jsonOptions = json.JoinOptions(
	json.Deterministic(true),
	json.FormatNilSliceAsNull(true),
	json.FormatNilMapAsNull(true),
	json.RejectUnknownMembers(true),
	json.WithMarshalers(json.JoinMarshalers(
		// This gets called for each value with a type of interface{}.
		json.MarshalToFunc(encodeInterface),
		json.MarshalToFunc(func(enc *jsontext.Encoder, f float32) error {
			return encodeNonFinite(enc, float64(f))
		}),
		json.MarshalToFunc(func(enc *jsontext.Encoder, f float64) error {
			return encodeNonFinite(enc, f)
		}),
		json.MarshalToFunc(func(enc *jsontext.Encoder, c complex64) error {
			return encodeComplex(enc, complex128(c), 64)
		}),
		json.MarshalToFunc(func(enc *jsontext.Encoder, c complex128) error {
			return encodeComplex(enc, c, 128)
		}),
	)),
	json.WithUnmarshalers(json.JoinUnmarshalers(
		// This gets called for each value with a type of interface{}.
		json.UnmarshalFromFunc(decodeInterface),
		json.UnmarshalFromFunc(func(dec *jsontext.Decoder, f *float32) error {
			x, err := decodeNonFinite(dec)
			if err != nil {
				return err
			}
			*f = float32(x)
			return nil
		}),
		json.UnmarshalFromFunc(func(dec *jsontext.Decoder, f *float64) error {
			x, err := decodeNonFinite(dec)
			if err != nil {
				return err
			}
			*f = x
			return nil
		}),
		json.UnmarshalFromFunc(func(dec *jsontext.Decoder, c *complex64) error {
			x, err := decodeComplex(dec, 64)
			if err != nil {
				return err
			}
			*c = complex64(x)
			return nil
		}),
		json.UnmarshalFromFunc(func(dec *jsontext.Decoder, c *complex128) error {
			x, err := decodeComplex(dec, 128)
			if err != nil {
				return err
			}
			*c = x
			return nil
		}),
	)),
)

// encodeInterface writes a non-nil value from an interface as an object of its
// lang type and its value, since otherwise it would get decoded with a generic
// type, such as a float64 for any number. It errors if the golang type of the
// value isn't exactly the golang type of its lang type, such as an int instead
// of an int64, since then it couldn't be decoded as the same golang type. A nil
// value returns errors.ErrUnsupported, which makes the json package write null.
func encodeInterface(enc *jsontext.Encoder, v *interface{}) error {
	if *v == nil {
		return errors.ErrUnsupported
	}
	typ := reflect.TypeOf(*v)
	t, err := types.TypeOf(typ)
	if err != nil {
		return errwrap.Wrapf(err, "can't encode a %s in an interface", typ)
	}
	if r, err := reflectType(t); err != nil || r != typ {
		return fmt.Errorf("can't encode a %s in an interface, it isn't a lang type", typ)
	}

	if err := enc.WriteToken(jsontext.BeginObject); err != nil {
		return err
	}
	if err := enc.WriteToken(jsontext.String("type")); err != nil {
		return err
	}
	if err := enc.WriteToken(jsontext.String(t.String())); err != nil {
		return err
	}
	if err := enc.WriteToken(jsontext.String("value")); err != nil {
		return err
	}
	if err := json.MarshalEncode(enc, *v); err != nil {
		return err
	}
	return enc.WriteToken(jsontext.EndObject)
}

// decodeInterface reads a value which was written by encodeInterface, and sets
// the interface to it, with the golang type of its lang type. Null returns
// errors.ErrUnsupported, which makes the json package set a nil interface.
func decodeInterface(dec *jsontext.Decoder, v *interface{}) error {
	if dec.PeekKind() == 'n' {
		return errors.ErrUnsupported
	}

	// readName reads the expected member name of the object.
	readName := func(name string) error {
		tok, err := dec.ReadToken()
		if err != nil {
			return err
		}
		if tok.Kind() != '"' || tok.String() != name {
			return fmt.Errorf("expected `%s` in an interface, got: %s", name, tok)
		}
		return nil
	}

	if tok, err := dec.ReadToken(); err != nil {
		return err
	} else if tok.Kind() != '{' {
		return fmt.Errorf("expected an object for an interface, got: %s", tok)
	}
	if err := readName("type"); err != nil {
		return err
	}
	var s string
	if err := json.UnmarshalDecode(dec, &s); err != nil {
		return err
	}
	t := types.NewType(s)
	if t == nil {
		return fmt.Errorf("invalid type in an interface: %s", s)
	}
	typ, err := reflectType(t)
	if err != nil {
		return err
	}
	if err := readName("value"); err != nil {
		return err
	}
	val := reflect.New(typ)
	if err := json.UnmarshalDecode(dec, val.Interface()); err != nil {
		return err
	}
	if tok, err := dec.ReadToken(); err != nil {
		return err
	} else if tok.Kind() != '}' {
		return fmt.Errorf("unexpected data in an interface: %s", tok)
	}

	*v = val.Elem().Interface()
	return nil
}

// reflectType returns the golang type of a lang type. It returns an error for
// the types which can't be represented, such as a variant, instead of the panic
// or nil type that Reflect gives, since the type might come from decoded data.
func reflectType(t *types.Type) (typ reflect.Type, reterr error) {
	defer func() {
		if r := recover(); r != nil {
			reterr = fmt.Errorf("can't represent type %s: %v", t, r)
		}
	}()
	if typ = t.Reflect(); typ == nil {
		return nil, fmt.Errorf("can't represent type %s", t)
	}
	return typ, nil
}

// encodeNonFinite writes a NaN or Inf float as a string, with the same names
// that the json package uses for its nonfinite format. Any other float returns
// errors.ErrUnsupported, which makes the json package encode it as a number.
func encodeNonFinite(enc *jsontext.Encoder, f float64) error {
	switch {
	case math.IsNaN(f):
		return enc.WriteToken(jsontext.String("NaN"))
	case math.IsInf(f, 1):
		return enc.WriteToken(jsontext.String("Infinity"))
	case math.IsInf(f, -1):
		return enc.WriteToken(jsontext.String("-Infinity"))
	}
	return errors.ErrUnsupported
}

// decodeNonFinite reads a NaN or Inf float from a string. Anything which isn't
// a string returns errors.ErrUnsupported, which makes the json package decode
// it normally. Any other string, such as a finite float, is an error.
func decodeNonFinite(dec *jsontext.Decoder) (float64, error) {
	if dec.PeekKind() != '"' {
		return 0, errors.ErrUnsupported
	}
	var s string
	if err := json.UnmarshalDecode(dec, &s); err != nil {
		return 0, err
	}
	switch s {
	case "NaN":
		return math.NaN(), nil
	case "Infinity":
		return math.Inf(1), nil
	case "-Infinity":
		return math.Inf(-1), nil
	}
	return 0, fmt.Errorf("invalid float string: %q", s)
}

// encodeComplex writes a complex number of the given bit size as a string.
func encodeComplex(enc *jsontext.Encoder, c complex128, bitSize int) error {
	s := strconv.FormatComplex(c, 'g', -1, bitSize)
	return enc.WriteToken(jsontext.String(s))
}

// decodeComplex reads a complex number of the given bit size from a string.
func decodeComplex(dec *jsontext.Decoder, bitSize int) (complex128, error) {
	var s string
	if err := json.UnmarshalDecode(dec, &s); err != nil {
		return 0, err
	}
	return strconv.ParseComplex(s, bitSize)
}

// Marshal encodes the input as json, keeping distinctions such as nil versus
// empty, which matter for resource fields. Decode it with Unmarshal.
func Marshal(in interface{}) ([]byte, error) {
	return json.Marshal(in, jsonOptions)
}

// Unmarshal decodes json from Marshal into the output, which must be a pointer.
// It errors on any unknown fields.
func Unmarshal(data []byte, out interface{}) error {
	return json.Unmarshal(data, out, jsonOptions)
}
