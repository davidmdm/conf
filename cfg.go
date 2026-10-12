package conf

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
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

			if field.opts.unmarshaller.unmarshal != nil {
				result := reflect.New(field.opts.unmarshaller.propType)
				if err := field.opts.unmarshaller.unmarshal([]byte(text), result.Interface()); err != nil {
					return err
				}
				field.value.Set(result.Elem().Interface())
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

func (parser Parser) Var[T any](p *T, name string, opts ...Option[T]) {
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

	if flag != "" && len(m[flag]) == 0 {
		m[flag] = []string{"true"}
	}

	return func(name string) (string, bool) {
		name = strings.ToLower(name)
		if value, ok := m[name]; ok {
			return strings.Join(value, ","), ok
		}
		name = strings.ReplaceAll(name, "_", "-")
		value, ok := m[name]
		return strings.Join(value, ","), ok
	}
}

type fsoptions struct {
	AlwaysRelative bool
}

type FSOption func(*fsoptions)

// FSAlwaysRelative when true will trim any leading slashes before performing any path lookups against an FS.
// An example of this being useful is to facilitate working with os.Root against the "/". Users will often want to
// configure with absolute paths and this allows an os.Root at "/" to accept absolute paths.
// It can also allow absolute paths to refer to the root when not centered at "/".
func FSAlwaysRelative(value bool) FSOption {
	return func(f *fsoptions) {
		f.AlwaysRelative = value
	}
}

func FS(filesystem fs.FS, opts ...FSOption) LookupFunc {
	var options fsoptions
	for _, apply := range opts {
		apply(&options)
	}
	return func(path string) (string, bool) {
		if options.AlwaysRelative {
			path = strings.TrimLeftFunc(path, func(r rune) bool { return r == filepath.Separator })
		}
		file, err := filesystem.Open(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return "", false
			}
			panic(err)
		}
		defer func() {
			if err := file.Close(); err != nil {
				panic(xerr.Join(recover().(error), err))
			}
		}()

		data, err := io.ReadAll(file)
		if err != nil {
			panic(err)
		}

		return string(data), true
	}
}

// FSFromRootPath creates a lookup func for a FS rooted at path.
// Absolute filepaths are treated relative to the root.
func FSFromRootPath(path string) (LookupFunc, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open root fs: %w", err)
	}
	return FS(root.FS(), FSAlwaysRelative(true)), nil
}

func MustFSFromRootPath(path string) LookupFunc {
	fn, err := FSFromRootPath(path)
	if err != nil {
		panic(err)
	}
	return fn
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
