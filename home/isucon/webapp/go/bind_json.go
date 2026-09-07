package main

import (
	"bytes"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"io"
	"sync"

	"github.com/labstack/echo/v4"
)

const postIsuConditionBindCap = 10

var bindJSONBufPool = sync.Pool{
	New: func() interface{} {
		return bytes.NewBuffer(make([]byte, 0, 4096))
	},
}

// jsonV2Binder は Echo 既定の json.Decoder の代わりに、body を一括読みして json/v2 で Unmarshal する。
// path/query の bind はしない。JSON body だけを Bind するハンドラ向け。
type jsonV2Binder struct{}

func (jsonV2Binder) Bind(i interface{}, c echo.Context) error {
	buf := bindJSONBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bindJSONBufPool.Put(buf)

	if _, err := buf.ReadFrom(c.Request().Body); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	if buf.Len() == 0 {
		return nil
	}
	return jsonv2.Unmarshal(buf.Bytes(), i, json.DefaultOptionsV1())
}
