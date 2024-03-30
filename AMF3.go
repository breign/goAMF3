package amf

import (
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"
)

type Reader interface {
	Read(p []byte) (n int, err error)
}

type Writer interface {
	Write(p []byte) (n int, err error)
}

type AvmObject struct {
	class         *AvmClass
	staticFields  []interface{}
	dynamicFields map[string]interface{}
}

type AvmClass struct {
	name           string
	externalizable bool
	dynamic        bool
	properties     []string
}

// An "Array" in AVM land is actually stored as a combination of an array and
// a dictionary.
type AvmArray struct {
	elements []interface{}
	fields   map[string]interface{}
}

// * Public functions *

// Read an AMF3 value from the stream.
func ReadValueAmf3(stream Reader) (interface{}, error) {
	cxt := &Decoder{}
	cxt.AmfVersion = 3
	cxt.stream = stream
	result := cxt.ReadValueAmf3()
	return result, cxt.decodeError
}

func WriteValueAmf3(stream Writer, value interface{}) error {
	cxt := &Encoder{}
	cxt.stream = stream
	return cxt.WriteValueAmf3(value)
}

// Type markers
const (
	AMF3Undefined      byte = 0x00
	AMF3Null           byte = 0x01
	AMF3False          byte = 0x02
	AMF3True           byte = 0x03
	AMF3Integer        byte = 0x04
	AMF3Double         byte = 0x05
	AMF3String         byte = 0x06
	AMF3Externalizable byte = 0x07
	AMF3Date           byte = 0x08
	AMF3Array          byte = 0x09
	AMF3Object         byte = 0x0a
	AMF3Dynamic        byte = 0x0b
	AMF3ByteArray      byte = 0x0c
	AMF3VectorInt      byte = 0x0d
	AMF3VectorUint     byte = 0x0d
	AMF3VectorDouble   byte = 0x0d
	AMF3VectorObject   byte = 0x0d
	AMF3Dictionary     byte = 0x11
)

type Decoder struct {
	stream Reader

	AmfVersion uint16

	// AMF3 messages can include references to previously-unpacked objects. These
	// tables hang on to objects for later use.
	stringTable []string
	classTable  []*AvmClass
	objectTable []interface{}

	decodeError error

	// When unpacking objects, we'll look in this map for the type name. If found,
	// we'll unpack the value into an instance of the associated type.
	typeMap map[string]reflect.Type
}

func NewDecoder(stream Reader, amfVersion uint16) *Decoder {
	decoder := &Decoder{}
	decoder.stream = stream
	decoder.AmfVersion = amfVersion
	decoder.typeMap = make(map[string]reflect.Type)
	return decoder
}

func (cxt *Decoder) saveError(err error) {
	if err == nil {
		return
	}
	if cxt.decodeError != nil {
		fmt.Println("warning: duplicate errors on Decoder")
	} else {
		cxt.decodeError = err
	}
}
func (cxt *Decoder) errored() bool {
	return cxt.decodeError != nil
}
func (cxt *Decoder) storeObjectInTable(obj interface{}) {
	cxt.objectTable = append(cxt.objectTable, obj)
}
func (cxt *Decoder) RegisterType(flexName string, instance interface{}) {
	cxt.typeMap[flexName] = reflect.TypeOf(instance)
}

