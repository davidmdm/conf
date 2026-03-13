package conf

import "reflect"

type options struct {
	required  bool
	nonEmpty  bool
	skipEmpty bool
	fallback  any
	json      reflect.Type
}

type Option[T any] func(*options)

func Required[T any](value bool) Option[T] {
	return func(o *options) {
		o.required = value
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

func JSON[T any](o *options) {
	o.json = reflect.TypeFor[T]()
}
