package types

import (
	"encoding/json"
	"testing"
)

// TestMoney
func TestMoneyString(t *testing.T) {
	type testTable struct {
		Val float64
		Str string
	}
	for _, v := range []testTable{
		{1.00, "1.00"},
		{1.000, "1.00"},
		{1.001, "1.00"},
		{1.0000001, "1.00"},
		{1.004, "1.00"},
		{1.0048, "1.00"},
		{1.0045, "1.00"},
		{1.0049, "1.00"},
		{1.00499, "1.00"},
		{1.009, "1.01"},
		{0.999, "1.00"},
		{0.996, "1.00"},
		{0.99, "0.99"},
		{0.990, "0.99"},
		{0.994, "0.99"},
		{0.992, "0.99"},
		{1.005, "1.01"},
		{0.995, "1.00"},
	} {
		m := Money(v.Val)
		if v.Str != m.String() {
			t.Logf("pv %v: p2f %.2f, pv %v", v.Val, v.Val, m.Val())
			t.Errorf("falied. %#v assest: %s, real: %s", v.Val, v.Str, m.String())
		}
	}
}

// TestMoneyJSON
func TestMoneyJSON(t *testing.T) {
	v := Money(1.99999)
	bs, _ := json.Marshal(v)

	var v2 Money
	json.Unmarshal(bs, &v2)
	if string(bs) != v2.String() {
		t.Errorf("assest: %s, real: %s", bs, v2)
	}
}

// TestZero ...
func TestZero(t *testing.T) {
	type tb struct {
		k Money
		v bool
	}
	for _, it := range []tb{
		{Money(0.0), true},
		{Money(0.001), true},
		{Money(0.000), true},
		{Money(0.004), true},
		{Money(0.005), false},
	} {
		k, v := it.k, it.v
		if k.IsZero() != v {
			t.Errorf("%+v -- %v", k, v)
		}
	}
}

func TestToUpper(t *testing.T) {
	type tb struct {
		k Money
		v string
	}
	for _, it := range []tb{
		{Money(12.0), "壹拾贰圆整"},
		{Money(532.001), "伍佰叁拾贰圆整"},
		{Money(6684.010), "陆仟陆佰捌拾肆圆零壹分"},
		{Money(5300.20), "伍仟叁佰圆贰角"},
		{Money(6900), "陆仟玖佰圆整"},
		{Money(7931.234), "柒仟玖佰叁拾壹圆贰角叁分"},
		{Money(531.319), "伍佰叁拾壹圆叁角贰分"},
		{Money(1.99), "壹圆玖角玖分"},
		{Money(1.999), "贰圆整"},
		{Money(20000.999), "贰万零壹圆整"},
		{Money(200000.999), "贰拾万零壹圆整"},
		{Money(3698521.35), "叁佰陆拾玖万捌仟伍佰贰拾壹圆叁角伍分"},
		{Money(0.0), "零圆整"},
	} {
		k, v := it.k, it.v
		if k.ToUpper() != v {
			t.Errorf("%+v -- %v , error:%v", k, v, k.ToUpper())
		}
	}
}

// TestEqual ...
func TestEqual(t *testing.T) {
	type tb struct {
		a, b Money
		eq   bool
	}
	for _, v := range []tb{
		{Money(20000.999), Money(20000.999), true},
		{Money(20000.999), Money(20000.998), true},
		{Money(20000.999), Money(20000.997), true},
		{Money(20000.999), Money(20000.995), true},
		{Money(20000.999), Money(20000.994), false},
		{Money(20000.999), Money(20000.99499999), false},
		{Money(0.999), Money(0.994), false},
		{Money(0.999), Money(0.995), true},
		{Money(0.99001), Money(0.9949999999), true},
	} {
		if v.a.Equal(v.b) != v.eq {
			t.Errorf("%v == %v not %v", v.a, v.b, v.eq)
		}
	}
}

// TestArithmetic ...
func TestArithmetic(t *testing.T) {
	type tb struct {
		a, b Money
	}
	for i, v := range []tb{
		{Money(1.004).Add(Money(1.004)), Money(2.00)},
		{Money(1.004).Add(Money(1.005)), Money(2.01)},
		{Money(1.005).Sub(Money(1.004)), Money(0.01)},
		{Money(1.006).Sub(Money(1.005)), Money(0.00)},
		{Money(1.003).Mul(2), Money(2.01)},
		{Money(1.002).Mul(2), Money(2.00)},
		{Money(1.009).Div(2), Money(0.50)},
		{Money(1.01).Div(2), Money(0.51)},
		{Money(0.99).Div(100), Money(0.01)},
		{Money(0.1).Div(20), Money(0.01)},
		{Money(0.1).Div(21), Money(0.00)},
	} {
		if v.a != v.b {
			t.Errorf("index %v failed.", i)
		}
	}
}

// TestFloor ...
func TestFloor(t *testing.T) {
	type tb struct {
		a, b Money
	}
	for i, v := range []tb{
		{Money(1.003), Money(1.00)},
		{Money(0.09), Money(0.00)},
		{Money(1.999), Money(1.00)},
		{Money(1.001), Money(1.00)},
	} {
		if v.a.Floor() != v.b {
			t.Errorf("Floor %v: %v, %v, %v", i, v.a, v.a.Floor(), v.b)
		}
	}
}

// TestCeil ...
func TestCeil(t *testing.T) {
	type tb struct {
		a, b Money
	}
	for i, v := range []tb{
		{Money(1.003), Money(2.00)},
		{Money(1.999), Money(2.00)},
		{Money(0.003), Money(1.00)},
	} {
		if v.a.Ceil() != v.b {
			t.Errorf("Ceil %v: %v, %v, %v", i, v.a, v.a.Ceil(), v.b)
		}
	}
}
