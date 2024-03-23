package amf

import (
	"bytes"
	"encoding/binary"
	"math"
	"time"
)

const (
	amf3MaxInt = 268435455  // (2^28)-1
	amf3MinInt = -268435456 // -(2^28)
)

func EncodeAMF3(v interface{}) []byte {
	switch v := v.(type) {
	case float64:
		return encodeDouble3(v)
	case int:
		return encodeInteger3(v)
	case uint:
		return encodeInteger3(int(v))
	case bool:
		return encodeBoolean3(v)
	case string:
		return encodeString3(v)
	case []byte:
		return encodeByteArray3(v)
	case nil:
		return encodeNull3()
	case map[string]interface{}:
		return encodeObject3(v)
	case time.Time:
		return encodeDate3(v)
	case []interface{}:
		return encodeArray3(v)
	}
	return nil
}

func encodeU29(v uint) []byte {
	msg := make([]byte, 0, 4)
	v &= 0x1fffffff
	if v <= 0x7f {
		msg = append(msg, byte(v))
	} else if v <= 0x3fff {
		msg = append(msg, byte((v>>7)|0x80))
		msg = append(msg, byte(v&0x7f))
	} else if v <= 0x1fffff {
		msg = append(msg, byte((v>>14)|0x80))
		msg = append(msg, byte((v>>7)|0x80))
		msg = append(msg, byte(v&0x7f))
	} else {
		msg = append(msg, byte((v>>22)|0x80))
		msg = append(msg, byte((v>>14)|0x80))
		msg = append(msg, byte((v>>7)|0x80))
		msg = append(msg, byte(v&0x7f))
	}
	return msg
}

func encodeInteger3(v int) []byte {
	if v >= amf3MinInt && v <= amf3MaxInt {
		msg := make([]byte, 0, 1+4) // 1 header + up to 4 U29
		msg = append(msg, amf3Integer)
		msg = append(msg, encodeU29(uint(v))...)
		return msg
	} else {
		return encodeDouble3(float64(v))
	}
}

func encodeDouble3(v float64) []byte {
	msg := make([]byte, 1+8) // 1 header + 8 float64
	msg[0] = amf3Double
	binary.BigEndian.PutUint64(msg[1:], uint64(math.Float64bits(v)))
	return msg
}

func encodeBoolean3(v bool) []byte {
	if v {
		return []byte{amf3True}
	}
	return []byte{amf3False}
}

func encodeNull3() []byte {
	return []byte{amf3Null}
}

func encodeString3(v string) []byte {
	if v == "" {
		return []byte{0x01} // Special case for empty strings in AMF3
	}
	var buf bytes.Buffer
	// Length (U29): High bit for inline string, then length
	strLen := len(v)
	u29Len := (strLen << 1) | 1
	buf.WriteByte(amf3String)
	buf.Write(encodeU29(uint(u29Len)))
	buf.WriteString(v)
	return buf.Bytes()
}

func encodeDate3(v time.Time) []byte {
	// Convert time to milliseconds since Unix epoch
	milliseconds := v.UnixNano() / 1000000
	msg := make([]byte, 0, 1+1+8)      // 1 byte for AMF3 date marker, 1 byte for U29 data (empty reference), 8 bytes for the timestamp
	msg = append(msg, amf3Date)        // Date marker
	msg = append(msg, encodeU29(1)...) // The U29 here is a flag (1 << 1) indicating that what follows is an inline value
	// Append the timestamp as a double (64-bit floating point)
	timestamp := make([]byte, 8)
	binary.BigEndian.PutUint64(timestamp, math.Float64bits(float64(milliseconds)))
	msg = append(msg, timestamp...)
	return msg
}

func encodeArray3(arr []interface{}) []byte {
	var buf bytes.Buffer
	buf.WriteByte(amf3Array)                        // Write the array marker
	buf.Write(encodeU29(uint((len(arr) << 1) | 1))) // Write the array length (U29)
	buf.WriteByte(0x01)                             // Empty string (associative portion of the array)
	// Encode and write each element
	for _, element := range arr {
		buf.Write(EncodeAMF3(element))
	}
	return buf.Bytes()
}

func encodePropertyName(name string) []byte {
	var buf bytes.Buffer
	// Encode the length of the property name as U29 (note: no string marker for property names)
	buf.Write(encodeU29(uint(len(name))<<1 | 1)) // Length * 2 + 1 to indicate inline string
	buf.WriteString(name)                        // UTF-8 string data
	return buf.Bytes()
}

func encodeObject3(obj map[string]interface{}) []byte {
	var buf bytes.Buffer
	buf.WriteByte(amf3Object) // Start with the AMF3 object marker
	buf.WriteByte(0x0B)       // Marker for dynamic object with no class definition
	buf.WriteByte(0x01)       // Empty string for class name; adjust for actual class names
	// Encode dynamic properties
	for key, value := range obj {
		buf.Write(encodePropertyName(key))
		buf.Write(EncodeAMF3(value))
	}
	buf.WriteByte(0x01) // End of dynamic properties
	return buf.Bytes()
}

func encodeByteArray3(data []byte) []byte {
	var buf bytes.Buffer
	buf.WriteByte(amf3ByteArray) // Write the AMF3 ByteArray marker
	// Encode the length of the byte array. AMF3 uses the first bit as a marker for inline (1) vs reference (0).
	// Here, we shift left by 1 and then OR with 1 to indicate an inline object.
	length := len(data)
	buf.Write(encodeU29(uint(length)<<1 | 1))
	buf.Write(data) // Append the actual data
	return buf.Bytes()
}
