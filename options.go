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

// Option configures how a variable registered with Parser.Var is parsed.
type Option[T any] func(*options)

// Required makes Parse fail if the variable is not found.
func Required[T any](value bool) Option[T] {
	return func(o *options) {
		o.required = value
	}
}

// Desc sets the variable's description shown by Parser.Help.
func Desc[T any](text string) Option[T] {
	return func(o *options) {
		o.desc = text
	}
}

// NonEmpty makes Parse fail if the variable is found but empty.
func NonEmpty[T any](value bool) Option[T] {
	return func(o *options) {
		o.nonEmpty = value
	}
}

// RequiredNonEmpty is shorthand for Required(true) and NonEmpty(true).
func RequiredNonEmpty[T any]() Option[T] {
	return func(o *options) {
		o.required = true
		o.nonEmpty = true
	}
}

// SkipEmpty makes Parse ignore the variable when it is found but empty, leaving its value unchanged.
// If this option is not used, and the variable is found and empty, the value will be parsed from the empty string.
// If parsing from the empty string is invalid, use this option to skip. Note that `conf.Default` will have no effect if this option is set.
// To use the default value the property must not be found.
func SkipEmpty[T any](value bool) Option[T] {
	return func(o *options) {
		o.skipEmpty = value
	}
}

// Default sets the value used when the variable is not found.
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

// WithValidator adds a validation function that runs after the variable is parsed or unmarshalled.
// It can be used multiple times to add several validators.
func WithValidator[T any](fn func(T) error) Option[T] {
	return func(o *options) {
		o.validators = append(o.validators, func(value any) error {
			return fn(value.(T))
		})
	}
}