// Helper functions.
func (cxt *Decoder) ReadByte() uint8 {
	buf := make([]byte, 1)
	_, err := cxt.stream.Read(buf)
	cxt.saveError(err)
	return buf[0]
}
func (cxt *Decoder) ReadUint8() uint8 {
	var value uint8
	err := binary.Read(cxt.stream, binary.BigEndian, &value)
	cxt.saveError(err)
	return value
}
func (cxt *Decoder) ReadUint16() uint16 {
	var value uint16
	err := binary.Read(cxt.stream, binary.BigEndian, &value)
	cxt.saveError(err)
	return value
}
func (cxt *Decoder) ReadUint32() uint32 {
	var value uint32
	err := binary.Read(cxt.stream, binary.BigEndian, &value)
	cxt.saveError(err)
	return value
}
func (cxt *Decoder) ReadFloat64() float64 {
	var value float64
	err := binary.Read(cxt.stream, binary.BigEndian, &value)
	cxt.saveError(err)
	return value
}
func (cxt *Encoder) WriteFloat64(value float64) error {
	return binary.Write(cxt.stream, binary.BigEndian, &value)
}
func (cxt *Decoder) ReadString() string {
	length := int(cxt.ReadUint16())
	if cxt.errored() {
		return ""
	}
	return cxt.ReadStringKnownLength(length)
}

func (cxt *Decoder) ReadStringKnownLength(length int) string {
	data := make([]byte, length)
	n, err := cxt.stream.Read(data)
	if n < length {
		cxt.saveError(fmt.Errorf("not enough bytes in ReadStringKnownLength (expected %d, found %d)", length, n))
		return ""
	}
	cxt.saveError(err)
	return string(data)
}

type Encoder struct {
	stream Writer
}

func NewEncoder(stream Writer) *Encoder {
	return &Encoder{stream}
}
func (cxt *Encoder) WriteUint16(value uint16) error {
	return binary.Write(cxt.stream, binary.BigEndian, &value)
}
func (cxt *Encoder) WriteUint32(value uint32) error {
	return binary.Write(cxt.stream, binary.BigEndian, &value)
}

func (cxt *Encoder) WriteString(str string) error {
	binary.Write(cxt.stream, binary.BigEndian, uint16(len(str)))
	_, err := cxt.stream.Write([]byte(str))
	return err
}
func (cxt *Encoder) writeByte(b uint8) error {
	return binary.Write(cxt.stream, binary.BigEndian, b)
}
func (cxt *Encoder) WriteBool(b bool) {
	val := 0x0
	if b {
		val = 0xff
	}
	binary.Write(cxt.stream, binary.BigEndian, uint8(val))
}

// Read a 29-bit compact encoded integer (as defined in AVM3)
func (cxt *Decoder) ReadUint29() uint32 {
	var result uint32 = 0
	for i := 0; i < 4; i++ {
		b := cxt.ReadByte()

		if cxt.errored() {
			return 0
		}

		if i == 3 {
			// Last byte does not use the special 0x80 bit.
			result = (result << 8) + uint32(b)
		} else {
			result = (result << 7) + (uint32(b) & 0x7f)
		}

		if (b & 0x80) == 0 {
			break
		}
	}
	return result
}

func (cxt *Encoder) WriteUint29(value uint32) error {

	// Make sure the value is only 29 bits.
	remainder := value & 0x1fffffff
	if remainder != value {
		fmt.Println("warning: WriteUint29 received a value that does not fit in 29 bits")
	}

	if remainder > 0x1fffff {
		cxt.writeByte(uint8(remainder>>22)&0x7f + 0x80)
		cxt.writeByte(uint8(remainder>>15)&0x7f + 0x80)
		cxt.writeByte(uint8(remainder>>8)&0x7f + 0x80)
		cxt.writeByte(uint8(remainder>>0) & 0xff)
	} else if remainder > 0x3fff {
		cxt.writeByte(uint8(remainder>>14)&0x7f + 0x80)
		cxt.writeByte(uint8(remainder>>7)&0x7f + 0x80)
		cxt.writeByte(uint8(remainder>>0) & 0x7f)
	} else if remainder > 0x7f {
		cxt.writeByte(uint8(remainder>>7)&0x7f + 0x80)
		cxt.writeByte(uint8(remainder>>0) & 0x7f)
	} else {
		cxt.writeByte(uint8(remainder))
	}

	return nil
}

