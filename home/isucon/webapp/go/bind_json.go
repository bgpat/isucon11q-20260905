package main

import (
	"bytes"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"io"
	"time"
	"unsafe"

	"github.com/labstack/echo/v4"
)

const postIsuConditionBindCap = 10

var (
	errBadJSON          = errors.New("bad request body")
	jsonLitTrue         = []byte("true")
	jsonLitFalse        = []byte("false")
	jsonLitNull         = []byte("null")
	jsonKeyIsSitting    = []byte("is_sitting")
	jsonKeyCondition    = []byte("condition")
	jsonKeyMessage      = []byte("message")
	jsonKeyTimestamp    = []byte("timestamp")
	jsonPrefixIsSitting = []byte(`{"is_sitting":`)
	jsonMidCondition    = []byte(`,"condition":`)
	jsonMidMessage      = []byte(`,"message":`)
	jsonMidTimestamp    = []byte(`,"timestamp":`)
)

// jsonV2Binder は Echo 既定の json.Decoder の代わりに body を一括読みする。
// POST /api/condition の配列はフィールドが固定なので型直読みする。
// エスケープ無し文字列は body のバッファをそのまま指す。
// それ以外は json/v2 Unmarshal。path/query の bind はしない。
type jsonV2Binder struct{}

func (jsonV2Binder) Bind(i interface{}, c echo.Context) error {
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	if len(body) == 0 {
		return nil
	}

	switch v := i.(type) {
	case *[]PostIsuConditionRequest:
		return parsePostIsuConditionJSON(body, v)
	default:
		return jsonv2.Unmarshal(body, i, json.DefaultOptionsV1())
	}
}

func internConditionCSV(s string) (string, bool) {
	dirty, overweight, broken, ok := parseConditionFlags(s)
	if !ok {
		return "", false
	}
	idx := 0
	if dirty {
		idx |= 1
	}
	if overweight {
		idx |= 2
	}
	if broken {
		idx |= 4
	}
	return packedConditionCSV[idx], true
}

func readPostIsuConditionRecords(r io.Reader) ([]conditionRecord, []time.Time, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil, errBadJSON
		}
		return nil, nil, err
	}
	if len(body) == 0 {
		return nil, nil, errBadJSON
	}
	records := make([]conditionRecord, 0, postIsuConditionBindCap)
	timestamps := make([]time.Time, 0, postIsuConditionBindCap)
	if err := parsePostIsuConditionRecords(body, &records, &timestamps); err != nil {
		return nil, nil, err
	}
	return records, timestamps, nil
}

func parsePostIsuConditionRecords(data []byte, records *[]conditionRecord, timestamps *[]time.Time) error {
	if records == nil || timestamps == nil {
		return errBadJSON
	}
	i := skipJSONWS(data, 0)
	if i >= len(data) || data[i] != '[' {
		return errBadJSON
	}
	i++
	if *records == nil {
		*records = make([]conditionRecord, 0, postIsuConditionBindCap)
	} else {
		*records = (*records)[:0]
	}
	if *timestamps == nil {
		*timestamps = make([]time.Time, 0, postIsuConditionBindCap)
	} else {
		*timestamps = (*timestamps)[:0]
	}
	i = skipJSONWS(data, i)
	if i < len(data) && data[i] == ']' {
		i = skipJSONWS(data, i+1)
		if i != len(data) {
			return errBadJSON
		}
		return nil
	}
	for {
		var rec conditionRecord
		var err error
		if next, ok := parsePostIsuConditionRecordFast(data, i, &rec); ok {
			i = next
		} else {
			i, err = parsePostIsuConditionRecord(data, i, &rec)
			if err != nil {
				return err
			}
		}
		*records = append(*records, rec)
		*timestamps = append(*timestamps, rec.Timestamp)
		if i < len(data) && data[i] == ',' {
			i++
			continue
		}
		if i < len(data) && data[i] == ']' {
			i = skipJSONWS(data, i+1)
			if i != len(data) {
				return errBadJSON
			}
			return nil
		}
		i = skipJSONWS(data, i)
		if i >= len(data) {
			return errBadJSON
		}
		if data[i] == ']' {
			i = skipJSONWS(data, i+1)
			if i != len(data) {
				return errBadJSON
			}
			return nil
		}
		if data[i] != ',' {
			return errBadJSON
		}
		i++
		i = skipJSONWS(data, i)
	}
}

