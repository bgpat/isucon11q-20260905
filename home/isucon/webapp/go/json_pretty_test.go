package main

import (
	"bytes"
	stdjson "encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestAppendPrettyJSONMatchesStdIndent(t *testing.T) {
	compacts := []string{
		`null`,
		`true`,
		`false`,
		`0`,
		`-1.25`,
		`""`,
		`"hello"`,
		`"a\"b\\c"`,
		`"<>&"`,
		`"{\",\":\"}"`,
		`[]`,
		`{}`,
		`[1]`,
		`[1,2,3]`,
		`[[]]`,
		`[{}]`,
		`{"a":1}`,
		`{"a":[]}`,
		`{"a":{}}`,
		`{"a":{"b":1},"c":[2,3]}`,
		`[{"jia_isu_uuid":"u","isu_name":"isu <x>&","timestamp":1,"is_sitting":true,"condition":"is_dirty=true","condition_level":"warning","message":"hello\nworld"}]`,
	}
	for _, compact := range compacts {
		var std bytes.Buffer
		if err := stdjson.Indent(&std, []byte(compact), "", "  "); err != nil {
			t.Fatalf("json.Indent %s: %v", compact, err)
		}
		got := appendPrettyJSON(nil, []byte(compact))
		if !bytes.Equal(std.Bytes(), got) {
			t.Fatalf("indent mismatch\nsrc: %s\nstd: %q\ngot: %q", compact, std.Bytes(), got)
		}
	}
}

func TestMarshalPrettyJSONMatchesBaselines(t *testing.T) {
	payloads := []struct {
		name string
		v    interface{}
	}{
		{"condition20", benchConditionList()},
		{"graph24", benchGraphList()},
		{"condition_level_fallback", []*GetIsuConditionResponse{{
			JIAIsuUUID:     "9e5c1109-beff-4598-b8f1-658d1994d55f",
			IsuName:        "isu <x>&",
			Timestamp:      1,
			IsSitting:      true,
			Condition:      "is_dirty=true,is_overweight=false,is_broken=false",
			ConditionLevel: "weird<level>",
			Message:        "hello\nworld",
		}}},
		{"initialize", InitializeResponse{Language: "go"}},
		{"me", GetMeResponse{JIAUserID: "user <id>&"}},
		{"isu", Isu{ID: 1, JIAIsuUUID: "9e5c1109-beff-4598-b8f1-658d1994d55f", Name: "n<x>", Character: "c&"}},
	}
	for _, payload := range payloads {
		std, err := marshalPrettyJSONStd(payload.v)
		if err != nil {
			t.Fatal(err)
		}
		got, err := marshalPrettyJSON(payload.v)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, std) {
			n := 120
			if len(got) < n {
				n = len(got)
			}
			t.Fatalf("%s: Echo/std pretty mismatch len std=%d got=%d prefix=%q", payload.name, len(std), len(got), got[:n])
		}
	}
}

func TestMarshalPrettyGraphMatchesTyped(t *testing.T) {
	list := benchGraphList()
	got := marshalPrettyGraph(list)
	want, err := marshalPrettyJSON(list)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("marshalPrettyGraph mismatch")
	}
}

func TestWritePrettyJSONMatchesWriteJSON(t *testing.T) {
	list := benchConditionList()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	recIface := httptest.NewRecorder()
	if err := writeJSON(e.NewContext(req, recIface), http.StatusOK, list); err != nil {
		t.Fatal(err)
	}
	recTyped := httptest.NewRecorder()
	if err := writePrettyJSON(e.NewContext(req, recTyped), http.StatusOK, list, prettyConditionCap(list), appendPrettyConditionList); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(recIface.Body.Bytes(), recTyped.Body.Bytes()) {
		t.Fatalf("typed write mismatch")
	}
}