func (cxt *Decoder) readByteArrayAmf3() []byte {
	// Decode the length as a U29 integer. This includes a flag in the lowest bit.
	ref := cxt.ReadUint29()

	// The lowest bit is a flag; shift right to get the actual length.
	length := int(ref >> 1)

	// Allocate the byte array with the obtained length.
	byteArray := make([]byte, length)

	// Read the byte array contents.
	n, err := cxt.stream.Read(byteArray)
	if err != nil {
		return nil
	}
	if n < length {
		// If we read fewer bytes than expected, it's an error.
		return nil
	}

	return byteArray
}

func (cxt *Encoder) writeByteArrayAmf3(data []byte) error {

	// Write the AMF3 ByteArray marker
	cxt.writeByte(AMF3ByteArray)

	// Encode the length of the byte array.
	length := len(data)
	cxt.WriteUint29(uint32(length)<<1 | 1)

	// Append the actual data
	_, err := cxt.stream.Write(data)
	return err
}

func (cxt *Decoder) readStringAmf3() string {
	ref := cxt.ReadUint29()

	if cxt.errored() {
		return ""
	}

	// Check the low bit to see if this is a reference
	if (ref & 1) == 0 {
		index := int(ref >> 1)
		if index >= len(cxt.stringTable) {
			cxt.saveError(fmt.Errorf("invalid string index: %d", index))
			return ""
		}

		return cxt.stringTable[index]
	}

	length := int(ref >> 1)

	if length == 0 {
		return ""
	}

	str := cxt.ReadStringKnownLength(length)
	cxt.stringTable = append(cxt.stringTable, str)

	return str
}

func (cxt *Decoder) readDateAmf3() interface{} {
	// Read the first U29 which includes the reference bit
	ref := cxt.ReadUint29()

	// Check for error after reading U29
	if cxt.errored() {
		return time.Time{}
	}

	// Check the low bit; for Date, we do not use object references,
	// so if the low bit is 0, it's an invalid format for a Date.
	if (ref & 1) == 0 {
		cxt.saveError(fmt.Errorf("invalid date format"))
		return time.Time{}
	}

	// Read the date value in milliseconds since the Unix epoch, encoded as a 64-bit floating point.
	millis := cxt.ReadFloat64()

	// Convert milliseconds to a time.Time object and return.
	// The Unix() method in time.Time accepts seconds and nanoseconds,
	// so convert milliseconds to nanoseconds for the second argument.
	dtime := time.Unix(0, int64(millis)*int64(time.Millisecond))
	return dtime
}

func (cxt *Encoder) WriteStringAmf3(s string) error {
	length := len(s)

	// TODO: Support outgoing string references.

	cxt.WriteUint29(uint32((length << 1) + 1))

	cxt.stream.Write([]byte(s))

	return nil
}

func (cxt *Encoder) WriteDateAmf3(v time.Time) error {
	// Convert time to milliseconds since Unix epoch
	milliseconds := v.UnixNano() / 1000000
	cxt.writeByte(AMF3Date) // Date marker
	cxt.WriteUint29(1)      // The U29 here is a flag (1 << 1) indicating that what follows is an inline value
	// Append the timestamp as a double (64-bit floating point)
	timestamp := make([]byte, 8)
	binary.BigEndian.PutUint64(timestamp, math.Float64bits(float64(milliseconds)))
	cxt.stream.Write(timestamp)
	return nil
}

