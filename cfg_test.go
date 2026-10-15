package conf_test

import (
	"cmp"
	"encoding"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/davidmdm/conf"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type Custom struct {
	Value any
}

func (c *Custom) UnmarshalText(data []byte) error {
	return json.Unmarshal(data, &c.Value)
}

func TestVar(t *testing.T) {
	var config struct {
		int         int
		uint        uint
		float64     float64
		boolean     bool
		duration    time.Duration
		string      string
		stringslice []string
		custom      Custom
		mapint      map[string]int
		url         url.URL
	}

	environment := conf.MakeParser(func() conf.LookupFunc {
		e := map[string]string{
			"i":      "-1",
			"ui":     "1",
			"f":      "3.14",
			"b":      "T",
			"d":      "5m",
			"s":      "hello",
			"ss":     "hello,world",
			"custom": `[1,2,3]`,
			"mapint": `x=3,y=1`,
			"url":    "https://examples.com",
		}
		return func(s string) (string, bool) {
			value, ok := e[s]
			return value, ok
		}
	}())

	environment.Var(&config.int, "i")
	environment.Var(&config.uint, "ui")
	environment.Var(&config.float64, "f")
	environment.Var(&config.boolean, "b")
	environment.Var(&config.string, "s")
	environment.Var(&config.duration, "d")
	environment.Var(&config.stringslice, "ss")
	environment.Var(&config.custom, "custom")
	environment.Var(&config.mapint, "mapint")
	environment.Var(&config.url, "url")

	require.NoError(t, environment.Parse())

	require.Equal(t, -1, config.int)
	require.Equal(t, uint(1), config.uint)
	require.Equal(t, 3.14, config.float64)
	require.Equal(t, true, config.boolean)
	require.Equal(t, "hello", config.string)
	require.Equal(t, []string{"hello", "world"}, config.stringslice)
	require.Equal(t, 5*time.Minute, config.duration)
	require.EqualValues(t, []any{1.0, 2.0, 3.0}, config.custom.Value)
	require.EqualValues(t, map[string]int{"x": 3, "y": 1}, config.mapint)
	require.Equal(t, "https://examples.com", config.url.String())
	require.Equal(t, "https", config.url.Scheme)
	require.Equal(t, "examples.com", config.url.Host)
}

func TestOptions(t *testing.T) {
	parser := conf.MakeParser(func() conf.LookupFunc {
		m := map[string]string{
			"nonEmpty":  "",
			"skipEmpty": "",
		}
		return func(s string) (string, bool) {
			value, ok := m[s]
			return value, ok
		}
	}())

	var required string
	var nonempty string
	var nonemptyNotPresent string
	var skipEmpty int

	parser.Var(&required, "required", conf.Required[string](true))
	parser.Var(&nonempty, "nonEmpty", conf.NonEmpty[string](true))
	parser.Var(&nonemptyNotPresent, "emptynonpresent", conf.NonEmpty[string](true))
	// This would fail when parsing if not skipped
	parser.Var(&skipEmpty, "skipEmpty", conf.SkipEmpty[int](true))

	require.EqualError(
		t,
		parser.Parse(),
		"failed to parse variable(s):\n  - nonEmpty: field is declared but empty: cannot be empty\n  - required: field is required",
	)
}

func TestMapParsingErrors(t *testing.T) {
	e1 := conf.MakeParser(func(name string) (string, bool) {
		return "3=4", true
	})

	var boolint map[bool]int
	e1.Var(&boolint, "BOOLINT")

	var intbool map[int]bool
	e1.Var(&intbool, "INTBOOL")

	errText := e1.Parse().Error()
	require.Contains(
		t,
		errText,
		"failed to parse variable(s):\n  - BOOLINT: failed to parse key: 3: strconv.ParseBool: parsing \"3\": invalid syntax\n  - INTBOOL: failed to parse value at key: 3: strconv.ParseBool: parsing \"4\": invalid syntax",
	)
}

