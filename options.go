package conf

type options struct {
	required bool
	fallback any
}

type Options[T any] struct {
	Required     bool
	DefaultValue T
}

func (opts Options[T]) toOptions() options {
	return options{
		required: opts.Required,
		fallback: opts.DefaultValue,
	}
}

type multiOpts[T any] []Options[T]

func (opts multiOpts[T]) toOptions() options {
	if len(opts) == 0 {
		var zero T
		return options{fallback: zero}
	}
	return opts[0].toOptions()
}
