package main

import (
	"bytes"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"testing"
)

func benchConditionList() []*GetIsuConditionResponse {
	out := make([]*GetIsuConditionResponse, conditionLimit)
	for i := range out {
		out[i] = &GetIsuConditionResponse{
			JIAIsuUUID:     "9e5c1109-beff-4598-b8f1-658d1994d55f",
			IsuName:        "isu <x>&日本語",
			Timestamp:      int64(1620000000 + i),
			IsSitting:      i%2 == 0,
			Condition:      "is_dirty=true,is_overweight=false,is_broken=false",
			ConditionLevel: "warning",
			Message:        "hello\nworld",
		}
	}
	return out
}

func benchGraphList() []GraphResponse {
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
	out := make([]GraphResponse, 24)
	for i := range out {
		start := int64(i) * 3600
		if i%3 == 0 {
			out[i] = GraphResponse{StartAt: start, EndAt: start + 3600, Data: nil, ConditionTimestamps: []int64{}}
			continue
		}
		out[i] = GraphResponse{StartAt: start, EndAt: start + 3600, Data: &dp, ConditionTimestamps: ts}
	}
	return out
}

func marshalPrettyJSONStd(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func benchmarkMarshalPretty(b *testing.B, v interface{}) {
	b.Helper()
	got, err := marshalPrettyJSON(v)
	if err != nil {
		b.Fatal(err)
	}
	want, err := marshalPrettyJSONStd(v)
	if err != nil {
		b.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		b.Fatalf("pretty mismatch vs encoding/json")
	}

	b.Run("typed_direct", func(b *testing.B) {
		b.ReportAllocs()
		switch x := v.(type) {
		case []*GetIsuConditionResponse:
			for i := 0; i < b.N; i++ {
				p := getPrettyJSONBuf()
				*p = append(appendPrettyConditionList(growBytes(*p, prettyConditionCap(x)), x), '\n')
				putPrettyJSONBuf(p)
			}
		case []GraphResponse:
			for i := 0; i < b.N; i++ {
				p := getPrettyJSONBuf()
				*p = append(appendPrettyGraphList(growBytes(*p, prettyGraphCap(x)), x), '\n')
				putPrettyJSONBuf(p)
			}
		default:
			b.Skip("no direct encoder")
		}
	})
	b.Run("std_indent", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := marshalPrettyJSONStd(v); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkMarshalPrettyJSON_condition20(b *testing.B) {
	benchmarkMarshalPretty(b, benchConditionList())
}

func BenchmarkMarshalPrettyJSON_graph24(b *testing.B) {
	benchmarkMarshalPretty(b, benchGraphList())
}

func BenchmarkAppendPrettyJSON(b *testing.B) {
	compact, err := jsonv2.Marshal(benchConditionList(), json.DefaultOptionsV1())
	if err != nil {
		b.Fatal(err)
	}
	b.Run("naive", func(b *testing.B) {
		b.ReportAllocs()
		dst := make([]byte, 0, len(compact)*2)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			dst = appendPrettyJSON(dst[:0], compact)
		}
	})
	b.Run("json_Indent", func(b *testing.B) {
		b.ReportAllocs()
		var buf bytes.Buffer
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf.Reset()
			if err := json.Indent(&buf, compact, "", "  "); err != nil {
				b.Fatal(err)
			}
		}
	})
}