func TestParsePanicRecovery(t *testing.T) {
	parser := conf.MakeParser(func(s string) (string, bool) {
		if s != strings.ToUpper(s) {
			panic("lookup key not capitalized")
		}
		return s, true
	})

	var x string

	parser.Var(&x, "GOOD")
	parser.Var(&x, "bad")
	parser.Var(&x, "lower")

	require.EqualError(
		t,
		parser.Parse(),
		"failed to parse variable(s):\n  - bad: lookup key not capitalized\n  - lower: lookup key not capitalized",
	)
}

type CapText string

var _ encoding.TextUnmarshaler = new(CapText)

func (text *CapText) UnmarshalText(data []byte) error {
	*text = CapText(strings.ToUpper(string(data)))
	return nil
}

func TestTextUnmarshaler(t *testing.T) {
	var text CapText

	environment := conf.MakeParser(func(s string) (string, bool) { return "value", true })
	environment.Var(&text, "VAR")

	require.NoError(t, environment.Parse())
	require.Equal(t, "VALUE", string(text))
}

type Base64Text string

var _ encoding.BinaryUnmarshaler = new(Base64Text)

func (text *Base64Text) UnmarshalBinary(data []byte) error {
	result, err := base64.RawStdEncoding.DecodeString(string(data))
	if err != nil {
		return err
	}
	*text = Base64Text(result)
	return nil
}

func TestBinaryUnmarshaler(t *testing.T) {
	var text Base64Text

	environment := conf.MakeParser(func(s string) (string, bool) { return "aGVsbG8gd29ybGQK", true })
	environment.Var(&text, "VAR")

	require.NoError(t, environment.Parse())
	require.Equal(t, "hello world\n", string(text))
}

func TestCommandLineArgsSource(t *testing.T) {
	commandline := conf.MakeParser(
		conf.CommandLineArgs("--database-url", "db", "-force", "--filter=FOO.*.sql", "-count", "42", "-input", "a", "--input", "b", "--rare_underscore"),
	)

	var (
		databaseURL    string
		force          bool
		filter         string
		count          int
		other          string
		input          []string
		rareUnderscore bool
	)

	commandline.Var(&databaseURL, "DATABASE_URL")
	commandline.Var(&force, "FORCE")
	commandline.Var(&filter, "FILTER")
	commandline.Var(&count, "COUNT")
	commandline.Var(&other, "OTHER")
	commandline.Var(&input, "INPUT")
	commandline.Var(&rareUnderscore, "RARE_UNDERSCORE")

	require.NoError(t, commandline.Parse())

	require.True(t, force)
	require.Equal(t, "db", databaseURL)
	require.Equal(t, "FOO.*.sql", filter)
	require.Equal(t, 42, count)
	require.Equal(t, "", other)
	require.Equal(t, []string{"a", "b"}, input)
	require.True(t, rareUnderscore)
}

func TestMultipleLookups(t *testing.T) {
	environment := conf.MakeParser(
		conf.CommandLineArgs("--max=42"),
		func(name string) (string, bool) {
			value, ok := map[string]string{
				"MAX":  "10",
				"NAME": "bob",
			}[name]
			return value, ok
		},
	)

	var (
		max  int
		name string
	)

	environment.Var(&max, "MAX")
	environment.Var(&name, "NAME")

	require.NoError(t, environment.Parse())

	require.Equal(t, 42, max)
	require.Equal(t, "bob", name)
}

func TestFileSystem(t *testing.T) {
	require.NoError(t, os.RemoveAll("./test_output"))
	require.NoError(t, os.MkdirAll("./test_output", 0o755))

	require.NoError(t, os.WriteFile("./test_output/secret.txt", []byte("hello world"), 0o644))

	fs := conf.MakeParser(conf.MustFSFromRootPath("./test_output"))

	var secret string
	fs.Var(&secret, "secret.txt")

	require.NoError(t, fs.Parse())
	require.Equal(t, "hello world", secret)
}

