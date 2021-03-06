// Copyright 2020 Google LLC
//
// Liceed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package yml

import (
	"fmt"

	"github.com/google/cel-policy-templates-go/policy/model"
)

// objRef defines a series of methods used to build an object model from the YAML decode step.
type objRef interface {
	// id assigns the relative source element identifier to the object.
	id(id int64)

	// assign a primitive value to the object.
	//
	// If the object is not a primitive value, return an error.
	assign(value interface{}) error

	encodeStyle(value model.EncodeStyle)

	initMap() error

	initList() error

	// field creates an objRef for the field with the given name for building nested objects.
	//
	// If the object does not have thefield  or is not a map-like type, the method will return
	// an error.
	field(id int64, name string) (objRef, error)

	// entry creates an objRef for the entry at the given 'idx' ordinal for building list entries.
	//
	// If the object is not a list or the index is not between 0 and the length of the list, the
	// function will return an error.
	entry(idx interface{}) (objRef, error)
}

// newBaseBuilder returns a base builder which implements the core methods of the objRef interface.
func newBaseBuilder(typeName string) *baseBuilder {
	return &baseBuilder{typeName: typeName}
}

type baseBuilder struct {
	typeName string
}

// id is an implementation of the objRef interface method.
func (b *baseBuilder) id(id int64) {}

// assign is an implementation of the objRef interface method.
func (b *baseBuilder) assign(val interface{}) error {
	return valueNotAssignableToType(b.typeName, val)
}

func (b *baseBuilder) encodeStyle(value model.EncodeStyle) {}

func (b *baseBuilder) initMap() error {
	return typeNotAssignableToType(b.typeName, model.MapType)
}

func (b *baseBuilder) initList() error {
	return typeNotAssignableToType(b.typeName, model.ListType)
}

// field is an implementation of the objRef interface method.
func (b *baseBuilder) field(id int64, name string) (objRef, error) {
	return nil, typeNotAssignableToType(b.typeName, model.MapType)
}

// entry is an implementation of the objRef interface method.
func (b *baseBuilder) entry(idx interface{}) (objRef, error) {
	return nil, typeNotAssignableToType(b.typeName, model.ListType)
}

func newParsedValueBuilder(pv *model.ParsedValue) *parsedValueBuilder {
	pv.Value = model.NewMapValue()
	return &parsedValueBuilder{
		baseBuilder: newBaseBuilder(model.MapType),
		pv:          pv,
		mv:          pv.Value,
	}
}

type parsedValueBuilder struct {
	*baseBuilder
	pv *model.ParsedValue
	mv *model.MapValue
}

func (b *parsedValueBuilder) id(id int64) {
	b.pv.ID = id
}

func (b *parsedValueBuilder) field(id int64, name string) (objRef, error) {
	field := model.NewMapField(id, name)
	b.mv.AddField(field)
	return newDynValueBuilder(field.Ref), nil
}

// newMapBuilder returns a builder for dynamic values of struct type.
func newMapBuilder(mv *model.MapValue) *mapBuilder {
	return &mapBuilder{
		baseBuilder: newBaseBuilder(model.MapType),
		mv:          mv,
	}
}

type mapBuilder struct {
	*baseBuilder
	mv *model.MapValue
}

func (b *mapBuilder) initMap() error {
	return nil
}

// prop returns a builder for a struct property.
func (b *mapBuilder) field(id int64, name string) (objRef, error) {
	field := model.NewMapField(id, name)
	b.mv.AddField(field)
	return newDynValueBuilder(field.Ref), nil
}

// newListBuilder returns a builder for a dynamic value of list type.
func newListBuilder(lv *model.ListValue) *listBuilder {
	return &listBuilder{
		baseBuilder: newBaseBuilder(model.ListType),
		listVal:     lv,
	}
}

type listBuilder struct {
	*baseBuilder
	listVal *model.ListValue
}

func (b *listBuilder) initList() error {
	return nil
}

