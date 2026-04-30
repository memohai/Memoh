package store

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func convertValue(src any, dst any) error {
	if dst == nil {
		return nil
	}
	dstValue := reflect.ValueOf(dst)
	if dstValue.Kind() != reflect.Pointer || dstValue.IsNil() {
		return errors.New("destination must be a non-nil pointer")
	}
	return assignValue(reflect.ValueOf(src), dstValue.Elem())
}

func assignValue(src reflect.Value, dst reflect.Value) error {
	if !dst.CanSet() {
		return nil
	}
	if !src.IsValid() {
		dst.Set(reflect.Zero(dst.Type()))
		return nil
	}
	if src.Kind() == reflect.Pointer {
		if src.IsNil() {
			dst.Set(reflect.Zero(dst.Type()))
			return nil
		}
		src = src.Elem()
	}
	if src.Type().AssignableTo(dst.Type()) {
		dst.Set(src)
		return nil
	}
	if src.Type().ConvertibleTo(dst.Type()) && simpleKind(src.Kind()) && simpleKind(dst.Kind()) {
		dst.Set(src.Convert(dst.Type()))
		return nil
	}
	if isSQLNullString(dst.Type()) {
		dst.Set(sqlNullString(stringValue(src), validValue(src)))
		return nil
	}
	if isPGText(dst.Type()) {
		dst.Set(pgText(stringValue(src), validValue(src)))
		return nil
	}
	if isPGUUID(dst.Type()) {
		dst.Set(pgUUID(stringValue(src), validValue(src)))
		return nil
	}
	if isPGTimestamptz(dst.Type()) {
		dst.Set(pgTimestamptz(timeValue(src), validValue(src)))
		return nil
	}
	switch dst.Kind() {
	case reflect.String:
		dst.SetString(stringValue(src))
		return nil
	case reflect.Bool:
		dst.SetBool(boolValue(src))
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		dst.SetInt(intValue(src))
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value := intValue(src)
		if value < 0 {
			value = 0
		}
		dst.SetUint(uint64(value))
		return nil
	case reflect.Slice:
		if dst.Type().Elem().Kind() == reflect.Uint8 {
			dst.SetBytes([]byte(stringValue(src)))
			return nil
		}
		if src.Kind() != reflect.Slice {
			return fmt.Errorf("cannot assign %s to slice %s", src.Type(), dst.Type())
		}
		items := reflect.MakeSlice(dst.Type(), 0, src.Len())
		for i := 0; i < src.Len(); i++ {
			item := reflect.New(dst.Type().Elem()).Elem()
			if err := assignValue(src.Index(i), item); err != nil {
				return err
			}
			items = reflect.Append(items, item)
		}
		dst.Set(items)
		return nil
	case reflect.Struct:
		return assignStruct(src, dst)
	}
	return fmt.Errorf("cannot assign %s to %s", src.Type(), dst.Type())
}

func assignStruct(src reflect.Value, dst reflect.Value) error {
	if src.Kind() != reflect.Struct {
		return fmt.Errorf("cannot assign %s to struct %s", src.Type(), dst.Type())
	}
	srcFields := fieldsByJSON(src.Type())
	for i := 0; i < dst.NumField(); i++ {
		dstField := dst.Type().Field(i)
		key := jsonKey(dstField)
		if key == "" {
			continue
		}
		srcIndex, ok := srcFields[key]
		if !ok {
			continue
		}
		if err := assignValue(src.Field(srcIndex), dst.Field(i)); err != nil {
			return fmt.Errorf("%s: %w", dstField.Name, err)
		}
	}
	return nil
}

func fieldsByJSON(t reflect.Type) map[string]int {
	out := make(map[string]int, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		if key := jsonKey(t.Field(i)); key != "" {
			out[key] = i
		}
	}
	return out
}

func jsonKey(field reflect.StructField) string {
	if field.PkgPath != "" {
		return ""
	}
	tag := field.Tag.Get("json")
	if tag == "-" {
		return ""
	}
	if tag != "" {
		return strings.Split(tag, ",")[0]
	}
	return strings.ToLower(field.Name)
}

func simpleKind(kind reflect.Kind) bool {
	switch kind {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64, reflect.String:
		return true
	default:
		return false
	}
}