func parsePostIsuConditionJSON(data []byte, dst *[]PostIsuConditionRequest) error {
	if dst == nil {
		return errBadJSON
	}
	i := skipJSONWS(data, 0)
	if i >= len(data) || data[i] != '[' {
		return errBadJSON
	}
	i++
	if *dst == nil {
		*dst = make([]PostIsuConditionRequest, 0, postIsuConditionBindCap)
	} else {
		*dst = (*dst)[:0]
	}
	i = skipJSONWS(data, i)
	if i < len(data) && data[i] == ']' {
		i = skipJSONWS(data, i+1)
		if i != len(data) {
			return errBadJSON
		}
		return nil
	}
	for {
		var item PostIsuConditionRequest
		var err error
		if next, ok := parsePostIsuConditionObjectFast(data, i, &item); ok {
			i = next
		} else {
			i, err = parsePostIsuConditionObject(data, i, &item)
			if err != nil {
				return err
			}
		}
		*dst = append(*dst, item)
		if i < len(data) && data[i] == ',' {
			i++
			continue
		}
		if i < len(data) && data[i] == ']' {
			i = skipJSONWS(data, i+1)
			if i != len(data) {
				return errBadJSON
			}
			return nil
		}
		i = skipJSONWS(data, i)
		if i >= len(data) {
			return errBadJSON
		}
		if data[i] == ']' {
			i = skipJSONWS(data, i+1)
			if i != len(data) {
				return errBadJSON
			}
			return nil
		}
		if data[i] != ',' {
			return errBadJSON
		}
		i++
		i = skipJSONWS(data, i)
	}
}

// parsePostIsuConditionObjectFast はベンチ典型の compact 並びだけを読む。
// {"is_sitting":bool,"condition":str,"message":str,"timestamp":int}
func parsePostIsuConditionObjectFast(data []byte, i int, item *PostIsuConditionRequest) (int, bool) {
	if len(data)-i < len(jsonPrefixIsSitting) || !bytes.Equal(data[i:i+len(jsonPrefixIsSitting)], jsonPrefixIsSitting) {
		return i, false
	}
	i += len(jsonPrefixIsSitting)
	switch {
	case i+5 <= len(data) && data[i] == 'f' && bytes.Equal(data[i:i+5], jsonLitFalse):
		item.IsSitting = false
		i += 5
	case i+4 <= len(data) && data[i] == 't' && bytes.Equal(data[i:i+4], jsonLitTrue):
		item.IsSitting = true
		i += 4
	default:
		return i, false
	}
	if len(data)-i < len(jsonMidCondition) || !bytes.Equal(data[i:i+len(jsonMidCondition)], jsonMidCondition) {
		return i, false
	}
	i += len(jsonMidCondition)
	var ok bool
	item.Condition, i, ok = parseJSONStringFast(data, i)
	if !ok {
		return i, false
	}
	if len(data)-i < len(jsonMidMessage) || !bytes.Equal(data[i:i+len(jsonMidMessage)], jsonMidMessage) {
		return i, false
	}
	i += len(jsonMidMessage)
	item.Message, i, ok = parseJSONStringFast(data, i)
	if !ok {
		return i, false
	}
	if len(data)-i < len(jsonMidTimestamp) || !bytes.Equal(data[i:i+len(jsonMidTimestamp)], jsonMidTimestamp) {
		return i, false
	}
	i += len(jsonMidTimestamp)
	var err error
	item.Timestamp, i, err = parseJSONInt64(data, i)
	if err != nil {
		return i, false
	}
	if i >= len(data) || data[i] != '}' {
		return i, false
	}
	return i + 1, true
}

func parseJSONStringFast(b []byte, i int) (string, int, bool) {
	if i >= len(b) || b[i] != '"' {
		return "", i, false
	}
	i++
	start := i
	for i < len(b) {
		c := b[i]
		if c == '"' {
			return bytesToString(b[start:i]), i + 1, true
		}
		if c == '\\' || c < 0x20 {
			return "", start - 1, false
		}
		i++
	}
	return "", i, false
}

func parsePostIsuConditionRecordFast(data []byte, i int, rec *conditionRecord) (int, bool) {
	var item PostIsuConditionRequest
	next, ok := parsePostIsuConditionObjectFast(data, i, &item)
	if !ok {
		return i, false
	}
	cond, ok := internConditionCSV(item.Condition)
	if !ok {
		return i, false
	}
	rec.IsSitting = item.IsSitting
	rec.Condition = cond
	rec.Message = item.Message
	rec.Timestamp = time.Unix(item.Timestamp, 0)
	return next, true
}