func (cxt *Decoder) readObjectAmf3() interface{} {

	ref := cxt.ReadUint29()

	if cxt.errored() {
		return nil
	}

	// Check the low bit to see if this is a reference
	if (ref & 1) == 0 {
		index := int(ref >> 1)
		if index >= len(cxt.objectTable) {
			cxt.saveError(fmt.Errorf("invalid object index: %d", index))
			return nil
		}
		return cxt.objectTable[index]
	}

	class := cxt.readClassDefinitionAmf3(ref)

	object := AvmObject{}
	object.class = class

	// For an anonymous class, just return a map[string] interface{}
	if object.class.name == "" {
		result := make(map[string]interface{})
		for prop := range class.properties {
			value := cxt.ReadValueAmf3()
			object.staticFields[prop] = value
		}
		if class.dynamic {
			for {
				name := cxt.readStringAmf3()
				if name == "" {
					break
				}
				value := cxt.ReadValueAmf3()
				result[name] = value
			}
		}
		return result
	}

	object.dynamicFields = make(map[string]interface{})

	fmt.Printf("AvmObject class name: %s\n", class.name)

	// Store the object in the table before doing any decoding.
	cxt.storeObjectInTable(&object)

	// Read static fields
	object.staticFields = make([]interface{}, len(class.properties))
	for i := range class.properties {
		value := cxt.ReadValueAmf3()
		object.staticFields[i] = value
	}

	fmt.Printf("static fields = %v\n", object.staticFields)
	fmt.Printf("static fields = %v\n", class.properties)

	if class.dynamic {
		// Parse dynamic fields
		for {
			name := cxt.readStringAmf3()
			if name == "" {
				break
			}

			value := cxt.ReadValueAmf3()
			object.dynamicFields[name] = value
		}
	}

	// If this type is registered, then unpack this result into an instance of the type.
	// TODO: This could be faster if we didn't create an intermediate AvmObject.
	goType, foundGoType := cxt.typeMap[class.name]

	if foundGoType {
		result := reflect.Indirect(reflect.New(goType))
		for i := 0; i < len(class.properties); i++ {
			value := reflect.ValueOf(object.staticFields[i])
			fieldName := class.properties[i]
			// The Go type will have field names with capital letters
			fieldName = strings.ToUpper(fieldName[:1]) + fieldName[1:]
			field := result.FieldByName(fieldName)
			fmt.Printf("Attempting to write %v to field %v\n", object.staticFields[i],
				class.properties[i])
			field.Set(value)
		}
		return result.Interface()
	}

	return object
}

func (cxt *Encoder) writeDynamicObjectAmf3(obj map[string]interface{}) error {
	// Start with the AMF3 object marker and dynamic object marker.
	// The 0x0B marker indicates a dynamic object with no class definition.
	cxt.writeByte(AMF3Object)
	cxt.WriteUint29(0x0B)

	// Write an empty string for class name to indicate no class definition.
	// This is part of the AMF3 dynamic object specification.
	cxt.WriteStringAmf3("")

	// Iterate over the map to write dynamic properties.
	for key, value := range obj {
		// Write property name.
		cxt.WriteStringAmf3(key)

		// Write property value.
		// Convert the interface{} to reflect.Value and write using writeReflectedValueAmf3.
		if err := cxt.writeReflectedValueAmf3(reflect.ValueOf(value)); err != nil {
			return err // Handle errors from writing the value.
		}
	}

	// Write the end of dynamic properties marker.
	cxt.WriteUint29(0x01) // End of dynamic object.

	return nil
}

func (cxt *Encoder) writeObjectAmf3(value interface{}) error {

	fmt.Printf("writeObjectAmf3 attempting to write a value of type %s\n",
		reflect.ValueOf(value).Type().Name())

	return nil
}

func (cxt *Encoder) writeAvmObject3(value *AvmObject) error {
	// TODO: Support outgoing object references.

	// writeClassDefinitionAmf3 will also write the ref section.
	cxt.writeClassDefinitionAmf3(value.class)

	return nil
}

