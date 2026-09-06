package main

import (
	"bytes"
	stdjson "encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	gojson "github.com/goccy/go-json"
	"github.com/labstack/echo/v4"
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

func TestWriteJSONMatchesEchoPretty(t *testing.T) {
	ts := make([]int64, 8)
	for i := range ts {
		ts[i] = int64(1000 + i)
	}
	dp := GraphDataPoint{
		Score: 80,
		Percentage: ConditionsPercentage{
			Sitting:      10,
			IsBroken:     0,
			IsDirty:      20,
			IsOverweight: 0,
		},
	}
	samples := []struct {
		name string
		code int
		v    interface{}
	}{
		{"initialize", http.StatusOK, InitializeResponse{Language: "go"}},
		{"me", http.StatusOK, GetMeResponse{JIAUserID: "user1"}},
		{"graph", http.StatusOK, []GraphResponse{
			{StartAt: 0, EndAt: 3600, Data: nil, ConditionTimestamps: []int64{}},
			{StartAt: 3600, EndAt: 7200, Data: &dp, ConditionTimestamps: ts},
		}},
		{"condition", http.StatusOK, []*GetIsuConditionResponse{{
			JIAIsuUUID:     "u",
			IsuName:        "isu <x>&",
			Timestamp:      1,
			IsSitting:      true,
			Condition:      "is_dirty=true,is_overweight=false,is_broken=false",
			ConditionLevel: "warning",
			Message:        "hello\nworld",
		}}},
		{"created-empty-maps", http.StatusCreated, jsonCompatibilityValue{
			Text:       "<script>&日本語",
			When:       time.Date(2026, 9, 6, 15, 0, 0, 123456789, time.FixedZone("JST", 9*60*60)),
			EmptySlice: []string{},
			EmptyMap:   map[string]string{},
			Integer:    9007199254740991,
			Float:      1.25,
			Number:     stdjson.Number("12345678901234567890"),
		}},
		{"nil-slice-map", http.StatusOK, struct {
			N *int           `json:"n"`
			S []int          `json:"s"`
			M map[string]int `json:"m"`
		}{}},
	}

	e := echo.New()
	e.Debug = true
	for _, tc := range samples {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			recEcho := httptest.NewRecorder()
			recFast := httptest.NewRecorder()
			if err := e.NewContext(req, recEcho).JSON(tc.code, tc.v); err != nil {
				t.Fatalf("echo JSON: %v", err)
			}
			if err := writeJSON(e.NewContext(req, recFast), tc.code, tc.v); err != nil {
				t.Fatalf("writeJSON: %v", err)
			}
			if recEcho.Code != recFast.Code {
				t.Fatalf("status echo=%d writeJSON=%d", recEcho.Code, recFast.Code)
			}
			if got, want := recFast.Header().Get(echo.HeaderContentType), recEcho.Header().Get(echo.HeaderContentType); got != want {
				t.Fatalf("content-type echo=%q writeJSON=%q", want, got)
			}
			if !bytes.Equal(recEcho.Body.Bytes(), recFast.Body.Bytes()) {
				t.Fatalf("body mismatch\necho: %s\nwriteJSON: %s", recEcho.Body.Bytes(), recFast.Body.Bytes())
			}
		})
	}
}