func parsePostIsuConditionRecord(data []byte, i int, rec *conditionRecord) (int, error) {
	var item PostIsuConditionRequest
	next, err := parsePostIsuConditionObject(data, i, &item)
	if err != nil {
		return next, err
	}
	cond, ok := internConditionCSV(item.Condition)
	if !ok {
		return next, errBadJSON
	}
	rec.IsSitting = item.IsSitting
	rec.Condition = cond
	rec.Message = item.Message
	rec.Timestamp = time.Unix(item.Timestamp, 0)
	return next, nil
}

func parsePostIsuConditionObject(data []byte, i int, item *PostIsuConditionRequest) (int, error) {
	i = skipJSONWS(data, i)
	if i >= len(data) || data[i] != '{' {
		return i, errBadJSON
	}
	i++
	i = skipJSONWS(data, i)
	if i < len(data) && data[i] == '}' {
		return i + 1, nil
	}
	for {
		key, next, err := parseJSONRawString(data, i)
		if err != nil {
			return next, err
		}
		i = skipJSONWS(data, next)
		if i >= len(data) || data[i] != ':' {
			return i, errBadJSON
		}
		i++
		i = skipJSONWS(data, i)
		switch len(key) {
		case 10:
			if bytes.Equal(key, jsonKeyIsSitting) {
				item.IsSitting, i, err = parseJSONBool(data, i)
				break
			}
			i, err = skipJSONValue(data, i)
		case 9:
			if key[0] == 'c' && bytes.Equal(key, jsonKeyCondition) {
				item.Condition, i, err = parseJSONStringOrNull(data, i)
				break
			}
			if key[0] == 't' && bytes.Equal(key, jsonKeyTimestamp) {
				item.Timestamp, i, err = parseJSONInt64(data, i)
				break
			}
			i, err = skipJSONValue(data, i)
		case 7:
			if bytes.Equal(key, jsonKeyMessage) {
				item.Message, i, err = parseJSONStringOrNull(data, i)
				break
			}
			i, err = skipJSONValue(data, i)
		default:
			i, err = skipJSONValue(data, i)
		}
		if err != nil {
			return i, err
		}
		i = skipJSONWS(data, i)
		if i >= len(data) {
			return i, errBadJSON
		}
		if data[i] == '}' {
			return i + 1, nil
		}
		if data[i] != ',' {
			return i, errBadJSON
		}
		i++
		i = skipJSONWS(data, i)
	}
}

func skipJSONWS(b []byte, i int) int {
	for i < len(b) {
		switch b[i] {
		case ' ', '\n', '\r', '\t':
			i++
		default:
			return i
		}
	}
	return i
}

func parseJSONStringOrNull(b []byte, i int) (string, int, error) {
	if i+4 <= len(b) && bytes.Equal(b[i:i+4], jsonLitNull) {
		return "", i + 4, nil
	}
	return parseJSONString(b, i)
}

func bytesToString(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}

func parseJSONString(b []byte, i int) (string, int, error) {
	raw, next, err := parseJSONRawString(b, i)
	if err != nil {
		return "", next, err
	}
	return bytesToString(raw), next, nil
}

func parseJSONRawString(b []byte, i int) ([]byte, int, error) {
	if i >= len(b) || b[i] != '"' {
		return nil, i, errBadJSON
	}
	start := i
	i++
	escaped := false
	for i < len(b) {
		c := b[i]
		if c == '\\' {
			escaped = true
			i++
			if i >= len(b) {
				return nil, i, errBadJSON
			}
			i++
			continue
		}
		if c == '"' {
			raw := b[start : i+1]
			i++
			if !escaped {
				return raw[1 : len(raw)-1], i, nil
			}
			var s string
			if err := json.Unmarshal(raw, &s); err != nil {
				return nil, i, err
			}
			return []byte(s), i, nil
		}
		if c < 0x20 {
			return nil, i, errBadJSON
		}
		i++
	}
	return nil, i, errBadJSON
}

func parseJSONBool(b []byte, i int) (bool, int, error) {
	if i+4 <= len(b) && bytes.Equal(b[i:i+4], jsonLitTrue) {
		return true, i + 4, nil
	}
	if i+5 <= len(b) && bytes.Equal(b[i:i+5], jsonLitFalse) {
		return false, i + 5, nil
	}
	return false, i, errBadJSON
}

