package conf

import (
	jsonv1 "encoding/json"
	"encoding/json/v2"
)

type options struct {
	required   bool
	nonEmpty   bool
	skipEmpty  bool
	fallback   any
	unmarshal  func([]byte, any) error
	desc       string
	validators []func(any) error
}

type Option[T any] func(*options)

func Required[T any](value bool) Option[T] {
	return func(o *options) {
		o.required = value
	}
}

func Desc[T any](text string) Option[T] {
	return func(o *options) {
		o.desc = text
	}
}

func NonEmpty[T any](value bool) Option[T] {
	return func(o *options) {
		o.nonEmpty = value
	}
}

func RequiredNonEmpty[T any]() Option[T] {
	return func(o *options) {
		o.required = true
		o.nonEmpty = true
	}
}

func SkipEmpty[T any](value bool) Option[T] {
	return func(o *options) {
		o.skipEmpty = value
	}
}

func Default[T any](value T) Option[T] {
	return func(o *options) {
		o.fallback = value
	}
}

// WithUnmarshaller lets you customize how you want the raw bytes of your text value unmarshalled into your variable.
func WithUnmarshaller[T any](fn func([]byte, any) error) Option[T] {
	return func(o *options) {
		o.unmarshal = fn
	}
}

// JSON will unmarshal your config variable using encoding/json/v2 with default v2 json options.
// If you need to use json v1 semantics use JSONV1 or use WithUnmarshaller directly.
func JSON[T any](o *options) {
	o.unmarshal = func(data []byte, out any) error {
		return json.Unmarshal(data, out)
	}
}

// JSONV1 is a convenience for calling WithUnmarshaller using json.Unmarshal with v1 semantics.
// If you are looking to use encoding/json/v2 semantics use conf.JSON
func JSONV1[T any](o *options) {
	o.unmarshal = jsonv1.Unmarshal
}

func WithValidator[T any](fn func(T) error) Option[T] {
	return func(o *options) {
		o.validators = append(o.validators, func(value any) error {
			return fn(value.(T))
		})
	}
}
