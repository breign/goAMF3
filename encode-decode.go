package AMF3

import (
	"bytes"
	"reflect"
	"time"
	"unicode"
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

func isTimeType(item interface{}) bool {
	_, ok := item.(time.Time)
	return ok
}

// InspectAndConvertPayload dynamically handles conversion based on whether the payload is a struct or a slice of structs.
func InspectAndConvertPayload(payload interface{}) interface{} {
	if isTimeType(payload) {
		return payload // Return time.Time objects as-is
	}

	payloadValue := reflect.ValueOf(payload)
	switch payloadValue.Kind() {
	case reflect.Slice, reflect.Array:
		// Skip slices of time.Time
		if payloadValue.Len() > 0 && isTimeType(payloadValue.Index(0).Interface()) {
			return payload
		}
		return SliceToIface(payload)
	case reflect.Struct, reflect.Ptr:
		// Skip time.Time objects
		if _, ok := payload.(time.Time); ok {
			return payload
		}
		return StructToIface(payload)
	default:
		return payload
	}
}

// to a slice of []map[string]interface{}
func SliceToIface(items interface{}) []map[string]interface{} {
	var result []map[string]interface{}
	itemsValue := reflect.ValueOf(items)
	if itemsValue.Kind() == reflect.Slice {
		for i := 0; i < itemsValue.Len(); i++ {
			item := itemsValue.Index(i).Interface()
			convertedItem := StructToIface(item)
			result = append(result, convertedItem)
		}
	}
	return result
}

func StructToIface(item interface{}) map[string]interface{} {
	itemValue := reflect.ValueOf(item)
	if itemValue.Kind() == reflect.Ptr {
		itemValue = itemValue.Elem()
	}

	result := make(map[string]interface{})
	if itemValue.Kind() == reflect.Struct {
		itemType := itemValue.Type()
		for i := 0; i < itemValue.NumField(); i++ {
			field := itemType.Field(i)
			fieldValue := itemValue.Field(i)

			// Check if the field is exported; PkgPath is empty for exported fields
			if field.PkgPath != "" {
				continue // Skip unexported fields
			}

			// Alternatively, check if the first letter of the field name is uppercase
			if !unicode.IsUpper(rune(field.Name[0])) {
				continue // Skip if the field is not exported based on name
			}

			// Check if the field is an embedded struct
			if field.Anonymous {
				// Recursively convert the embedded struct and merge its fields
				embeddedFields := StructToIface(fieldValue.Interface())
				for k, v := range embeddedFields {
					result[k] = v
				}
			} else {
				result[field.Name] = fieldValue.Interface()
			}
		}
	}
	return result
}

func StructToIfaceOLD2(item interface{}) map[string]interface{} {
	itemValue := reflect.ValueOf(item)
	if itemValue.Kind() == reflect.Ptr {
		itemValue = itemValue.Elem()
	}

	result := make(map[string]interface{})
	if itemValue.Kind() == reflect.Struct {
		itemType := itemValue.Type()
		for i := 0; i < itemValue.NumField(); i++ {
			field := itemType.Field(i)
			fieldValue := itemValue.Field(i)

			// Check if the field is an embedded struct
			if field.Anonymous {
				// Recursively convert the embedded struct and merge its fields
				embeddedFields := StructToIface(fieldValue.Interface())
				for k, v := range embeddedFields {
					result[k] = v
				}
			} else {
				result[field.Name] = fieldValue.Interface()
			}
		}
	}
	return result
}

// to a struct of map[string]interface{}
func StructToIfaceOLD(item interface{}) map[string]interface{} {
	itemValue := reflect.ValueOf(item)
	if itemValue.Kind() == reflect.Ptr {
		itemValue = itemValue.Elem()
	}
	if itemValue.Kind() == reflect.Struct {
		result := make(map[string]interface{})
		itemType := itemValue.Type()
		for i := 0; i < itemValue.NumField(); i++ {
			field := itemType.Field(i)
			fieldValue := itemValue.Field(i)
			result[field.Name] = fieldValue.Interface()
		}
		return result
	}
	return make(map[string]interface{})
}
