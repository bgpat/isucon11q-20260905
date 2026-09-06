package main

import (
	stdjson "encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	gojson "github.com/goccy/go-json"
)

type jsonCompatibilityValue struct {
	Text       string            `json:"text"`
	When       time.Time         `json:"when"`
	NilSlice   []string          `json:"nil_slice"`
	EmptySlice []string          `json:"empty_slice"`
	NilMap     map[string]string `json:"nil_map"`
	EmptyMap   map[string]string `json:"empty_map"`
	Integer    int64             `json:"integer"`
	Float      float64           `json:"float"`
	Number     stdjson.Number    `json:"number"`
}

func TestGoJSONMarshalCompatibility(t *testing.T) {
	v := jsonCompatibilityValue{
		Text:       "<script>&日本語",
		When:       time.Date(2026, 9, 6, 15, 0, 0, 123456789, time.FixedZone("JST", 9*60*60)),
		EmptySlice: []string{},
		EmptyMap:   map[string]string{},
		Integer:    9007199254740991,
		Float:      1.25,
		Number:     stdjson.Number("12345678901234567890"),
	}
	std, stdErr := stdjson.Marshal(v)
	fast, fastErr := gojson.Marshal(v)
	if (stdErr != nil) != (fastErr != nil) {
		t.Fatalf("Marshal error mismatch: std=%v go-json=%v", stdErr, fastErr)
	}
	if !reflect.DeepEqual(std, fast) {
		t.Fatalf("Marshal bytes mismatch:\nstd: %s\ngo-json: %s", std, fast)
	}
}

func TestGoJSONUnmarshalCompatibility(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"normal", `{"target_base_url":"https://example.test/<x>&","isu_uuid":"00000000-0000-0000-0000-000000000001"}`},
		{"invalid", `{"target_base_url":`},
		{"number", `{"integer":9007199254740991,"float":1.25,"number":12345678901234567890}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var std JIAServiceRequest
			var fast JIAServiceRequest
			stdErr := stdjson.Unmarshal([]byte(tc.body), &std)
			fastErr := gojson.Unmarshal([]byte(tc.body), &fast)
			if (stdErr != nil) != (fastErr != nil) {
				t.Fatalf("Unmarshal error mismatch: std=%v go-json=%v", stdErr, fastErr)
			}
			if stdErr == nil && !reflect.DeepEqual(std, fast) {
				t.Fatalf("Unmarshal value mismatch: std=%#v go-json=%#v", std, fast)
			}
		})
	}
}

func TestGoJSONHTTPResponseCompatibility(t *testing.T) {
	v := jsonCompatibilityValue{Text: "<script>&", EmptySlice: []string{}, EmptyMap: map[string]string{}}
	std, _ := stdjson.Marshal(v)
	fast, _ := gojson.Marshal(v)
	for name, body := range map[string][]byte{"std": std, "go-json": fast} {
		r := httptest.NewRecorder()
		r.Header().Set("Content-Type", "application/json; charset=UTF-8")
		r.WriteHeader(http.StatusCreated)
		_, _ = r.Write(body)
		if r.Code != http.StatusCreated || r.Header().Get("Content-Type") != "application/json; charset=UTF-8" {
			t.Fatalf("%s HTTP status/header mismatch: status=%d header=%q", name, r.Code, r.Header().Get("Content-Type"))
		}
		if !reflect.DeepEqual(r.Body.Bytes(), std) {
			t.Fatalf("%s HTTP body mismatch: %s", name, r.Body.Bytes())
		}
	}
}
