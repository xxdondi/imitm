package main

import "bytes"

// Constants from protobuf source
// https://github.com/protocolbuffers/protobuf/blob/b106bb2abc5b1551dfd42b9c770c363935a77c62/python/google/protobuf/internal/wire_format.py#L26
const PB_WIRETYPE_LENGTH_DELIMITED = 2

// https://github.com/protocolbuffers/protobuf/blob/main/python/google/protobuf/internal/wire_format.py#L67
const PB_TAG_TYPE_BITS = 3

type FieldKey struct {
	Tag          []byte
	CorruptedTag []byte
	Key          int
	Description_ string
}

type FieldKeyCursor struct {
	Body     *[]byte
	Pos      int
	FieldKey *FieldKey
}

func fieldKeyToTag(fieldKey int) []byte {
	// Pack the field key like protobuf does
	packedTag := (fieldKey << PB_TAG_TYPE_BITS) | PB_WIRETYPE_LENGTH_DELIMITED
	// Now encode it as a varint
	var encoded []byte
	mask := byte(0x7F)
	for {
		b := byte(packedTag) & mask
		packedTag >>= 7
		if packedTag > 0 {
			b |= 0x80
		}
		encoded = append(encoded, b)
		if packedTag == 0 {
			break
		}
	}
	return encoded
}

func NewFieldKey(key int, description string) *FieldKey {
	return &FieldKey{
		fieldKeyToTag(key),
		fieldKeyToTag(key - 1),
		key,
		description,
	}
}

func FindFieldKey(body []byte, fieldKey *FieldKey) *FieldKeyCursor {
	idx := bytes.Index(body, fieldKey.Tag)
	if idx < 0 {
		return nil
	}
	return &FieldKeyCursor{&body, idx, fieldKey}
}

func (cursor *FieldKeyCursor) Next() *FieldKeyCursor {
	idx := bytes.Index((*cursor.Body)[cursor.Pos+1:], cursor.FieldKey.Tag)
	if idx < 0 {
		return nil
	}
	cursor.Pos = cursor.Pos + idx + 1
	return cursor
}

func (cursor *FieldKeyCursor) ReplaceAllAfter(limit int) *FieldKeyCursor {
	modifiedBody := bytes.ReplaceAll((*cursor.Body)[cursor.Pos:], cursor.FieldKey.Tag, cursor.FieldKey.CorruptedTag)
	// Copy back into the original body
	copy((*cursor.Body)[cursor.Pos:], modifiedBody)
	return cursor
}
