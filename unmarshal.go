package binstruct

import (
	"reflect"
	"strings"

	"github.com/pkg/errors"
)

type unmarshal struct {
	r Reader
}

// An InvalidUnmarshalError describes an invalid argument passed to Unmarshal.
// (The argument to Unmarshal must be a non-nil pointer.)
type InvalidUnmarshalError struct {
	Type reflect.Type
}

func (e *InvalidUnmarshalError) Error() string {
	if e.Type == nil {
		return "binstruct: Unmarshal(nil)"
	}

	if e.Type.Kind() != reflect.Ptr {
		return "binstruct: Unmarshal(non-pointer " + e.Type.String() + ")"
	}
	return "binstruct: Unmarshal(nil " + e.Type.String() + ")"
}

func (u *unmarshal) Unmarshal(v interface{}) error {
	return u.unmarshal(v, nil)
}

func (u *unmarshal) unmarshal(v interface{}, parentStructValues []reflect.Value) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return &InvalidUnmarshalError{reflect.TypeOf(v)}
	}

	structValue := rv.Elem()
	numField := structValue.NumField()

	valueType := structValue.Type()
	for i := 0; i < numField; i++ {
		fieldType := valueType.Field(i)
		tags, err := parseTag(fieldType.Tag.Get(tagName))
		if err != nil {
			return errors.Wrapf(err, `failed parseTag for field "%s"`, fieldType.Name)
		}

		fieldData, err := parseReadDataFromTags(structValue, tags)
		if err != nil {
			return errors.Wrapf(err, `failed parse ReadData from tags for field "%s"`, fieldType.Name)
		}

		fieldValue := structValue.Field(i)
		err = u.setValueToField(structValue, fieldValue, fieldData, parentStructValues)
		if err != nil {
			return errors.Wrapf(err, `failed set value to field "%s"`, fieldType.Name)
		}
	}

	return nil
}

