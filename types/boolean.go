package types

import (
	"encoding/json"
	"strconv"
)

// Bool gorm bool
// gorm:"column:xxxx;type:TINYINT;default:-1"
type Bool int32

const (
	// False .
	False Bool = -1
	// True .
	True Bool = 1
	// Any .
	Any Bool = 2
)

// BoolName 对应关系
var BoolName = map[Bool]string{
	False: "FALSE",
	True:  "TRUE",
	Any:   "ANY",
}

// New ...
func NewBool(v bool) Bool {
	if v {
		return True
	}
	return False
}

// ToString .
func (x Bool) ToString() string {
	if v, ok := BoolName[x]; ok {
		return v
	}
	return "FALSE"
}

// Value ...
func (b Bool) Value() bool {
	return b == True
}

// MarshalJSON .
func (b Bool) MarshalJSON() ([]byte, error) {
	v := b.Value()
	return json.Marshal(v)
}

// UnmarshalJSON .
func (b *Bool) UnmarshalJSON(data []byte) error {
	v, err := strconv.ParseBool(string(data))
	if err != nil {
		return err
	}
	*b = NewBool(v)
	return nil
}

// MarshalYAML .
func (b Bool) MarshalYAML() (interface{}, error) {
	v := b.Value()
	return v, nil
}

// UnmarshalYAML .
func (b *Bool) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var v bool
	err := unmarshal(&v)
	if err != nil {
		return err
	}
	*b = NewBool(v)
	return nil
}
