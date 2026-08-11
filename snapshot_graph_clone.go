package agent

import (
	"fmt"
	"reflect"
)

type snapshotVisit struct {
	kind     reflect.Kind
	typeOf   reflect.Type
	pointer  uintptr
	length   int
	capacity int
}

type snapshotCloner struct {
	visited map[snapshotVisit]reflect.Value
}

func newSnapshotCloner() *snapshotCloner {
	return &snapshotCloner{visited: make(map[snapshotVisit]reflect.Value)}
}

func (cloner *snapshotCloner) cloneValue(
	value reflect.Value,
	path string,
) (reflect.Value, error) {
	if !value.IsValid() {
		return value, nil
	}

	switch value.Kind() {
	case reflect.Interface:
		return cloner.cloneInterface(value, path)
	case reflect.Map:
		return cloner.cloneMap(value, path)
	case reflect.Slice:
		return cloner.cloneSlice(value, path)
	case reflect.Array:
		return cloner.cloneArray(value, path)
	case reflect.Pointer:
		return cloner.clonePointer(value, path)
	case reflect.Struct:
		return cloner.cloneStruct(value, path)
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return reflect.Value{}, newSnapshotCloneError(path, value.Type())
	case reflect.Invalid,
		reflect.Bool,
		reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr,
		reflect.Float32,
		reflect.Float64,
		reflect.Complex64,
		reflect.Complex128,
		reflect.String:
		return value, nil
	}

	return value, nil
}

func (cloner *snapshotCloner) cloneInterface(
	value reflect.Value,
	path string,
) (reflect.Value, error) {
	if value.IsNil() {
		return reflect.Zero(value.Type()), nil
	}

	clonedValue, err := cloner.cloneValue(value.Elem(), path)
	if err != nil {
		return reflect.Value{}, err
	}
	cloned := reflect.New(value.Type()).Elem()
	cloned.Set(clonedValue)

	return cloned, nil
}

func (cloner *snapshotCloner) cloneMap(
	value reflect.Value,
	path string,
) (reflect.Value, error) {
	if value.IsNil() {
		return reflect.Zero(value.Type()), nil
	}
	visit := snapshotVisit{kind: value.Kind(), typeOf: value.Type(), pointer: value.Pointer()}
	if cloned, ok := cloner.visited[visit]; ok {
		return cloned, nil
	}

	cloned := reflect.MakeMapWithSize(value.Type(), value.Len())
	cloner.visited[visit] = cloned
	iterator := value.MapRange()
	for iterator.Next() {
		clonedKey, err := cloner.cloneValue(iterator.Key(), path+"[key]")
		if err != nil {
			return reflect.Value{}, err
		}
		clonedValue, err := cloner.cloneValue(iterator.Value(), path+"[value]")
		if err != nil {
			return reflect.Value{}, err
		}
		cloned.SetMapIndex(clonedKey, clonedValue)
	}

	return cloned, nil
}

func (cloner *snapshotCloner) cloneSlice(
	value reflect.Value,
	path string,
) (reflect.Value, error) {
	if value.IsNil() {
		return reflect.Zero(value.Type()), nil
	}
	visit := snapshotVisit{
		kind: value.Kind(), typeOf: value.Type(), pointer: value.Pointer(),
		length: value.Len(), capacity: value.Cap(),
	}
	if cloned, ok := cloner.visited[visit]; ok {
		return cloned, nil
	}

	cloned := reflect.MakeSlice(value.Type(), value.Len(), value.Cap())
	cloner.visited[visit] = cloned
	for index := range value.Len() {
		clonedValue, err := cloner.cloneValue(value.Index(index), fmt.Sprintf("%s[%d]", path, index))
		if err != nil {
			return reflect.Value{}, err
		}
		cloned.Index(index).Set(clonedValue)
	}

	return cloned, nil
}

func (cloner *snapshotCloner) cloneArray(
	value reflect.Value,
	path string,
) (reflect.Value, error) {
	cloned := reflect.New(value.Type()).Elem()
	for index := range value.Len() {
		clonedValue, err := cloner.cloneValue(value.Index(index), fmt.Sprintf("%s[%d]", path, index))
		if err != nil {
			return reflect.Value{}, err
		}
		cloned.Index(index).Set(clonedValue)
	}

	return cloned, nil
}

func (cloner *snapshotCloner) clonePointer(
	value reflect.Value,
	path string,
) (reflect.Value, error) {
	if value.IsNil() {
		return reflect.Zero(value.Type()), nil
	}
	visit := snapshotVisit{kind: value.Kind(), typeOf: value.Type(), pointer: value.Pointer()}
	if cloned, ok := cloner.visited[visit]; ok {
		return cloned, nil
	}

	cloned := reflect.New(value.Type().Elem())
	cloner.visited[visit] = cloned
	clonedValue, err := cloner.cloneValue(value.Elem(), path+"*")
	if err != nil {
		return reflect.Value{}, err
	}
	cloned.Elem().Set(clonedValue)

	return cloned, nil
}

func (cloner *snapshotCloner) cloneStruct(
	value reflect.Value,
	path string,
) (reflect.Value, error) {
	cloned := reflect.New(value.Type()).Elem()
	cloned.Set(value)
	for index := range value.NumField() {
		fieldType := value.Type().Field(index)
		fieldPath := path + "." + fieldType.Name
		if !fieldType.IsExported() {
			if containsMutableValue(value.Field(index)) {
				return reflect.Value{}, newSnapshotCloneError(fieldPath, fieldType.Type)
			}

			continue
		}

		clonedField, err := cloner.cloneValue(value.Field(index), fieldPath)
		if err != nil {
			return reflect.Value{}, err
		}
		cloned.Field(index).Set(clonedField)
	}

	return cloned, nil
}

func containsMutableValue(value reflect.Value) bool {
	if !value.IsValid() {
		return false
	}

	switch value.Kind() {
	case reflect.Interface:
		return !value.IsNil() && containsMutableValue(value.Elem())
	case reflect.Map, reflect.Slice, reflect.Pointer, reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return !value.IsNil()
	case reflect.Array:
		for index := range value.Len() {
			if containsMutableValue(value.Index(index)) {
				return true
			}
		}
	case reflect.Struct:
		for index := range value.NumField() {
			if containsMutableValue(value.Field(index)) {
				return true
			}
		}
	case reflect.Invalid,
		reflect.Bool,
		reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr,
		reflect.Float32,
		reflect.Float64,
		reflect.Complex64,
		reflect.Complex128,
		reflect.String:
		return false
	}

	return false
}

func newSnapshotCloneError(path string, typeOf reflect.Type) *SnapshotCloneError {
	return &SnapshotCloneError{Path: path, Type: typeOf.String()}
}
