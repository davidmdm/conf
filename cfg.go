package conf

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"reflect"
	"slices"
	"strings"

	"github.com/davidmdm/x/xerr"
)

// LookupFunc looks up the raw string value for a variable name, reporting whether it was found.
type LookupFunc func(string) (string, bool)

type field struct {
	value value
	opts  options
	typ   reflect.Type
}

// Parser holds a set of registered configuration variables and parses them from its lookup functions.
type Parser struct {
	fields map[string]field
	lookup LookupFunc
}

// MakeParser creates a Parser that looks up values using the given lookup functions, tried in order until one finds the name.
// Nil functions are ignored. If none are provided, os.LookupEnv is used.
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

// Parse looks up and sets every registered variable, applying its options.
// Errors for all variables are collected and returned together.
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

			defer func() {
				if err != nil || len(field.opts.validators) == 0 {
					return
				}
				errs := make([]error, len(field.opts.validators))
				for i, validator := range field.opts.validators {
					errs[i] = validator(field.value.Get())
				}
				err = xerr.MultiErrFrom("failed to validate", errs...)
			}()

			if field.opts.unmarshal != nil {
				result := reflect.New(field.typ)
				if err := field.opts.unmarshal([]byte(text), result.Interface()); err != nil {
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

// Var registers p to be set from the variable name when the parser is parsed.
func (parser Parser) Var[T any](p *T, name string, opts ...Option[T]) {
	var options options
	for _, apply := range opts {
		apply(&options)
	}
	parser.fields[name] = field{
		value: genericValue[T]{p},
		typ:   reflect.TypeFor[T](),
		opts:  options,
	}
}

// Help returns a listing of the registered variables with their types and descriptions, sorted by name.
func (parser Parser) Help() string {
	var builder strings.Builder

	builder.WriteString("Configuration Vars:\n")

	for _, name := range slices.Sorted(maps.Keys(parser.fields)) {
		field := parser.fields[name]
		builder.WriteString(indent(fmt.Sprintf("%s (type: %s)", name, field.typ), 2))
		builder.WriteString("\n")
		if desc := field.opts.desc; desc != "" {
			builder.WriteString(indent(desc, 4))
			builder.WriteString("\n")
		}
	}

	return strings.TrimSpace(builder.String())
}

func indent(value string, num int) string {
	indent := strings.Repeat(" ", num)
	var builder strings.Builder
	for line := range strings.SplitSeq(value, "\n") {
		_, _ = builder.WriteString(indent)
		_, _ = builder.WriteString(line)
		_, _ = builder.WriteString("\n")
	}
	return strings.TrimRight(builder.String(), "\n")
}

// CommandLineArgs returns a lookup function that will search the provided args for flags.
// Since we often want our EnvironmentVariable name declarations to be reusable for command line args
// the lookup is case-insensitive and all underscores are changes to dashes.
// For example, a variable mapped to DATABASE_URL can be found using the --database-url flag when working with CommandLineArgs.
func CommandLineArgs(args ...string) LookupFunc {
	if args == nil {
		args = os.Args[1:]
	}

	var (
		m    = map[string][]string{}
		flag = ""
	)

	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			if flag != "" && len(m[flag]) == 0 {
				m[flag] = []string{"true"}
				flag = ""
			}
			key, value, ok := strings.Cut(arg, "=")
			flag = strings.ToLower(strings.TrimLeft(key, "-"))
			if ok {
				m[flag] = append(m[flag], value)
				flag = ""
			}
			continue
		}
		if flag == "" {
			continue
		}
		m[flag] = append(m[flag], arg)
		flag = ""
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

// FSOption configures the lookup function returned by FS.
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

// FS returns a lookup func that treats variable names as file paths within filesystem and returns the file's contents.
// Since it is backed by an fs.FS, paths must use "/" as the separator regardless of the operating system.
// Paths are cleaned before lookup, and a file that does not exist is reported as not found.
// Any other error opening, reading, or closing the file causes a panic, which Parse reports as an error for that variable.
func FS(filesystem fs.FS, opts ...FSOption) LookupFunc {
	var options fsoptions
	for _, apply := range opts {
		apply(&options)
	}
	return func(filepath string) (string, bool) {
		if options.AlwaysRelative {
			filepath = strings.TrimLeft(filepath, "/")
		}
		file, err := filesystem.Open(path.Clean(filepath))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return "", false
			}
			panic(err)
		}
		defer func() {
			if closeErr := file.Close(); closeErr != nil {
				err, _ := recover().(error)
				panic(xerr.Join(err, closeErr))
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

// MustFSFromRootPath is like FSFromRootPath but panics if an error occurs.
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

// Environ is the default Parser, which looks up variables from the environment.
var Environ = MakeParser(os.LookupEnv)
