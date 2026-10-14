package youtube

import (
	"io"

	"google.golang.org/protobuf/encoding/protowire"
)

type wireField struct {
	start, end, payloadStart int
	num                      protowire.Number
	typ                      protowire.Type
}

func wireFields(data []byte) ([]wireField, bool) {
	var fields []wireField
	for pos := 0; pos < len(data); {
		start := pos
		num, typ, n := protowire.ConsumeTag(data[pos:])
		if n < 0 || num < 1 || num > (1<<29)-1 {
			return nil, false
		}
		pos += n
		size := protowire.ConsumeFieldValue(num, typ, data[pos:])
		if size < 0 {
			return nil, false
		}
		f := wireField{start: start, end: pos + size, num: num, typ: typ}
		if typ == protowire.BytesType {
			payload, consumed := protowire.ConsumeBytes(data[pos:])
			if consumed < 0 {
				return nil, false
			}
			f.payloadStart = pos + consumed - len(payload)
		}
		fields = append(fields, f)
		pos += size
	}
	return fields, true
}

type replayBody struct {
	io.Reader
	io.Closer
}
