package conf

type options struct {
	required  bool
	nonEmpty  bool
	skipEmpty bool
	fallback  any
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
