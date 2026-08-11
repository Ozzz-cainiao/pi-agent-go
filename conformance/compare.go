package conformance

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// Compare 将 runner 结果与 fixture golden 逐字段比较。
func Compare(fixture Fixture, actual Result) error {
	if mismatch := compareStrings("events", fixture.Expected.Events, actual.Events); mismatch != nil {
		mismatch.Fixture = fixture.Name
		return mismatch
	}
	if mismatch := compareMessages(fixture.Expected.Messages, actual.Messages); mismatch != nil {
		mismatch.Fixture = fixture.Name
		return mismatch
	}
	if fixture.Expected.StopReason != actual.StopReason {
		return &MismatchError{
			Fixture: fixture.Name, Field: "stop_reason",
			Want: fixture.Expected.StopReason, Got: actual.StopReason,
		}
	}
	return nil
}

func compareStrings(field string, want, got []string) *MismatchError {
	limit := min(len(want), len(got))
	for index := range limit {
		if want[index] != got[index] {
			return &MismatchError{Field: fmt.Sprintf("%s[%d]", field, index), Want: want[index], Got: got[index]}
		}
	}
	if len(want) != len(got) {
		return &MismatchError{Field: field + ".length", Want: len(want), Got: len(got)}
	}
	return nil
}

func compareMessages(want, got []MessageRecord) *MismatchError {
	limit := min(len(want), len(got))
	for index := range limit {
		if !reflect.DeepEqual(want[index], got[index]) {
			return &MismatchError{Field: fmt.Sprintf("messages[%d]", index), Want: want[index], Got: got[index]}
		}
	}
	if len(want) != len(got) {
		return &MismatchError{Field: "messages.length", Want: len(want), Got: len(got)}
	}
	return nil
}

func formatValue(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(encoded)
}
