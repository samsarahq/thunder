package fields

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIsZero(t *testing.T) {
	var zeroSlice []string
	var zeroMap map[string]string
	var zeroPointer *time.Location
	var zeroInterface testing.TB
	var nonZeroInterface testing.TB
	nonZeroInterface = t

	tests := []struct {
		Name   string
		Give   interface{}
		IsZero bool
	}{
		{"zero int", int(0), true},
		{"int", int(1), false},
		{"zero int64", int64(0), true},
		{"int64", int64(1), false},
		{"zero bool", false, true},
		{"bool", true, false},
		{"zero uint", uint(0), true},
		{"Uint", uint(1), false},
		{"zero float", float64(0), true},
		{"float", float64(12.2), false},
		{"zero string", "", true},
		{"string", "not zero", false},
		{"zero array", [0]string{}, true},
		{"array", [4]string{"1", "2", "3", "4"}, false},
		{"zero slice", ([]string)(nil), true},
		{"type zero slice", zeroSlice, true},
		{"slice", []string{}, false},
		{"zero map", (map[string]string)(nil), true},
		{"type zero map", zeroMap, true},
		{"map", map[string]string{}, false},
		{"zero pointer", (*time.Location)(nil), true},
		{"type zero pointer", zeroPointer, true},
		{"pointer", time.UTC, false},
		{"type zero interface", zeroInterface, true},
		{"interface", nonZeroInterface, false},
		{"zero struct", time.Time{}, true},
		{"struct", time.Now(), false},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			assert.Equal(t, tt.IsZero, isZero(reflect.ValueOf(tt.Give)))
		})
	}
}
