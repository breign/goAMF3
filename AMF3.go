package amf

type AMFVersion uint8

const (
	AMF3 AMFVersion = 0x3
)

const (
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