func parseJSONInt64(b []byte, i int) (int64, int, error) {
	if i >= len(b) {
		return 0, i, errBadJSON
	}
	neg := false
	if b[i] == '-' {
		neg = true
		i++
	}
	if i >= len(b) || b[i] < '0' || b[i] > '9' {
		return 0, i, errBadJSON
	}
	if b[i] == '0' && i+1 < len(b) && b[i+1] >= '0' && b[i+1] <= '9' {
		return 0, i, errBadJSON
	}
	var n uint64
	for i < len(b) && b[i] >= '0' && b[i] <= '9' {
		d := uint64(b[i] - '0')
		if n > (1<<64-1)/10 || (n == (1<<64-1)/10 && d > 9) {
			return 0, i, errBadJSON
		}
		n = n*10 + d
		i++
	}
	if i < len(b) && (b[i] == '.' || b[i] == 'e' || b[i] == 'E') {
		return 0, i, errBadJSON
	}
	if neg {
		if n > 1<<63 {
			return 0, i, errBadJSON
		}
		return -int64(n), i, nil
	}
	if n > 1<<63-1 {
		return 0, i, errBadJSON
	}
	return int64(n), i, nil
}

func skipJSONValue(b []byte, i int) (int, error) {
	if i >= len(b) {
		return i, errBadJSON
	}
	switch b[i] {
	case '"':
		_, next, err := parseJSONRawString(b, i)
		return next, err
	case 't':
		if i+4 <= len(b) && bytes.Equal(b[i:i+4], jsonLitTrue) {
			return i + 4, nil
		}
	case 'f':
		if i+5 <= len(b) && bytes.Equal(b[i:i+5], jsonLitFalse) {
			return i + 5, nil
		}
	case 'n':
		if i+4 <= len(b) && bytes.Equal(b[i:i+4], jsonLitNull) {
			return i + 4, nil
		}
	case '{':
		i++
		i = skipJSONWS(b, i)
		if i < len(b) && b[i] == '}' {
			return i + 1, nil
		}
		for {
			_, next, err := parseJSONRawString(b, i)
			if err != nil {
				return next, err
			}
			i = skipJSONWS(b, next)
			if i >= len(b) || b[i] != ':' {
				return i, errBadJSON
			}
			i++
			i = skipJSONWS(b, i)
			i, err = skipJSONValue(b, i)
			if err != nil {
				return i, err
			}
			i = skipJSONWS(b, i)
			if i >= len(b) {
				return i, errBadJSON
			}
			if b[i] == '}' {
				return i + 1, nil
			}
			if b[i] != ',' {
				return i, errBadJSON
			}
			i++
			i = skipJSONWS(b, i)
		}
	case '[':
		i++
		i = skipJSONWS(b, i)
		if i < len(b) && b[i] == ']' {
			return i + 1, nil
		}
		for {
			var err error
			i, err = skipJSONValue(b, i)
			if err != nil {
				return i, err
			}
			i = skipJSONWS(b, i)
			if i >= len(b) {
				return i, errBadJSON
			}
			if b[i] == ']' {
				return i + 1, nil
			}
			if b[i] != ',' {
				return i, errBadJSON
			}
			i++
			i = skipJSONWS(b, i)
		}
	default:
		if b[i] == '-' || (b[i] >= '0' && b[i] <= '9') {
			return skipJSONNumber(b, i)
		}
	}
	return i, errBadJSON
}

func skipJSONNumber(b []byte, i int) (int, error) {
	if i < len(b) && b[i] == '-' {
		i++
	}
	if i >= len(b) || b[i] < '0' || b[i] > '9' {
		return i, errBadJSON
	}
	if b[i] == '0' {
		i++
	} else {
		for i < len(b) && b[i] >= '0' && b[i] <= '9' {
			i++
		}
	}
	if i < len(b) && b[i] == '.' {
		i++
		if i >= len(b) || b[i] < '0' || b[i] > '9' {
			return i, errBadJSON
		}
		for i < len(b) && b[i] >= '0' && b[i] <= '9' {
			i++
		}
	}
	if i < len(b) && (b[i] == 'e' || b[i] == 'E') {
		i++
		if i < len(b) && (b[i] == '+' || b[i] == '-') {
			i++
		}
		if i >= len(b) || b[i] < '0' || b[i] > '9' {
			return i, errBadJSON
		}
		for i < len(b) && b[i] >= '0' && b[i] <= '9' {
			i++
		}
	}
	return i, nil
}