func (cxt *Encoder) writeReflectedStructAmf3(value reflect.Value) error {

	if value.Kind() != reflect.Struct {
		return fmt.Errorf("writeReflectedStructAmf3 called with non-struct value")
	}

	// Ref is, non-object-ref, non-class-ref, non-externalizable, non-dynamic
	// TODO: Support object refs and class refs.
	ref := 0x2

	numFields := value.Type().NumField()

	ref += numFields << 4

	cxt.WriteUint29(uint32(ref))

	// Class name
	cxt.WriteStringAmf3(value.Type().Name())
	fmt.Printf("wrote class name = %s\n", value.Type().Name())

	// Property names
	for i := 0; i < numFields; i++ {
		structField := value.Type().Field(i)
		cxt.WriteStringAmf3(structField.Name)
		fmt.Printf("wrote field name = %s\n", structField.Name)
	}

	// Property values
	for i := 0; i < numFields; i++ {
		cxt.writeReflectedValueAmf3(value.Field(i))
	}

	return nil
}

func (cxt *Decoder) readClassDefinitionAmf3(ref uint32) *AvmClass {
	// Check for a reference to an existing class definition
	if (ref & 2) == 0 {
		return cxt.classTable[int(ref>>2)]
	}

	// Parse a class definition
	className := cxt.readStringAmf3()

	externalizable := ref&4 != 0
	dynamic := ref&8 != 0
	propertyCount := ref >> 4

	class := AvmClass{className, externalizable, dynamic, make([]string, propertyCount)}

	// Property names
	for i := uint32(0); i < propertyCount; i++ {
		class.properties[i] = cxt.readStringAmf3()
	}

	// Save the new class in the loopup table
	cxt.classTable = append(cxt.classTable, &class)

	fmt.Printf("read class name = %s\n", class.name)

	return &class
}

func (cxt *Encoder) writeClassDefinitionAmf3(class *AvmClass) {
	// TODO: Support class references
	ref := uint32(0x2)

	if class.externalizable {
		ref += 0x4
	}
	if class.dynamic {
		ref += 0x8
	}

	ref += uint32(len(class.properties) << 4)

	cxt.WriteUint29(ref)

	cxt.WriteStringAmf3(class.name)

	// Property names
	for _, name := range class.properties {
		cxt.WriteStringAmf3(name)
	}
}

func (cxt *Decoder) readArrayAmf3() interface{} {
	ref := cxt.ReadUint29()

	if cxt.errored() {
		return nil
	}

	// Check the low bit to see if this is a reference
	if (ref & 1) == 0 {
		index := int(ref >> 1)
		if index >= len(cxt.objectTable) {
			cxt.saveError(fmt.Errorf("invalid array reference: %d", index))
			return nil
		}

		return cxt.objectTable[index]
	}

	elementCount := int(ref >> 1)

	// Read name-value pairs, if any.
	key := cxt.readStringAmf3()

	// No name-value pairs, return a flat Go array.
	if key == "" {
		result := make([]interface{}, elementCount)
		for i := 0; i < elementCount; i++ {
			result[i] = cxt.ReadValueAmf3()
		}
		return result
	}

	result := &AvmArray{}
	result.fields = make(map[string]interface{})

	// Store the object in the table before doing any decoding.
	cxt.storeObjectInTable(result)

	for key != "" {
		result.fields[key] = cxt.ReadValueAmf3()
		key = cxt.readStringAmf3()
	}

	// Read dense elements
	result.elements = make([]interface{}, elementCount)
	for i := 0; i < elementCount; i++ {
		result.elements[i] = cxt.ReadValueAmf3()
	}

	return result
}

func (cxt *Encoder) writeReflectedArrayAmf3(value reflect.Value) error {

	elementCount := value.Len()

	// TODO: Support outgoing array references
	ref := (elementCount << 1) + 1

	cxt.WriteUint29(uint32(ref))

	// Write an empty key since this is just a flat array.
	cxt.WriteStringAmf3("")

	for i := 0; i < elementCount; i++ {
		cxt.WriteValueAmf3(value.Index(i).Interface())
	}
	return nil
}

func (cxt *Encoder) writeFlatArrayAmf3(value []interface{}) error {
	elementCount := len(value)

	// TODO: Support outgoing array references
	ref := (elementCount << 1) + 1

	cxt.WriteUint29(uint32(ref))

	// Write an empty key since this is just a flat array.
	cxt.WriteStringAmf3("")

	// Write dense elements
	for i := 0; i < elementCount; i++ {
		cxt.WriteValueAmf3(value[i])
	}
	return nil
}