func (u *unmarshal) setValueToField(structValue, fieldValue reflect.Value, fieldData *fieldReadData, parentStructValues []reflect.Value) error {
	if fieldData == nil {
		fieldData = &fieldReadData{}
	}

	if fieldData.Ignore {
		return nil
	}

	err := setOffset(u.r, fieldData)
	if err != nil {
		return errors.Wrap(err, "set offset")
	}

	if fieldData.FuncName != "" {
		okCallFunc, err := callFunc(u.r, fieldData.FuncName, structValue, fieldValue)
		if err != nil {
			return errors.Wrap(err, "call custom func")
		}

		if !okCallFunc {
			// Try call function from parent structs
			for i := len(parentStructValues) - 1; i >= 0; i-- {
				sv := parentStructValues[i]
				okCallFunc, err = callFunc(u.r, fieldData.FuncName, sv, fieldValue)
				if err != nil {
					return errors.Wrap(err, "call custom func")
				}

				if okCallFunc {
					return nil
				}
			}

			message := `
failed call method, expected methods:
	func (*{{Struct}}) {{MethodName}}(r binstruct.Reader) error {} 
or
	func (*{{Struct}}) {{MethodName}}(r binstruct.Reader) ({{FieldType}}, error) {}
`
			message = strings.NewReplacer(
				`{{Struct}}`, structValue.Type().Name(),
				`{{MethodName}}`, fieldData.FuncName,
				`{{FieldType}}`, fieldValue.Type().String(),
			).Replace(message)
			return errors.New(message)
		}

		return nil
	}

	switch fieldValue.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var value int64
		var err error

		switch {
		case fieldData.Length != nil && *fieldData.Length == 1 || fieldValue.Kind() == reflect.Int8:
			v, e := u.r.ReadInt8()
			value = int64(v)
			err = e
		case fieldData.Length != nil && *fieldData.Length == 2 || fieldValue.Kind() == reflect.Int16:
			v, e := u.r.ReadInt16()
			value = int64(v)
			err = e
		case fieldData.Length != nil && *fieldData.Length == 4 || fieldValue.Kind() == reflect.Int32:
			v, e := u.r.ReadInt32()
			value = int64(v)
			err = e
		case fieldData.Length != nil && *fieldData.Length == 8 || fieldValue.Kind() == reflect.Int64:
			value, err = u.r.ReadInt64()
		default: // reflect.Int:
			err = errors.New("need set tag with len or use int8/int16/int32/int64")
		}
		if err != nil {
			return err
		}

		if fieldValue.CanSet() {
			fieldValue.SetInt(value)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		var value uint64
		var err error

		switch {
		case fieldData.Length != nil && *fieldData.Length == 1 || fieldValue.Kind() == reflect.Uint8:
			v, e := u.r.ReadUint8()
			value = uint64(v)
			err = e
		case fieldData.Length != nil && *fieldData.Length == 2 || fieldValue.Kind() == reflect.Uint16:
			v, e := u.r.ReadUint16()
			value = uint64(v)
			err = e
		case fieldData.Length != nil && *fieldData.Length == 4 || fieldValue.Kind() == reflect.Uint32:
			v, e := u.r.ReadUint32()
			value = uint64(v)
			err = e
		case fieldData.Length != nil && *fieldData.Length == 8 || fieldValue.Kind() == reflect.Uint64:
			value, err = u.r.ReadUint64()
		default: // reflect.Uint:
			err = errors.New("need set tag with len or use uint8/uint16/uint32/uint64")
		}
		if err != nil {
			return err
		}

		if fieldValue.CanSet() {
			fieldValue.SetUint(value)
		}
	case reflect.Float32:
		f, err := u.r.ReadFloat32()
		if err != nil {
			return err
		}

		if fieldValue.CanSet() {
			fieldValue.SetFloat(float64(f))
		}
	case reflect.Float64:
		f, err := u.r.ReadFloat64()
		if err != nil {
			return err
		}

		if fieldValue.CanSet() {
			fieldValue.SetFloat(f)
		}
	case reflect.Bool:
		b, err := u.r.ReadBool()
		if err != nil {
			return err
		}

		if fieldValue.CanSet() {
			fieldValue.SetBool(b)
		}
	case reflect.String:
		if fieldData.Length == nil {
			return errors.New("need set tag with len for string")
		}

		_, b, err := u.r.ReadBytes(int(*fieldData.Length))
		if err != nil {
			return err
		}

		if fieldValue.CanSet() {
			fieldValue.SetString(string(b))
		}
	case reflect.Slice:
		if fieldData.Length == nil {
			return errors.New("need set tag with len for slice")
		}

		for i := int64(0); i < *fieldData.Length; i++ {
			tmpV := reflect.New(fieldValue.Type().Elem()).Elem()
			err := u.setValueToField(structValue, tmpV, fieldData.ElemFieldData, parentStructValues)
			if err != nil {
				return err
			}
			if fieldValue.CanSet() {
				fieldValue.Set(reflect.Append(fieldValue, tmpV))
			}
		}
	case reflect.Array:
		var arrLen int64

		if fieldData.Length != nil {
			arrLen = *fieldData.Length
		}

		if arrLen == 0 {
			arrLen = int64(fieldValue.Len())
		}

		for i := int64(0); i < arrLen; i++ {
			tmpV := reflect.New(fieldValue.Type().Elem()).Elem()
			err := u.setValueToField(structValue, tmpV, fieldData.ElemFieldData, parentStructValues)
			if err != nil {
				return err
			}
			if fieldValue.CanSet() {
				fieldValue.Index(int(i)).Set(tmpV)
			}
		}
	case reflect.Struct:
		err := u.unmarshal(fieldValue.Addr().Interface(), append(parentStructValues, structValue))
		if err != nil {
			return errors.Wrap(err, "unmarshal struct")
		}
	default:
		return errors.New(`type "` + fieldValue.Kind().String() + `" not supported`)
	}

	return nil
}

func callFunc(r Reader, funcName string, structValue, fieldValue reflect.Value) (bool, error) {
	// Call methods
	m := structValue.Addr().MethodByName(funcName)

	readerType := reflect.TypeOf((*Reader)(nil)).Elem()
	if m.IsValid() && m.Type().NumIn() == 1 && m.Type().In(0) == readerType {
		ret := m.Call([]reflect.Value{reflect.ValueOf(r)})

		errorType := reflect.TypeOf((*error)(nil)).Elem()

		// Method(r binstruct.Reader) error
		if len(ret) == 1 && ret[0].Type() == errorType {
			if !ret[0].IsNil() {
				return true, ret[0].Interface().(error)
			}

			return true, nil
		}

		// Method(r binstruct.Reader) (FieldType, error)
		if len(ret) == 2 && ret[0].Type() == fieldValue.Type() && ret[1].Type() == errorType {
			if !ret[1].IsNil() {
				return true, ret[1].Interface().(error)
			}

			if fieldValue.CanSet() {
				fieldValue.Set(ret[0])
			}
			return true, nil
		}
	}

	return false, nil
}

func setOffset(r Reader, fieldData *fieldReadData) error {
	for _, v := range fieldData.Offsets {
		_, err := r.Seek(v.Offset, v.Whence)
		if err != nil {
			return errors.Wrap(err, "seek")
		}
	}

	return nil
}
