// exprhelper 类型转换 helper 的单元测试
// 覆盖：常见类型路径、字符串数字、零值、不可识别类型

package exprhelper

import "testing"

func TestToInt64(t *testing.T) {
	cases := []struct {
		name    string
		in      any
		want    int64
		wantOk  bool
	}{
		{"int64", int64(42), 42, true},
		{"int", 42, 42, true},
		{"int32", int32(42), 42, true},
		{"float64 截断", 3.9, 3, true},
		{"float32 截断", float32(3.9), 3, true},
		{"bool true", true, 1, true},
		{"bool false", false, 0, true},
		{"数字字符串", "123", 123, true},
		{"小数字符串退化", "3.7", 3, true},
		{"非数字字符串", "abc", 0, false},
		{"nil", nil, 0, false},
		{"struct 不识别", struct{}{}, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ToInt64(c.in)
			if got != c.want || ok != c.wantOk {
				t.Errorf("ToInt64(%v) = (%d, %v), want (%d, %v)",
					c.in, got, ok, c.want, c.wantOk)
			}
		})
	}
}

func TestToFloat64(t *testing.T) {
	cases := []struct {
		name   string
		in     any
		want   float64
		wantOk bool
	}{
		{"float64", 3.14, 3.14, true},
		{"int64", int64(42), 42, true},
		{"int", 42, 42, true},
		{"bool true", true, 1, true},
		{"bool false", false, 0, true},
		{"数字字符串", "1.5", 1.5, true},
		{"非数字字符串", "abc", 0, false},
		{"nil", nil, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ToFloat64(c.in)
			if got != c.want || ok != c.wantOk {
				t.Errorf("ToFloat64(%v) = (%v, %v), want (%v, %v)",
					c.in, got, ok, c.want, c.wantOk)
			}
		})
	}
}

func TestToBool(t *testing.T) {
	cases := []struct {
		name   string
		in     any
		want   bool
		wantOk bool
	}{
		{"true", true, true, true},
		{"false", false, false, true},
		{"int 非零", 42, true, true},
		{"int 零", 0, false, true},
		{"float 非零", 0.1, true, true},
		{"float 零", 0.0, false, true},
		{`字符串 "true"`, "true", true, true},
		{`字符串 "1"`, "1", true, true},
		{`字符串 "false"`, "false", false, true},
		{`字符串 "0"`, "0", false, true},
		{"空字符串", "", false, true},
		{"未知字符串", "yes", false, false},
		{"nil", nil, false, true},
		{"struct 不识别", struct{}{}, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ToBool(c.in)
			if got != c.want || ok != c.wantOk {
				t.Errorf("ToBool(%v) = (%v, %v), want (%v, %v)",
					c.in, got, ok, c.want, c.wantOk)
			}
		})
	}
}

func TestToString(t *testing.T) {
	cases := []struct {
		name   string
		in     any
		want   string
		wantOk bool
	}{
		{"string", "hello", "hello", true},
		{"int64", int64(42), "42", true},
		{"int", 42, "42", true},
		{"float64", 3.14, "3.14", true},
		{"bool true", true, "true", true},
		{"bool false", false, "false", true},
		{"nil", nil, "", true},
		{"struct 不识别", struct{}{}, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ToString(c.in)
			if got != c.want || ok != c.wantOk {
				t.Errorf("ToString(%v) = (%q, %v), want (%q, %v)",
					c.in, got, ok, c.want, c.wantOk)
			}
		})
	}
}