func (cxt *Encoder) writeMixedArray3(value *AvmArray) error {
	elementCount := len(value.elements)

	// TODO: Support outgoing array references
	ref := (elementCount << 1) + 1

	cxt.WriteUint29(uint32(ref))

	// Write fields
	for k, v := range value.fields {
		cxt.WriteStringAmf3(k)
		cxt.WriteValueAmf3(v)
	}

	// Write a null name to indicate the end of fields.
	cxt.WriteStringAmf3("")

	// Write dense elements
	for i := 0; i < elementCount; i++ {
		cxt.WriteValueAmf3(value.elements[i])
	}
	return nil
}

func (cxt *Decoder) ReadValue() interface{} {
	return cxt.ReadValueAmf3()
}

func (cxt *Decoder) ReadValueAmf3() interface{} {

	// Read type marker
	typeMarker := cxt.ReadByte()

	if cxt.errored() {
		return nil
	}

	switch typeMarker {
	case AMF3Null, AMF3Undefined:
		return nil
	case AMF3False:
		return false
	case AMF3True:
		return true
	case AMF3Integer:
		return cxt.ReadUint29()
	case AMF3Double:
		return cxt.ReadFloat64()
	case AMF3String:
		return cxt.readStringAmf3()
	case AMF3Externalizable:
		// TODO
	case AMF3Date:
		return cxt.readDateAmf3()
	case AMF3Object:
		return cxt.readObjectAmf3()
	case AMF3Dynamic:
		// TODO
	case AMF3ByteArray:
		return cxt.readByteArrayAmf3()
	case AMF3Array:
		return cxt.readArrayAmf3()
	}

	cxt.saveError(fmt.Errorf("AMF3 type marker was not supported"))
	return nil
}

func (cxt *Encoder) WriteValueAmf3(value interface{}) error {

	if value == nil {
		return cxt.writeByte(AMF3Null)
	}

	return cxt.writeReflectedValueAmf3(reflect.ValueOf(value))
}

func (cxt *Encoder) writeReflectedValueAmf3(value reflect.Value) error {
	// Get by Kind()
	switch value.Kind() {
	case reflect.String:
		cxt.writeByte(AMF3String)
		return cxt.WriteStringAmf3(value.String())
	case reflect.Bool:
		if value.Bool() {
			return cxt.writeByte(AMF3True)
		}
		return cxt.writeByte(AMF3False)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		cxt.writeByte(AMF3Integer)
		return cxt.WriteUint29(uint32(value.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		cxt.writeByte(AMF3Integer)
		return cxt.WriteUint29(uint32(value.Uint()))
	case reflect.Int64, reflect.Uint64:
		cxt.writeByte(AMF3Double)
		return cxt.WriteFloat64(float64(value.Int()))
	case reflect.Float32, reflect.Float64:
		cxt.writeByte(AMF3Double)
		return cxt.WriteFloat64(value.Float())
	case reflect.Array, reflect.Slice:
		// Specifically check for []byte to encode as AMF3 ByteArray
		if value.Type().Elem().Kind() == reflect.Uint8 {
			return cxt.writeByteArrayAmf3(value.Interface().([]uint8))
		} else {
			cxt.writeByte(AMF3Array)
			return cxt.writeReflectedArrayAmf3(value)
		}
	default:
		// Handle time.Time and map[string]interface{} types by type assertion
		switch v := value.Interface().(type) {
		case time.Time:
			return cxt.WriteDateAmf3(v)
		case map[string]interface{}:
			return cxt.writeDynamicObjectAmf3(v)
		default:
			return fmt.Errorf("writeReflectedValueAmf3 doesn't support kind %s or type: %s", value.Kind().String(), value.Type().String())
		}
	}
}
