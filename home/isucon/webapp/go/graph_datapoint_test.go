package main

import (
	"fmt"
	"strings"
	"testing"
)

func calculateGraphDataPointLegacy(isuConditions []IsuCondition) (GraphDataPoint, error) {
	conditionsCount := map[string]int{"is_broken": 0, "is_dirty": 0, "is_overweight": 0}
	rawScore := 0
	for _, condition := range isuConditions {
		badConditionsCount := 0

		if !isValidConditionFormatLegacy(condition.Condition) {
			return GraphDataPoint{}, fmt.Errorf("invalid condition format")
		}

		for _, condStr := range strings.Split(condition.Condition, ",") {
			keyValue := strings.Split(condStr, "=")
			if keyValue[1] == "true" {
				conditionsCount[keyValue[0]] += 1
				badConditionsCount++
			}
		}

		if badConditionsCount >= 3 {
			rawScore += scoreConditionLevelCritical
		} else if badConditionsCount >= 1 {
			rawScore += scoreConditionLevelWarning
		} else {
			rawScore += scoreConditionLevelInfo
		}
	}

	sittingCount := 0
	for _, condition := range isuConditions {
		if condition.IsSitting {
			sittingCount++
		}
	}

	n := len(isuConditions)
	return GraphDataPoint{
		Score: rawScore * 100 / 3 / n,
		Percentage: ConditionsPercentage{
			Sitting:      sittingCount * 100 / n,
			IsBroken:     conditionsCount["is_broken"] * 100 / n,
			IsOverweight: conditionsCount["is_overweight"] * 100 / n,
			IsDirty:      conditionsCount["is_dirty"] * 100 / n,
		},
	}, nil
}

func isValidConditionFormatLegacy(conditionStr string) bool {
	keys := []string{"is_dirty=", "is_overweight=", "is_broken="}
	const valueTrue = "true"
	const valueFalse = "false"
	idxCondStr := 0
	for idxKeys, key := range keys {
		if !strings.HasPrefix(conditionStr[idxCondStr:], key) {
			return false
		}
		idxCondStr += len(key)
		if strings.HasPrefix(conditionStr[idxCondStr:], valueTrue) {
			idxCondStr += len(valueTrue)
		} else if strings.HasPrefix(conditionStr[idxCondStr:], valueFalse) {
			idxCondStr += len(valueFalse)
		} else {
			return false
		}
		if idxKeys < (len(keys) - 1) {
			if conditionStr[idxCondStr] != ',' {
				return false
			}
			idxCondStr++
		}
	}
	return idxCondStr == len(conditionStr)
}

func TestCalculateGraphDataPointMatchesLegacy(t *testing.T) {
	cases := [][]IsuCondition{
		{
			{Condition: "is_dirty=false,is_overweight=false,is_broken=false", IsSitting: false},
		},
		{
			{Condition: "is_dirty=true,is_overweight=false,is_broken=false", IsSitting: true},
			{Condition: "is_dirty=false,is_overweight=true,is_broken=false", IsSitting: false},
			{Condition: "is_dirty=true,is_overweight=true,is_broken=true", IsSitting: true},
		},
		{
			{Condition: "is_dirty=true,is_overweight=true,is_broken=true", IsSitting: true},
			{Condition: "is_dirty=true,is_overweight=true,is_broken=true", IsSitting: true},
			{Condition: "is_dirty=true,is_overweight=true,is_broken=true", IsSitting: false},
		},
	}
	for i, conditions := range cases {
		got, err := calculateGraphDataPoint(conditions)
		if err != nil {
			t.Fatalf("case %d: unexpected error: %v", i, err)
		}
		want, err := calculateGraphDataPointLegacy(conditions)
		if err != nil {
			t.Fatalf("case %d: legacy error: %v", i, err)
		}
		if got != want {
			t.Fatalf("case %d: got %+v want %+v", i, got, want)
		}
	}
}

func TestCalculateGraphDataPointInvalid(t *testing.T) {
	_, err := calculateGraphDataPoint([]IsuCondition{{Condition: "bad"}})
	if err == nil {
		t.Fatal("expected invalid condition format error")
	}
}

func TestIsValidConditionFormat(t *testing.T) {
	valid := []string{
		"is_dirty=false,is_overweight=false,is_broken=false",
		"is_dirty=true,is_overweight=true,is_broken=true",
		"is_dirty=true,is_overweight=false,is_broken=true",
	}
	invalid := []string{
		"",
		"is_dirty=true",
		"is_dirty=true,is_overweight=false,is_broken=maybe",
		"is_broken=false,is_dirty=false,is_overweight=false",
		"is_dirty=false,is_overweight=false,is_broken=false,",
	}
	for _, s := range valid {
		if !isValidConditionFormat(s) {
			t.Fatalf("expected valid: %q", s)
		}
	}
	for _, s := range invalid {
		if isValidConditionFormat(s) {
			t.Fatalf("expected invalid: %q", s)
		}
	}
}

func BenchmarkCalculateGraphDataPoint(b *testing.B) {
	conditions := make([]IsuCondition, 256)
	for i := range conditions {
		switch i % 4 {
		case 0:
			conditions[i] = IsuCondition{Condition: "is_dirty=false,is_overweight=false,is_broken=false", IsSitting: i%2 == 0}
		case 1:
			conditions[i] = IsuCondition{Condition: "is_dirty=true,is_overweight=false,is_broken=false", IsSitting: true}
		case 2:
			conditions[i] = IsuCondition{Condition: "is_dirty=true,is_overweight=true,is_broken=false", IsSitting: false}
		default:
			conditions[i] = IsuCondition{Condition: "is_dirty=true,is_overweight=true,is_broken=true", IsSitting: true}
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := calculateGraphDataPoint(conditions); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCalculateGraphDataPointLegacy(b *testing.B) {
	conditions := make([]IsuCondition, 256)
	for i := range conditions {
		conditions[i] = IsuCondition{Condition: "is_dirty=true,is_overweight=false,is_broken=true", IsSitting: i%3 == 0}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := calculateGraphDataPointLegacy(conditions); err != nil {
			b.Fatal(err)
		}
	}
}