func TestFromRoot(t *testing.T) {
	rootFS, err := conf.FSFromRootPath(".")
	require.NoError(t, err)

	parser := conf.MakeParser(rootFS)

	var (
		value string
		gomod string
	)
	parser.Var(&value, "outside.root.txt")
	parser.Var(&gomod, "/go.mod", conf.RequiredNonEmpty[string]())

	require.NoError(t, parser.Parse())
}

func TestInvalidDestination(t *testing.T) {
	var invalid struct{ string }
	parser := conf.MakeParser(func(s string) (string, bool) { return "VALUE", true })

	parser.Var(&invalid, "NAME")

	require.EqualError(t, parser.Parse(), "failed to parse variable(s): NAME: destination type not supported: struct { string }")
}

func TestPointerToURL(t *testing.T) {
	parser := conf.MakeParser(func(s string) (string, bool) { return "http://localhost:3000", true })

	var url *url.URL
	parser.Var(&url, "...")

	require.NoError(t, parser.Parse())
	require.Equal(t, "http://localhost:3000", url.String())
}

func TestJsonOption(t *testing.T) {
	parser := conf.MakeParser(func(s string) (string, bool) { return `[1,2,3]`, true })

	var ints []int
	parser.Var(&ints, "...", conf.JSON[[]int])

	require.NoError(t, parser.Parse())
	require.Equal(t, []int{1, 2, 3}, ints)
}

func TestYamlUnmarshaller(t *testing.T) {
	parser := conf.MakeParser(func(s string) (string, bool) {
		const doc = `---
hello: world
foo: bar`
		return doc, true
	})

	var data map[string]any
	parser.Var(&data, "...", conf.WithUnmarshaller[map[string]any](yaml.Unmarshal))

	require.NoError(t, parser.Parse())

	require.Equal(t, "world", data["hello"])
	require.Equal(t, "bar", data["foo"])
}

func TestWithValidators(t *testing.T) {
	parser := conf.MakeParser(func(s string) (string, bool) { return `[1,2,3]`, true })

	var ints []int
	parser.Var(
		&ints,
		"...",
		conf.JSON[[]int],
		conf.WithValidator(func(s []int) error {
			if len(s) < 5 {
				return fmt.Errorf("requires at least 5 elements")
			}
			return nil
		}),
		conf.WithValidator(func(s []int) error {
			for _, value := range s {
				if value%2 == 0 {
					return fmt.Errorf("cannot contain even values but got: %d", value)
				}
			}
			return nil
		}),
	)

	err := parser.Parse()
	require.ErrorContains(t, err, "requires at least 5 elements", err.Error())
	require.ErrorContains(t, err, "cannot contain even values but got: 2", err.Error())
}

// register returns a func that declares a variable of type T named VAR on the parser,
// and returns a getter for its value after parsing.
func register[T any](opts ...conf.Option[T]) func(conf.Parser) func() any {
	return func(p conf.Parser) func() any {
		var x T
		p.Var(&x, "VAR", opts...)
		return func() any { return x }
	}
}

