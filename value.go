package conf

import (
	"encoding"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type value interface {
	Parse(string) error
	Set(any)
}

type genericValue[T any] struct{ dst *T }

func (v genericValue[T]) Set(value any) {
	*v.dst = value.(T)
}

func (v genericValue[T]) Parse(text string) error {
	return parse(reflect.ValueOf(v.dst), text, true)
}

func parse(v reflect.Value, text string, topLevel bool) error {
	t := v.Type()

	for t.Kind() == reflect.Pointer {
		if v.IsNil() {
			v.Set(reflect.New(t.Elem()))
		}

		if unmarshaler, ok := v.Interface().(encoding.TextUnmarshaler); ok {
			return unmarshaler.UnmarshalText([]byte(text))
		}

		if unmarshaler, ok := v.Interface().(encoding.BinaryUnmarshaler); ok {
			return unmarshaler.UnmarshalBinary([]byte(text))
		}

		t = t.Elem()
		v = v.Elem()
	}

	switch t.Kind() {
	case reflect.String:
		v.SetString(text)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v.Kind() == reflect.Int64 && t.PkgPath() == "time" && t.Name() == "Duration" {
			d, err := time.ParseDuration(text)
			if err != nil {
				return err
			}
			v.SetInt(int64(d))
			break
		}

		val, err := strconv.ParseInt(text, 0, t.Bits())
		if err != nil {
			return err
		}
		v.SetInt(val)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		val, err := strconv.ParseUint(text, 0, t.Bits())
		if err != nil {
			return err
		}
		v.SetUint(val)
	case reflect.Bool:
		val, err := strconv.ParseBool(text)
		if err != nil {
			return err
		}
		v.SetBool(val)
	case reflect.Float32, reflect.Float64:
		val, err := strconv.ParseFloat(text, t.Bits())
		if err != nil {
			return err
		}
		v.SetFloat(val)
	case reflect.Slice:
		if !topLevel {
			return fmt.Errorf("cannot support deep slices")
		}

		if t.Elem().Kind() == reflect.Uint8 {
			v.Set(reflect.ValueOf([]byte(text)))
			return nil
		}

		if strings.TrimSpace(text) == "" {
			v.Set(reflect.MakeSlice(t, 0, 0))
			return nil
		}

		items := strings.Split(text, ",")
		slice := reflect.MakeSlice(t, len(items), len(items))
		for i, subtext := range items {
			if err := parse(slice.Index(i), subtext, false); err != nil {
				return err
			}
		}

		v.Set(slice)

	case reflect.Map:
		if !topLevel {
			return fmt.Errorf("cannot support deep maps")
		}

		text = strings.TrimSpace(text)
		if text == "" {
			return nil
		}

		target := reflect.MakeMap(t)
		for _, elem := range strings.Split(text, ",") {
			key, value, ok := strings.Cut(elem, "=")
			if !ok {
				continue
			}
			k := reflect.New(t.Key()).Elem()
			if err := parse(k, key, false); err != nil {
				return fmt.Errorf("failed to parse key: %s: %w", key, err)
			}

			v := reflect.New(t.Elem()).Elem()
			if err := parse(v, value, false); err != nil {
				return fmt.Errorf("failed to parse value at key: %s: %w", key, err)
			}

			target.SetMapIndex(k, v)
		}

		v.Set(target)

	default:
		return fmt.Errorf("destination type not supported: %s", t)
	}

	return nil
}
