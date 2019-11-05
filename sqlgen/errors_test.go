package sqlgen

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorWithQuery(t *testing.T) {

	ints := []int{1, 2, 3, 4}
	intsInterfaceArr := make([]interface{}, len(ints))
	for i, s := range ints {
		intsInterfaceArr[i] = s
	}

	strs := []string{"a", "b", "c", "asdf"}
	strsInterfaceArr := make([]interface{}, len(strs))
	for i, s := range strs {
		strsInterfaceArr[i] = s
	}

	bools := []bool{true, false, false, true}
	boolsInterfaceArr := make([]interface{}, len(bools))
	for i, s := range bools {
		boolsInterfaceArr[i] = s
	}

	testcases := []struct {
		title         string
		originalError error
		clause        string
		args          []interface{}
		isNested      bool
	}{
		{
			title:         "nil original error",
			originalError: nil,
			clause:        "some clause, foo, bar, baz",
			args:          intsInterfaceArr,
		},
		{
			title:         "ok original error with no args",
			originalError: fmt.Errorf("original"),
			clause:        "select * from mysql.users",
		},
		{
			title:         "ok original error with no clause",
			originalError: fmt.Errorf("original"),
			args:          boolsInterfaceArr,
		},
		{
			title:         "ok original error with clause and args",
			originalError: fmt.Errorf("original"),
			clause:        "some clause, foo, bar, baz",
			args:          intsInterfaceArr,
		},
		{
			title: "wrapped original error with clause and args",
			originalError: &ErrorWithQuery{
				err:    fmt.Errorf("innermost"),
				clause: "some inner clause",
				args:   intsInterfaceArr,
			},
			clause:   "some outer clause, foo, bar, baz",
			args:     strsInterfaceArr,
			isNested: true,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.title, func(t *testing.T) {
			originalError := testcase.originalError
			wrapped := &ErrorWithQuery{
				err:    originalError,
				clause: testcase.clause,
				args:   testcase.args,
			}

			if originalError == nil {
				assert.Equal(t, wrapped.Error(), "")
				return
			}

			assert.Contains(t, wrapped.Error(), fmt.Sprintf("Error: %s;", originalError.Error()))
			assert.Contains(t, wrapped.Error(), fmt.Sprintf("query clause: '%s'", testcase.clause))
			assert.Contains(t, wrapped.Error(), fmt.Sprintf("query args: '%v'", testcase.args))

			assert.Equal(t, wrapped.Unwrap(), originalError)

			if testcase.isNested {
				assert.Contains(t, wrapped.Unwrap().Error(), "query clause:")
				assert.Contains(t, wrapped.Unwrap().Error(), "query args:")
			} else {
				assert.NotContains(t, wrapped.Unwrap().Error(), "query clause:")
				assert.NotContains(t, wrapped.Unwrap().Error(), "query args:")
			}
		})
	}
}

func TestErrorWithQuery_Reason(t *testing.T) {
	type fields struct {
		err    error
		clause string
		args   []interface{}
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "nil err and args",
			fields: fields{
				clause: "clause",
				args:   nil,
			},
			want: "Error in query clause: 'clause'; query args: '[]'",
		},
		{
			name: "args array is not empty",
			fields: fields{
				clause: "clause",
				args:   []interface{}{"a", "b"},
			},
			want: "Error in query clause: 'clause'; query args: '[a b]'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &ErrorWithQuery{
				err:    tt.fields.err,
				clause: tt.fields.clause,
				args:   tt.fields.args,
			}
			if got := e.Reason(); got != tt.want {
				t.Errorf("Reason() = %v, want %v", got, tt.want)
			}
		})
	}
}
