package main

import (
	"bytes"
	stdjson "encoding/json"
	jsonv2 "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	gojson "github.com/goccy/go-json"
	"github.com/labstack/echo/v4"
)

func typicalPostIsuConditionBody(n int) []byte {
	var b strings.Builder
	b.WriteByte('[')
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"is_sitting":false,"condition":"is_dirty=false,is_overweight=false,is_broken=false","message":"ISUが疲れたみたいです","timestamp":1624373760}`)
	}
	b.WriteByte(']')
	return []byte(b.String())
}

func newJSONContext(method, path string, body []byte) echo.Context {
	e := echo.New()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("jia_isu_uuid")
	c.SetParamValues("00000000-0000-0000-0000-000000000001")
	return c
}

func TestJSONV2BinderMatchesStdUnmarshal(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"typical10", string(typicalPostIsuConditionBody(10))},
		{"one", `[{"is_sitting":true,"condition":"is_dirty=true,is_overweight=false,is_broken=false","message":"hello","timestamp":1}]`},
		{"pretty", "[\n  {\n    \"timestamp\": 2,\n    \"message\": \"x\",\n    \"condition\": \"is_dirty=false,is_overweight=false,is_broken=false\",\n    \"is_sitting\": true\n  }\n]\n"},
		{"escaped", `[{"is_sitting":false,"condition":"is_dirty=false,is_overweight=false,is_broken=false","message":"say \"hi\"\nnext","timestamp":3}]`},
		{"extra-field", `[{"is_sitting":false,"condition":"is_dirty=false,is_overweight=false,is_broken=false","message":"m","timestamp":4,"extra":{"n":1}}]`},
		{"empty", `[]`},
		{"unicode", `[{"is_sitting":false,"condition":"is_dirty=false,is_overweight=false,is_broken=false","message":"日本語<>&","timestamp":5}]`},
		{"null-message", `[{"is_sitting":false,"condition":"is_dirty=false,is_overweight=false,is_broken=false","message":null,"timestamp":6}]`},
		{"missing-fields", `[{"timestamp":7}]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var std []PostIsuConditionRequest
			stdErr := stdjson.Unmarshal([]byte(tc.body), &std)
			got := make([]PostIsuConditionRequest, 0, 10)
			err := (jsonV2Binder{}).Bind(&got, newJSONContext(http.MethodPost, "/api/condition/u", []byte(tc.body)))
			if (stdErr != nil) != (err != nil) {
				t.Fatalf("error mismatch: std=%v bind=%v", stdErr, err)
			}
			if stdErr != nil {
				return
			}
			if !reflect.DeepEqual(std, got) {
				t.Fatalf("value mismatch\nstd=%#v\nbind=%#v", std, got)
			}
		})
	}
}

func TestJSONV2BinderInitializeRequest(t *testing.T) {
	body := []byte(`{"jia_service_url":"http://example.test:5000"}`)
	var std InitializeRequest
	if err := stdjson.Unmarshal(body, &std); err != nil {
		t.Fatal(err)
	}
	var got InitializeRequest
	if err := (jsonV2Binder{}).Bind(&got, newJSONContext(http.MethodPost, "/initialize", body)); err != nil {
		t.Fatal(err)
	}
	if std != got {
		t.Fatalf("std=%#v bind=%#v", std, got)
	}
}

func TestJSONV2BinderRejectsInvalid(t *testing.T) {
	cases := []string{
		`{`,
		`[{"is_sitting":true}`,
		`[{"timestamp":1.5}]`,
		`[{"timestamp":01}]`,
		`not-json`,
	}
	for _, body := range cases {
		var req []PostIsuConditionRequest
		if err := (jsonV2Binder{}).Bind(&req, newJSONContext(http.MethodPost, "/api/condition/u", []byte(body))); err == nil {
			t.Fatalf("expected error for %q", body)
		}
	}
}

func TestJSONV2BinderMatchesEchoDefaultBinder(t *testing.T) {
	body := typicalPostIsuConditionBody(10)
	var echoReq []PostIsuConditionRequest
	if err := echo.New().Binder.Bind(&echoReq, newJSONContext(http.MethodPost, "/api/condition/u", body)); err != nil {
		t.Fatal(err)
	}
	var fastReq []PostIsuConditionRequest
	if err := (jsonV2Binder{}).Bind(&fastReq, newJSONContext(http.MethodPost, "/api/condition/u", body)); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(echoReq, fastReq) {
		t.Fatalf("echo=%#v\nfast=%#v", echoReq, fastReq)
	}
}

