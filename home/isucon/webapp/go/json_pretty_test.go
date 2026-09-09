package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestTypedPrettyJSONMatchesStd(t *testing.T) {
	cond := benchConditionList()
	condFallback := []*GetIsuConditionResponse{{
		JIAIsuUUID:     "9e5c1109-beff-4598-b8f1-658d1994d55f",
		IsuName:        "isu <x>&",
		Timestamp:      1,
		IsSitting:      true,
		Condition:      "is_dirty=true,is_overweight=false,is_broken=false",
		ConditionLevel: "weird<level>",
		Message:        "hello\nworld",
	}}
	graph := benchGraphList()
	init := InitializeResponse{Language: "go"}
	me := GetMeResponse{JIAUserID: "user <id>&"}
	isu := Isu{ID: 1, JIAIsuUUID: "9e5c1109-beff-4598-b8f1-658d1994d55f", Name: "n<x>", Character: "c&"}

	cases := []struct {
		name string
		v    interface{}
		got  []byte
	}{
		{"condition20", cond, append(appendPrettyConditionList(nil, cond), '\n')},
		{"graph24", graph, marshalPrettyGraph(graph)},
		{"condition_level_fallback", condFallback, append(appendPrettyConditionList(nil, condFallback), '\n')},
		{"initialize", init, append(appendPrettyInitialize(nil, init), '\n')},
		{"me", me, append(appendPrettyMe(nil, me), '\n')},
		{"isu", isu, append(appendPrettyIsu(nil, isu), '\n')},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			std, err := marshalPrettyJSONStd(tc.v)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(tc.got, std) {
				n := 120
				if len(tc.got) < n {
					n = len(tc.got)
				}
				t.Fatalf("Echo/std pretty mismatch len std=%d got=%d prefix=%q", len(std), len(tc.got), tc.got[:n])
			}
		})
	}
}

func TestWritePrettyJSONMatchesStd(t *testing.T) {
	list := benchConditionList()
	std, err := marshalPrettyJSONStd(list)
	if err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	if err := writePrettyJSON(e.NewContext(req, rec), http.StatusOK, list, prettyConditionCap(list), appendPrettyConditionList); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rec.Body.Bytes(), std) {
		t.Fatalf("writePrettyJSON mismatch")
	}
}
