// 表达式节点（算术 / 比较 / 逻辑）共享的类型转换 helper
// 设计原则：尽量容错，转换失败返回零值 + ok=false，由调用方决定是否 Warn
// 不用 reflect，靠 type switch 覆盖 JSON / Wails / TOML 反序列化常见的 Go 类型

package exprhelper

import "strconv"

// ToInt64 容错转 int64
// 支持：int / int32 / int64 / float32 / float64 / bool / 数字字符串
func ToInt64(v any) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case int:
		return int64(x), true
	case int32:
		return int64(x), true
	case float64:
		return int64(x), true
	case float32:
		return int64(x), true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	case string:
		if n, err := strconv.ParseInt(x, 10, 64); err == nil {
			return n, true
		}
		// 退化：允许数字字符串带小数点，截断为整数
		if f, err := strconv.ParseFloat(x, 64); err == nil {
			return int64(f), true
		}
	}
	return 0, false
}

// ToFloat64 容错转 float64
func ToFloat64(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int64:
		return float64(x), true
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	case string:
		if f, err := strconv.ParseFloat(x, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// ToBool 容错转 bool
// 数字非零为 true；字符串只识别 "true"/"1" / "false"/"0"
func ToBool(v any) (bool, bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	case int64:
		return x != 0, true
	case int:
		return x != 0, true
	case int32:
		return x != 0, true
	case float64:
		return x != 0, true
	case float32:
		return x != 0, true
	case string:
		switch x {
		case "true", "1":
			return true, true
		case "false", "0", "":
			return false, true
		}
	case nil:
		return false, true
	}
	return false, false
}

// ToString 容错转 string
func ToString(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case int64:
		return strconv.FormatInt(x, 10), true
	case int:
		return strconv.Itoa(x), true
	case int32:
		return strconv.FormatInt(int64(x), 10), true
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64), true
	case float32:
		return strconv.FormatFloat(float64(x), 'g', -1, 32), true
	case bool:
		return strconv.FormatBool(x), true
	case nil:
		return "", true
	}
	return "", false
}
