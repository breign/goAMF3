package goAMF3

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"
)

func EncodeAMF3(v interface{}) []byte {
	replyBuffer := bytes.NewBuffer(make([]byte, 0, 1024000))
	encoder := NewEncoder(replyBuffer)
	encoder.WriteValueAmf3(v)
	return replyBuffer.Bytes()
}

func DecodeAMF3(b []byte) interface{} {
	reader := bytes.NewReader(b)
	decoder := NewDecoder(reader, 0)
	return decoder.ReadValueAmf3()
}

type AMFByteArray []byte

const (
	amfSafeMaxDepth = 64
	amfIntMin       = -268435456
	amfIntMax       = 268435455
)

var jsonNumberType = reflect.TypeOf(json.Number(""))

// InspectAndConvertPayload dynamically handles conversion based on whether the payload is a struct or a slice of structs.
func InspectAndConvertPayload(v any) any {
	seen := make(map[uintptr]struct{}, 32)
	return amfSafeValue(reflect.ValueOf(v), 0, seen)
}

func amfSafeValue(rv reflect.Value, depth int, seen map[uintptr]struct{}) any {
	if depth > amfSafeMaxDepth {
		return nil
	}
	if !rv.IsValid() {
		return nil
	}

	for rv.Kind() == reflect.Interface || rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		if rv.Kind() == reflect.Pointer {
			ptr := rv.Pointer()
			if ptr != 0 {
				if _, ok := seen[ptr]; ok {
					return nil
				}
				seen[ptr] = struct{}{}
			}
		}
		rv = rv.Elem()
		if !rv.IsValid() {
			return nil
		}
	}

	// Keep time.Time (AMF3 Date)
	if rv.Type() == reflect.TypeOf(time.Time{}) {
		return rv.Interface().(time.Time)
	}

	// ADD: handle json.Number (type alias of string)
	// This avoids passing json.Number through to the AMF encoder.
	if rv.Type() == jsonNumberType {
		n := json.Number(rv.String())
		if i, err := n.Int64(); err == nil {
			if i >= amfIntMin && i <= amfIntMax {
				return int32(i)
			}
			return float64(i)
		}
		if f, err := n.Float64(); err == nil {
			i := int64(f)
			if float64(i) == f && i >= amfIntMin && i <= amfIntMax {
				return int32(i)
			}
			return f
		}
		// fallback: as plain string
		return n.String()
	}

	switch rv.Kind() {
	case reflect.Struct:
		return amfStructToMap(rv, depth, seen)

	case reflect.Slice:
		// your existing []byte / []uint8 handling unchanged...
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			b := make([]byte, rv.Len())
			reflect.Copy(reflect.ValueOf(b), rv)
			if rv.Type() == reflect.TypeOf(AMFByteArray(nil)) {
				return b
			}
			if len(b) > 0 && utf8.Valid(b) && !bytes.Contains(b, []byte{0}) {
				return string(b)
			}
			return b
		}
		n := rv.Len()
		arr := make([]any, 0, n)
		for i := 0; i < n; i++ {
			arr = append(arr, amfSafeValue(rv.Index(i), depth+1, seen))
		}
		return arr

	case reflect.Array:
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			b := make([]byte, rv.Len())
			for i := 0; i < rv.Len(); i++ {
				b[i] = byte(rv.Index(i).Uint())
			}
			return b
		}
		n := rv.Len()
		arr := make([]any, 0, n)
		for i := 0; i < n; i++ {
			arr = append(arr, amfSafeValue(rv.Index(i), depth+1, seen))
		}
		return arr

	case reflect.Map:
		out := make(map[string]any, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			k := iter.Key()
			kv := amfSafeValue(k, depth+1, seen)
			key := ""
			if ks, ok := kv.(string); ok {
				key = ks
			} else {
				key = fmt.Sprint(kv)
			}
			out[key] = amfSafeValue(iter.Value(), depth+1, seen)
		}
		return out

	// ADD: normalize ints to AMF int32 when possible, else float64
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i := rv.Int()
		if i >= amfIntMin && i <= amfIntMax {
			return int32(i)
		}
		return float64(i)

	// ADD: normalize uints similarly (note: AMF int is signed)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u := rv.Uint()
		if u <= uint64(amfIntMax) {
			return int32(u)
		}
		return float64(u)

	// ADD: normalize floats: integer-like in range => int32, else float64
	case reflect.Float32, reflect.Float64:
		f := rv.Float()
		i := int64(f)
		if float64(i) == f && i >= amfIntMin && i <= amfIntMax {
			return int32(i)
		}
		return f

	default:
		return rv.Interface()
	}
}

func amfStructToMap(rv reflect.Value, depth int, seen map[uintptr]struct{}) map[string]any {
	out := make(map[string]any, rv.NumField())
	rt := rv.Type()

	for i := 0; i < rv.NumField(); i++ {
		sf := rt.Field(i)
		// exported fields only
		if sf.PkgPath != "" {
			continue
		}

		// Respect json tags (aligns with your JSON wire)
		name, omitEmpty, skip := fieldJSONName(sf)
		if skip {
			continue
		}

		fv := rv.Field(i)

		// Flatten anonymous embedded structs (common in envelope types)
		if sf.Anonymous {
			// Only flatten struct or *struct
			k := fv.Kind()
			if k == reflect.Pointer {
				if fv.IsNil() {
					continue
				}
				k = fv.Elem().Kind()
			}
			if k == reflect.Struct {
				m := amfSafeValue(fv, depth+1, seen)
				if mm, ok := m.(map[string]any); ok {
					for kk, vv := range mm {
						if vv == nil {
							continue
						}
						out[kk] = vv
					}
					continue
				}
			}
		}

		if name == "" {
			name = sf.Name
		}

		val := amfSafeValue(fv, depth+1, seen)
		if val == nil {
			// omit nils to avoid goAMF3 reflect panics and noisy payload
			continue
		}
		if omitEmpty && isEmpty(val) {
			continue
		}
		out[name] = val
	}

	return out
}

func fieldJSONName(sf reflect.StructField) (name string, omitempty bool, skip bool) {
	tag := sf.Tag.Get("json")
	if tag == "-" {
		return "", false, true
	}
	if tag == "" {
		return "", false, false
	}

	parts := strings.Split(tag, ",")
	if len(parts) > 0 {
		name = parts[0]
	}
	for _, p := range parts[1:] {
		if p == "omitempty" {
			omitempty = true
		}
	}
	return name, omitempty, false
}

func isEmpty(v any) bool {
	switch x := v.(type) {
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	case []byte:
		return len(x) == 0
	}
	return v == nil
}
