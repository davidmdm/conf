package conf

type options struct {
	required bool
	fallback any
}

type typedOption[T any] struct {
	Required     bool
	DefaultValue T
}

type Option[T any] func(*typedOption[T])

func Required[T any](value bool) Option[T] {
	return func(o *typedOption[T]) {
		o.Required = value
	}
}

func Default[T any](value T) Option[T] {
	return func(o *typedOption[T]) {
		o.DefaultValue = value
	}
}

func (opts typedOption[T]) toOptions() options {
	return options{
		required: opts.Required,
		fallback: opts.DefaultValue,
	}
}
