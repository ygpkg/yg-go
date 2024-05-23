package server

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
)

var existsStructInfo = map[string]*structParser{}

type structParser struct {
	Depth   int  `json:"Depth"`
	IsPtr   bool `json:"IsPtr,omitempty"`
	IsSlice bool `json:"IsSlice,omitempty"`

	Kind      string
	FieldName string

	NumField int    `json:"NumField,omitempty"`
	Name     string `json:"Name,omitempty"`
	Tag      string `json:"Tag,omitempty"`

	Key *structParser `json:"Key,omitempty"`
	Val *structParser `json:"Val,omitempty"`

	Fields []*structParser `json:"Fields,omitempty"`
}

func parseInterface(v interface{}) error {
	sp := &structParser{}
	rt := reflect.TypeOf(v)

	sp.parse(rt)

	sp.Print()
	return nil
}

func (sp *structParser) parse(rt reflect.Type) error {
	if rt.PkgPath() != "" {
		sp.Name = fmt.Sprintf("%s.%s", rt.PkgPath(), rt.Name())
		if oldsp, ok := existsStructInfo[sp.Name]; ok {
			sp.Kind = oldsp.Kind
			sp.Name += ".copy"
			return nil
		}
		existsStructInfo[sp.Name] = sp
	}

	sp.Kind = rt.Kind().String()

	switch rt.Kind() {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Uintptr, reflect.Float32, reflect.Float64,
		reflect.Complex64, reflect.Complex128,
		reflect.String:

	case reflect.Struct:
		sp.parseStruct(rt)

	case reflect.Ptr:
		sp.IsPtr = true
		sp.parse(rt.Elem())

	case reflect.Array, reflect.Slice:
		sp.IsSlice = true
		sp.parse(rt.Elem())

	case reflect.Map:
		sp.parseMap(rt)

	case reflect.Chan, reflect.Func:
		panic(fmt.Errorf("invalid struct %v", sp))
	case reflect.UnsafePointer, reflect.Interface:
		panic(fmt.Errorf("unsupport struct %v", sp))
	}
	return nil
}
func (sp *structParser) parseStruct(rt reflect.Type) error {
	sp.NumField = rt.NumField()
	for i := 0; i < sp.NumField; i++ {
		fd := rt.Field(i)
		sp.newStructField(fd)
	}

	return nil
}
func (sp *structParser) parseMap(rt reflect.Type) error {
	keysp := &structParser{}
	keysp.Depth = sp.Depth + 1
	keysp.FieldName = "map.key"
	keysp.parse(rt.Key())
	sp.Key = keysp

	valsp := &structParser{}
	valsp.Depth = sp.Depth + 1
	valsp.FieldName = "map.value"
	valsp.parse(rt.Elem())
	sp.Val = valsp

	return nil
}

func (sp *structParser) newStructField(fd reflect.StructField) *structParser {
	rt := fd.Type
	nsp := &structParser{}
	nsp.Depth = sp.Depth + 1
	nsp.FieldName = fd.Name
	nsp.Tag = string(fd.Tag.Get("json"))

	nsp.parse(rt)

	sp.Fields = append(sp.Fields, nsp)
	return nsp
}

// Print 打印结构体信息
func (sp *structParser) Print() {
	edr := json.NewEncoder(os.Stdout)
	edr.SetIndent(" ", " ")
	edr.Encode(sp)

	for k, v := range existsStructInfo {
		fmt.Printf("%s %v \n", k, v)
	}
}