// entry returns a builder for a list element at the given index.
func (b *listBuilder) entry(idx interface{}) (objRef, error) {
	err := checkIndexRange(idx, len(b.listVal.Entries))
	if err != nil {
		return nil, err
	}
	dyn := model.NewEmptyDynValue()
	b.listVal.Entries = append(b.listVal.Entries, dyn)
	return newDynValueBuilder(dyn), nil
}

// newDynValueBuilder returns a builder for a model.DynValue.
func newDynValueBuilder(dyn *model.DynValue) *dynValueBuilder {
	return &dynValueBuilder{
		dyn: dyn,
	}
}

type dynValueBuilder struct {
	dyn *model.DynValue
	lb  *listBuilder
	mb  *mapBuilder
}

// id sets the source element id of the dyn literal.
func (b *dynValueBuilder) id(id int64) {
	b.dyn.ID = id
}

// assign will set the value of the model.DynValue.
//
// If the builder had previously been configured to produce list or struct values, the function
// returns an error.
func (b *dynValueBuilder) assign(val interface{}) error {
	if b.mb != nil {
		return valueNotAssignableToType(model.MapType, val)
	}
	if b.lb != nil {
		return valueNotAssignableToType(model.ListType, val)
	}
	var vn model.ValueNode
	switch v := val.(type) {
	case bool:
		vn = model.BoolValue(v)
	case float64:
		vn = model.DoubleValue(v)
	case int64:
		vn = model.IntValue(v)
	case string:
		vn = model.StringValue(v)
	case uint64:
		vn = model.UintValue(v)
	case *model.MultilineStringValue:
		vn = v
	case model.PlainTextValue:
		vn = v
	case model.NullValue:
		vn = v
	default:
		return valueNotAssignableToType(model.AnyType, v)
	}
	b.dyn.Value = vn
	return nil
}

func (b *dynValueBuilder) encodeStyle(value model.EncodeStyle) {
	b.dyn.EncodeStyle = value
}

func (b *dynValueBuilder) initMap() error {
	if b.lb != nil {
		return typeNotAssignableToType(model.ListType, model.MapType)
	}
	if b.mb == nil {
		sv := model.NewMapValue()
		b.dyn.Value = sv
		b.mb = newMapBuilder(sv)
	}
	return nil
}

// field returns a builder for a map field.
//
// If the dyn builder was previously configured as a list builder, the function will error.
func (b *dynValueBuilder) field(id int64, name string) (objRef, error) {
	if b.mb == nil {
		return nil, noSuchProperty(model.AnyType, name)
	}
	return b.mb.field(id, name)
}

func (b *dynValueBuilder) initList() error {
	if b.mb != nil {
		return typeNotAssignableToType(model.MapType, model.ListType)
	}
	if b.lb == nil {
		lv := model.NewListValue()
		b.dyn.Value = lv
		b.lb = newListBuilder(lv)
	}
	return nil
}

// entry returns a builder for an entry within a list value.
//
// If the dyn builder was previously configured as a struct, this function will error.
func (b *dynValueBuilder) entry(idx interface{}) (objRef, error) {
	if b.lb == nil {
		return nil, noSuchProperty("dyn", "[]")
	}
	return b.lb.entry(idx)
}

// helper methods for formatting builder-related error messages.

func checkIndexRange(idx interface{}, sz int) error {
	i, ok := idx.(int)
	if !ok {
		return invalidIndexType("int", idx)
	}
	if i < 0 || i > sz {
		return indexOutOfRange(idx, sz)
	}
	return nil
}

func typeNotAssignableToType(typeName, valType string) error {
	return fmt.Errorf("type not assignable to target: target=%v, type=%s", typeName, valType)
}

func valueNotAssignableToType(typeName string, val interface{}) error {
	return fmt.Errorf("type not assignable to target: target=%s, type=%T", typeName, val)
}

func noSuchProperty(typeName, prop string) error {
	return fmt.Errorf("no such property: type=%s, property=%s", typeName, prop)
}

func indexOutOfRange(idx interface{}, len int) error {
	return fmt.Errorf("index out of range: index=%v, len=%d", idx, len)
}

func invalidIndexType(typeName string, idx interface{}) error {
	return fmt.Errorf("invalid index type: index-type:%s, argument=%T", typeName, idx)
}