func TestParse(t *testing.T) {
	invalid := conf.WithValidator(func(int) error { return errors.New("invalid") })

	cases := []struct {
		Name    string
		Value   string
		Missing bool
		Run     func(conf.Parser) func() any
		Want    any
		Err     string
	}{
		{
			Name:  "hex int",
			Value: "0x10",
			Run:   register[int](),
			Want:  16,
		},
		{
			Name:  "int8 overflow",
			Value: "128",
			Run:   register[int8](),
			Err:   "value out of range",
		},
		{
			Name:  "invalid int",
			Value: "x",
			Run:   register[int](),
			Err:   "invalid syntax",
		},
		{
			Name:  "negative uint",
			Value: "-1",
			Run:   register[uint](),
			Err:   "invalid syntax",
		},
		{
			Name:  "float32",
			Value: "1.5",
			Run:   register[float32](),
			Want:  float32(1.5),
		},
		{
			Name:  "invalid float",
			Value: "x",
			Run:   register[float64](),
			Err:   "invalid syntax",
		},
		{
			Name:  "invalid bool",
			Value: "maybe",
			Run:   register[bool](),
			Err:   "invalid syntax",
		},
		{
			Name:  "invalid duration",
			Value: "5x",
			Run:   register[time.Duration](),
			Err:   `unknown unit "x"`,
		},
		{
			Name:  "pointer",
			Value: "5",
			Run:   register[*int](),
			Want:  new(5),
		},
		{
			Name:  "bytes",
			Value: "hello",
			Run:   register[[]byte](),
			Want:  []byte("hello"),
		},
		{
			Name:  "int slice",
			Value: "1,2,3",
			Run:   register[[]int](),
			Want:  []int{1, 2, 3},
		},
		{Name: "blank slice", Value: " ", Run: register[[]int](), Want: []int{}},
		{
			Name:  "invalid slice item",
			Value: "1,x",
			Run:   register[[]int](),
			Err:   "invalid syntax",
		},
		{
			Name:  "deep slice",
			Value: "1",
			Run:   register[[][]int](),
			Err:   "cannot support deep slices",
		},
		{
			Name:  "blank map",
			Value: " ",
			Run:   register[map[string]int](),
			Want:  map[string]int(nil),
		},
		{
			Name:  "map skips entries without =",
			Value: "a=1,b",
			Run:   register[map[string]int](),
			Want:  map[string]int{"a": 1},
		},
		{
			Name:  "deep map",
			Value: "a=1",
			Run:   register[[]map[string]int](),
			Err:   "cannot support deep maps",
		},
		{
			Name:    "default when missing",
			Missing: true,
			Run:     register(conf.Default(7)),
			Want:    7,
		},
		{
			Name:  "default ignored when present",
			Value: "1",
			Run:   register(conf.Default(7)),
			Want:  1,
		},
		{
			Name:    "missing stays zero",
			Missing: true,
			Run:     register[int](),
			Want:    0,
		},
		{
			Name:  "required present",
			Value: "x",
			Run:   register(conf.Required[string](true)),
			Want:  "x",
		},
		{
			Name:    "required false",
			Missing: true,
			Run:     register(conf.Required[string](false)),
			Want:    "",
		},
		{
			Name:  "empty string allowed",
			Value: "",
			Run:   register[string](),
			Want:  "",
		},
		{
			Name:    "required non empty missing",
			Missing: true,
			Run:     register(conf.RequiredNonEmpty[string]()),
			Err:     "field is required",
		},
		{
			Name:  "required non empty empty",
			Value: "",
			Run:   register(conf.RequiredNonEmpty[string]()),
			Err:   "cannot be empty",
		},
		{
			Name:  "json v1",
			Value: `{"name":"bob"}`,
			Run:   register(conf.Option[struct{ Name string }](conf.JSONV1[struct{ Name string }])),
			Want:  struct{ Name string }{"bob"},
		},
		{
			Name:  "json invalid",
			Value: `[1,`,
			Run:   register(conf.Option[[]int](conf.JSON[[]int])),
			Err:   "unexpected EOF",
		},
		{
			Name:  "validator passes",
			Value: "1",
			Run:   register(conf.WithValidator(func(int) error { return nil })),
			Want:  1,
		},
		{
			Name:    "validator skipped when missing",
			Missing: true,
			Run:     register(invalid),
			Want:    0,
		},
		{
			Name:    "validator skipped for default",
			Missing: true,
			Run:     register(conf.Default(7), invalid),
			Want:    7,
		},
		{
			Name:  "validator skipped when empty is skipped",
			Value: "",
			Run:   register(conf.SkipEmpty[int](true), invalid),
			Want:  0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			parser := conf.MakeParser(func(string) (string, bool) { return tc.Value, !tc.Missing })
			get := tc.Run(parser)

			err := parser.Parse()
			if tc.Err != "" {
				require.ErrorContains(t, err, tc.Err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.Want, get())
		})
	}
}