func TestJSONV2BinderSurvivesBufferPoolReuse(t *testing.T) {
	body1 := typicalPostIsuConditionBody(10)
	var req1 []PostIsuConditionRequest
	if err := (jsonV2Binder{}).Bind(&req1, newJSONContext(http.MethodPost, "/api/condition/u", body1)); err != nil {
		t.Fatal(err)
	}
	want := req1[0].Message
	body2 := []byte(`[{"is_sitting":true,"condition":"is_dirty=true,is_overweight=true,is_broken=true","message":"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx","timestamp":9}]`)
	var req2 []PostIsuConditionRequest
	if err := (jsonV2Binder{}).Bind(&req2, newJSONContext(http.MethodPost, "/api/condition/u", body2)); err != nil {
		t.Fatal(err)
	}
	if req1[0].Message != want {
		t.Fatalf("pool reuse corrupted first bind: got %q want %q", req1[0].Message, want)
	}
}

func BenchmarkEchoDefaultBind(b *testing.B) {
	body := typicalPostIsuConditionBody(10)
	binder := echo.New().Binder
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var req []PostIsuConditionRequest
		if err := binder.Bind(&req, newJSONContext(http.MethodPost, "/api/condition/u", body)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONV2Bind(b *testing.B) {
	body := typicalPostIsuConditionBody(10)
	binder := jsonV2Binder{}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := make([]PostIsuConditionRequest, 0, postIsuConditionBindCap)
		if err := binder.Bind(&req, newJSONContext(http.MethodPost, "/api/condition/u", body)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStdJSONDecoderCondition(b *testing.B) {
	body := typicalPostIsuConditionBody(10)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := make([]PostIsuConditionRequest, 0, postIsuConditionBindCap)
		if err := stdjson.NewDecoder(bytes.NewReader(body)).Decode(&req); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONV2UnmarshalCondition(b *testing.B) {
	body := typicalPostIsuConditionBody(10)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := make([]PostIsuConditionRequest, 0, postIsuConditionBindCap)
		if err := jsonv2.Unmarshal(body, &req, stdjson.DefaultOptionsV1()); err != nil {
			b.Fatal(err)
		}
	}
}

func TestParsePostIsuConditionRecordsTypical(t *testing.T) {
	body := typicalPostIsuConditionBody(10)
	var std []PostIsuConditionRequest
	if err := stdjson.Unmarshal(body, &std); err != nil {
		t.Fatal(err)
	}
	records := make([]conditionRecord, 0, 10)
	timestamps := make([]time.Time, 0, 10)
	if err := parsePostIsuConditionRecords(body, &records, &timestamps); err != nil {
		t.Fatal(err)
	}
	if len(records) != len(std) {
		t.Fatalf("len records=%d std=%d", len(records), len(std))
	}
	for i := range std {
		if records[i].IsSitting != std[i].IsSitting || records[i].Message != std[i].Message || records[i].Timestamp.Unix() != std[i].Timestamp {
			t.Fatalf("i=%d rec=%#v std=%#v", i, records[i], std[i])
		}
		if records[i].Condition != packedConditionCSV[0] {
			t.Fatalf("condition not interned: %q", records[i].Condition)
		}
		if timestamps[i] != records[i].Timestamp {
			t.Fatalf("timestamp mismatch")
		}
	}
}

func TestReadPostIsuConditionRecordsRejectsInvalidCondition(t *testing.T) {
	body := []byte(`[{"is_sitting":false,"condition":"nope","message":"m","timestamp":1}]`)
	if _, _, err := readPostIsuConditionRecords(bytes.NewReader(body)); err == nil {
		t.Fatal("expected error")
	}
}

func BenchmarkTypedParseCondition(b *testing.B) {
	body := typicalPostIsuConditionBody(10)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := make([]PostIsuConditionRequest, 0, postIsuConditionBindCap)
		if err := parsePostIsuConditionJSON(body, &req); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGoJSONUnmarshalCondition(b *testing.B) {
	body := typicalPostIsuConditionBody(10)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := make([]PostIsuConditionRequest, 0, postIsuConditionBindCap)
		if err := gojson.Unmarshal(body, &req); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTypedParseConditionRecords(b *testing.B) {
	body := typicalPostIsuConditionBody(10)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		records := make([]conditionRecord, 0, postIsuConditionBindCap)
		timestamps := make([]time.Time, 0, postIsuConditionBindCap)
		if err := parsePostIsuConditionRecords(body, &records, &timestamps); err != nil {
			b.Fatal(err)
		}
	}
}
