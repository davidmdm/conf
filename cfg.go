package conf

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/davidmdm/x/xerr"
)

type LookupFunc func(string) (string, bool)

type field struct {
	value value
	opts  options
}

type Parser struct {
	fields map[string]field
	lookup LookupFunc
}

func MakeParser(funcs ...LookupFunc) Parser {
	lookupFuncs := make([]LookupFunc, 0, len(funcs))
	for _, fn := range funcs {
		if fn == nil {
			continue
		}
		lookupFuncs = append(lookupFuncs, fn)
	}

	lookup := os.LookupEnv
	if len(lookupFuncs) > 0 {
		lookup = joinLookupFuncs(lookupFuncs...)
	}

	return Parser{
		fields: make(map[string]field),
		lookup: lookup,
	}
}

func (parser Parser) Parse() error {
	errs := make([]error, 0, len(parser.fields))
	for name, field := range parser.fields {
		if err := func() (err error) {
			defer func() {
				if recovered := recover(); recovered != nil {
					err = fmt.Errorf("%v", recovered)
				}
			}()

			text, ok := parser.lookup(name)
			if ok && text == "" {
				switch {
				case field.opts.nonEmpty:
					return fmt.Errorf("field is declared but empty: cannot be empty")
				case field.opts.skipEmpty:
					return
				}
			}

			if !ok {
				if field.opts.required {
					return errors.New("field is required")
				}
				if field.opts.fallback != nil {
					field.value.Set(field.opts.fallback)
				}
				return nil
			}

			return field.value.Parse(text)
		}(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}

	return xerr.MultiErrOrderedFrom("failed to parse variable(s)", errs...)
}

// MustParse is like Parse but panics if an error occurs
func (parser Parser) MustParse() {
	if err := parser.Parse(); err != nil {
		panic(err)
	}
}

func Var[T any](parser Parser, p *T, name string, opts ...Option[T]) {
	var options options
	for _, apply := range opts {
		apply(&options)
	}
	parser.fields[name] = field{
		value: genericValue[T]{p},
		opts:  options,
	}
}

// CommandLineArgs returns a lookup function that will search the provided args for flags.
// Since we often want our EnvironmentVariable name declarations to be reusable for command line args
// the lookup is case-insensitive and all underscores are changes to dashes.
// For example, a variable mapped to DATABASE_URL can be found using the --database-url flag when working with CommandLineArgs.
func CommandLineArgs(args ...string) LookupFunc {
	if len(args) == 0 {
		args = os.Args[1:]
	}

	var (
		m    = map[string][]string{}
		flag = ""
	)

	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "-"):
			if flag != "" && len(m[flag]) == 0 {
				m[flag] = []string{"true"}
			}
			flag = strings.ToLower(strings.TrimLeft(arg, "-"))
			if key, value, ok := strings.Cut(flag, "="); ok {
				m[key] = append(m[key], value)
				flag = ""
			}
		case flag == "":
			// skip positional args
		default:
			m[flag] = append(m[flag], arg)
			flag = ""
		}
	}

	return func(name string) (string, bool) {
		name = strings.ReplaceAll(strings.ToLower(name), "_", "-")
		value, ok := m[name]
		return strings.Join(value, ","), ok
	}
}

type FileSystemOptions struct {
	Base string
}

func FileSystem(opts FileSystemOptions) LookupFunc {
	if opts.Base == "" {
		opts.Base = "."
	}
	return func(path string) (string, bool) {
		if !filepath.IsAbs(path) {
			path = filepath.Join(opts.Base, path)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				panic(err)
			}
			return "", false
		}

		return string(data), true
	}
}

func joinLookupFuncs(fns ...LookupFunc) LookupFunc {
	return func(key string) (value string, ok bool) {
		for _, fn := range fns {
			value, ok = fn(key)
			if ok {
				return
			}
		}
		return
	}
}

var Environ = MakeParser(os.LookupEnv)