func TestMustParse(t *testing.T) {
	parser := conf.MakeParser(func(string) (string, bool) { return "x", true })

	var s string
	parser.Var(&s, "S")
	require.NotPanics(t, parser.MustParse)

	var i int
	parser.Var(&i, "I")
	require.Panics(t, parser.MustParse)
}

func TestHelp(t *testing.T) {
	parser := conf.MakeParser()

	var (
		b string
		a int
	)
	parser.Var(&b, "B")
	parser.Var(&a, "A", conf.Desc[int]("first line\nsecond line"))

	require.Equal(
		t,
		"Configuration Vars:\n  A (type: int)\n    first line\n    second line\n  B (type: string)",
		parser.Help(),
	)
}

func TestCommandLineArgsLookup(t *testing.T) {
	cases := []struct {
		name string
		args []string
		key  string
		want string
		ok   bool
	}{
		{name: "skips positional args", args: []string{"pos", "--a", "1", "pos2"}, key: "A", want: "1", ok: true},
		{name: "case insensitive", args: []string{"--NAME", "bob"}, key: "name", want: "bob", ok: true},
		{name: "empty equals value", args: []string{"--a="}, key: "A", want: "", ok: true},
		{name: "trailing bool", args: []string{"--a", "1", "--b"}, key: "B", want: "true", ok: true},
		{name: "missing", args: []string{"--a", "1"}, key: "B", ok: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value, ok := conf.CommandLineArgs(tc.args...)(tc.key)
			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.want, value)
		})
	}
}

func TestNilLookupsSkipped(t *testing.T) {
	parser := conf.MakeParser(nil, func(string) (string, bool) { return "x", true })

	var s string
	parser.Var(&s, "S")

	require.NoError(t, parser.Parse())
	require.Equal(t, "x", s)
}

func TestFSLookup(t *testing.T) {
	filesystem := fstest.MapFS{"dir/a.txt": {Data: []byte("hello")}}

	cases := []struct {
		name string
		opts []conf.FSOption
		path string
		want string
		ok   bool
	}{
		{name: "relative", path: "dir/a.txt", want: "hello", ok: true},
		{name: "unclean path", path: "dir/../dir/a.txt", want: "hello", ok: true},
		{name: "missing", path: "dir/b.txt", ok: false},
		{name: "absolute without always relative", path: "/dir/a.txt", ok: false},
		{name: "absolute with always relative", opts: []conf.FSOption{conf.FSAlwaysRelative(true)}, path: "/dir/a.txt", want: "hello", ok: true},
		{name: "repeated leading slashes with always relative", opts: []conf.FSOption{conf.FSAlwaysRelative(true)}, path: "//dir/a.txt", want: "hello", ok: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value, ok := conf.FS(filesystem, tc.opts...)(tc.path)
			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.want, value)
		})
	}
}

// faultyFile is a filesystem whose only file fails to read and/or close with the given errors.
type faultyFile struct{ readErr, closeErr error }

func (f faultyFile) Open(string) (fs.File, error) { return f, nil }
func (f faultyFile) Stat() (fs.FileInfo, error)   { return nil, nil }
func (f faultyFile) Read([]byte) (int, error)     { return 0, cmp.Or(f.readErr, io.EOF) }
func (f faultyFile) Close() error                 { return f.closeErr }

func TestFSErrors(t *testing.T) {
	cases := []struct {
		name string
		file faultyFile
		err  string
	}{
		{
			name: "close fails",
			file: faultyFile{closeErr: errors.New("close failed")},
			err:  "failed to parse variable(s): f: close failed",
		},
		{
			name: "read and close fail",
			file: faultyFile{readErr: errors.New("read failed"), closeErr: errors.New("close failed")},
			err:  "failed to parse variable(s): f: errors:\n  - read failed\n  - close failed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parser := conf.MakeParser(conf.FS(tc.file))

			var s string
			parser.Var(&s, "f")

			require.EqualError(t, parser.Parse(), tc.err)
		})
	}
}