func validValue(value reflect.Value) bool {
	if !value.IsValid() {
		return false
	}
	if value.Kind() == reflect.Pointer {
		return !value.IsNil()
	}
	if value.Type() == reflect.TypeOf(sql.NullString{}) {
		return value.FieldByName("Valid").Bool()
	}
	if isPGText(value.Type()) || isPGUUID(value.Type()) || isPGTimestamptz(value.Type()) {
		return value.FieldByName("Valid").Bool()
	}
	return true
}

func stringValue(value reflect.Value) string {
	if !value.IsValid() {
		return ""
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return ""
		}
		value = value.Elem()
	}
	if value.Type() == reflect.TypeOf(sql.NullString{}) {
		if !value.FieldByName("Valid").Bool() {
			return ""
		}
		return value.FieldByName("String").String()
	}
	if isPGText(value.Type()) {
		if !value.FieldByName("Valid").Bool() {
			return ""
		}
		return value.FieldByName("String").String()
	}
	if isPGUUID(value.Type()) {
		if !value.FieldByName("Valid").Bool() {
			return ""
		}
		if stringer, ok := value.Interface().(fmt.Stringer); ok {
			return stringer.String()
		}
	}
	if isPGTimestamptz(value.Type()) {
		if !value.FieldByName("Valid").Bool() {
			return ""
		}
		return value.FieldByName("Time").Interface().(time.Time).UTC().Format(time.RFC3339Nano)
	}
	switch value.Kind() {
	case reflect.String:
		return value.String()
	case reflect.Slice:
		if value.Type().Elem().Kind() == reflect.Uint8 {
			return string(value.Bytes())
		}
	case reflect.Bool:
		if value.Bool() {
			return "true"
		}
		return "false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(value.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(value.Uint(), 10)
	}
	return fmt.Sprint(value.Interface())
}

func boolValue(value reflect.Value) bool {
	if !value.IsValid() {
		return false
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return false
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.Bool:
		return value.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int() != 0
	case reflect.String:
		parsed, _ := strconv.ParseBool(value.String())
		return parsed
	}
	return stringValue(value) == "1"
}

func intValue(value reflect.Value) int64 {
	if !value.IsValid() {
		return 0
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return 0
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.Bool:
		if value.Bool() {
			return 1
		}
		return 0
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		unsigned := value.Uint()
		if unsigned > math.MaxInt64 {
			return 0
		}
		return int64(unsigned) // #nosec G115 -- guarded by MaxInt64 check above.
	case reflect.String:
		parsed, _ := strconv.ParseInt(value.String(), 10, 64)
		return parsed
	}
	return 0
}

func timeValue(value reflect.Value) time.Time {
	if !value.IsValid() {
		return time.Time{}
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return time.Time{}
		}
		value = value.Elem()
	}
	if value.Type() == reflect.TypeOf(time.Time{}) {
		return value.Interface().(time.Time)
	}
	if isPGTimestamptz(value.Type()) {
		if !value.FieldByName("Valid").Bool() {
			return time.Time{}
		}
		return value.FieldByName("Time").Interface().(time.Time)
	}
	raw := stringValue(value)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func isPGText(t reflect.Type) bool {
	return t.PkgPath() == "github.com/jackc/pgx/v5/pgtype" && t.Name() == "Text"
}

func isSQLNullString(t reflect.Type) bool {
	return t == reflect.TypeOf(sql.NullString{})
}

func isPGUUID(t reflect.Type) bool {
	return t.PkgPath() == "github.com/jackc/pgx/v5/pgtype" && t.Name() == "UUID"
}

func isPGTimestamptz(t reflect.Type) bool {
	return t.PkgPath() == "github.com/jackc/pgx/v5/pgtype" && t.Name() == "Timestamptz"
}

func sqlNullString(value string, valid bool) reflect.Value {
	return reflect.ValueOf(sql.NullString{String: value, Valid: valid})
}

func pgText(value string, valid bool) reflect.Value {
	return reflect.ValueOf(pgtype.Text{String: value, Valid: valid})
}

func pgUUID(value string, valid bool) reflect.Value {
	var id pgtype.UUID
	if valid {
		_ = id.Scan(value)
	}
	return reflect.ValueOf(id)
}

func pgTimestamptz(value time.Time, valid bool) reflect.Value {
	return reflect.ValueOf(pgtype.Timestamptz{Time: value, Valid: valid && !value.IsZero()})
}
