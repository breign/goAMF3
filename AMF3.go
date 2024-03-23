package amf

type AMFVersion uint8

const (
	AMF3 AMFVersion = 0x3
)

const (
	amf3Null           byte = 0x01
	amf3False          byte = 0x02
	amf3True           byte = 0x03
	amf3Integer        byte = 0x04
	amf3Double         byte = 0x05
	amf3String         byte = 0x06
	amf3Externalizable byte = 0x07
	amf3Date           byte = 0x08
	amf3Array          byte = 0x09
	amf3Object         byte = 0x0a
	amf3Dynamic        byte = 0x0b
	amf3ByteArray      byte = 0x0c
	amf3VectorInt      byte = 0x0d
	amf3VectorUint     byte = 0x0d
	amf3VectorDouble   byte = 0x0d
	amf3VectorObject   byte = 0x0d
	amf3Dictionary     byte = 0x11
)
