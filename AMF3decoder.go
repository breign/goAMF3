package amf

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"time"
)

// decodeAMF3 is the entry point for decoding an AMF3-encoded byte slice.
func DecodeAMF3(r *bytes.Reader) (interface{}, error) {
	marker, err := r.ReadByte()
	if err != nil {
		return nil, err
	}

	switch marker {
	case amf3Null:
		return nil, nil
	case amf3False:
		return false, nil
	case amf3True:
		return true, nil
	case amf3Integer:
		return decodeInteger3(r)
	case amf3Double:
		return decodeDouble3(r)
	case amf3String:
		return decodeString3(r)
	case amf3Date:
		return decodeDate3(r)
	case amf3Array:
		return decodeArray3(r)
	case amf3Object:
		return decodeObject3(r)
	case amf3ByteArray:
		return decodeByteArray3(r)
	default:
		return nil, errors.New("unknown AMF3 marker")
	}
}

// Helper function to decode U29 from the reader
func decodeU29(r *bytes.Reader) (uint32, int, error) {
	var value uint32
	bytesRead := 0
	for {
		if bytesRead > 3 {
			return 0, bytesRead, errors.New("U29 value too long")
		}
		b, err := r.ReadByte()
		if err != nil {
			return 0, bytesRead, err
		}
		bytesRead++
		value = (value << 7) | uint32(b&0x7F)
		if (b & 0x80) == 0 {
			break
		}
	}
	return value, bytesRead, nil
}

// decodeInteger3 decodes an AMF3 integer.
func decodeInteger3(r *bytes.Reader) (int, error) {
	u29, _, err := decodeU29(r)
	if err != nil {
		return 0, err
	}
	// In AMF3, integers are represented as 29 bits so check for sign extension.
	if u29&0x10000000 != 0 { // Check if the 29th bit is set.
		u29 |= 536870911 // Extend the sign to the whole int.
	}
	return int(u29), nil
}

// decodeDouble3 decodes an AMF3 double (float64).
func decodeDouble3(r *bytes.Reader) (float64, error) {
	var bits uint64
	err := binary.Read(r, binary.BigEndian, &bits)
	if err != nil {
		return 0, err
	}
	return math.Float64frombits(bits), nil
}

// decodeString3 decodes an AMF3 string.
func decodeString3(r *bytes.Reader) (string, error) {
	length, _, err := decodeU29(r)
	if err != nil {
		return "", err
	}
	length >>= 1 // The lowest bit is a flag, so shift right to get the actual length.
	if length == 0 {
		return "", nil // Empty string.
	}
	str := make([]byte, length)
	_, err = io.ReadFull(r, str)
	if err != nil {
		return "", err
	}
	return string(str), nil
}

// decodeDate3 decodes an AMF3 date.
func decodeDate3(r *bytes.Reader) (time.Time, error) {
	_, _, err := decodeU29(r) // Consume the U29 representing the date (unused here).
	if err != nil {
		return time.Time{}, err
	}
	var millis float64
	err = binary.Read(r, binary.BigEndian, &millis)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(0, int64(millis)*int64(time.Millisecond)), nil
}

// decodeArray3 decodes an AMF3 array.
func decodeArray3(r *bytes.Reader) ([]interface{}, error) {
	length, _, err := decodeU29(r)
	if err != nil {
		return nil, err
	}
	// The lowest bit is a flag for the array type, so shift right to get the actual length.
	actualLength := length >> 1

	var arr []interface{}
	for i := uint32(0); i <= actualLength; i++ {
		if i == 0 {
			b, err := r.ReadByte()
			if err != nil {
				return nil, err // Handle the error properly.
			}
			if b == 1 { // nil byte as a first byte in array, just skip it
				continue // Skip only if the first byte is 0.
			} else {
				if err := r.UnreadByte(); err != nil {
					return nil, err // Handle potential error from UnreadByte.
				}
			}
		}
		elem, err := DecodeAMF3(r) // Assuming DecodeAMF3 is correctly defined to handle *bytes.Reader.
		if err != nil {
			return nil, err
		}
		arr = append(arr, elem)
	}

	return arr, nil
}

func decodePropertyName(reader *bytes.Reader) (string, int, error) {
	// Decode the U29 that represents the length of the property name
	lengthU29, bytesReadU29, err := decodeU29(reader)
	if err != nil {
		return "", bytesReadU29, err
	}

	// The length is stored as (length << 1) | 1, so we need to reverse this operation to get the actual length
	actualLength := lengthU29 >> 1
	if actualLength == 0 {
		return "", bytesReadU29, nil
	}

	// Read the property name bytes based on the length decoded
	nameBytes := make([]byte, actualLength)
	n, err := reader.Read(nameBytes)
	if err != nil {
		return "", bytesReadU29 + n, err
	}
	if uint32(n) != actualLength {
		return "", bytesReadU29 + n, fmt.Errorf("expected to read %d bytes for property name, but read %d", actualLength, n)
	}

	return string(nameBytes), bytesReadU29 + n, nil
}

// decodeObject3 decodes an AMF3 encoded object into a Go map[string]interface{}.
func decodeObject3(reader *bytes.Reader) (map[string]interface{}, error) {
	// Read the first byte to get the object marker
	marker, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	if marker != amf3Dynamic {
		return nil, fmt.Errorf("expected AMF3 object marker but got: %x", marker)
	}

	// The next part is the U29O-traits. For simplicity, we'll assume all objects are dynamic with no class definition
	// This is a simplification and might need to be adjusted for full AMF3 support
	u29Traits, _, err := decodeU29(reader)
	if err != nil {
		return nil, err
	}
	if (u29Traits & 0x03) != 1 { // Checking if the object is dynamic
		return nil, errors.New("only dynamic AMF3 objects are supported in this example")
	}

	result := make(map[string]interface{})

	// Decode dynamic properties until we hit an empty string marker indicating the end of the object
	for {
		key, _, err := decodePropertyName(reader)
		if err != nil {
			return nil, err
		}
		if key == "" { // End of dynamic properties
			break
		}

		value, err := DecodeAMF3(reader) // Assumes implementation of decodeAMF3 that directly reads from *bytes.Reader
		if err != nil {
			return nil, err
		}

		result[key] = value
	}

	return result, nil
}

// decodeByteArray3 decodes an AMF3 encoded byte array into a Go byte slice.
func decodeByteArray3(r *bytes.Reader) (interface{}, error) {
	length, _, err := decodeU29(r)
	if err != nil {
		return nil, err
	}
	// The lowest bit is a flag; shift right to get the actual length.
	length >>= 1

	byteArray := make([]byte, length)
	_, err = r.Read(byteArray)
	if err != nil {
		return nil, err
	}

	return byteArray, nil
}
