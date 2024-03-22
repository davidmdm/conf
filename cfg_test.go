package conf_test

import (
	"encoding"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/davidmdm/conf"
	"github.com/stretchr/testify/require"
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
		}
		return func(s string) (string, bool) {
			value, ok := e[s]
			return value, ok
		}
	}())

	conf.Var(environment, &config.int, "i")
	conf.Var(environment, &config.uint, "ui")
	conf.Var(environment, &config.float64, "f")
	conf.Var(environment, &config.boolean, "b")
	conf.Var(environment, &config.string, "s")
	conf.Var(environment, &config.duration, "d")
	conf.Var(environment, &config.stringslice, "ss")
	conf.Var(environment, &config.custom, "custom")
	conf.Var(environment, &config.mapint, "mapint")

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

	conf.Var(parser, &required, "required", conf.Required[string](true))
	conf.Var(parser, &nonempty, "nonEmpty", conf.NonEmpty[string](true))
	conf.Var(parser, &nonemptyNotPresent, "emptynonpresent", conf.NonEmpty[string](true))
	// This would fail when parsing if not skipped
	conf.Var(parser, &skipEmpty, "skipEmpty", conf.SkipEmpty[int](true))

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
	conf.Var(e1, &boolint, "BOOLINT")

	var intbool map[int]bool
	conf.Var(e1, &intbool, "INTBOOL")

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

	conf.Var(parser, &x, "GOOD")
	conf.Var(parser, &x, "bad")
	conf.Var(parser, &x, "lower")

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
	conf.Var(environment, &text, "VAR")

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
	conf.Var(environment, &text, "VAR")

	require.NoError(t, environment.Parse())
	require.Equal(t, "hello world\n", string(text))
}

func TestCommandLineArgsSource(t *testing.T) {
	environment := conf.MakeParser(
		conf.CommandLineArgs("--database-url", "db", "-force", "--filter=*.sql", "-count", "42", "-input", "a", "--input", "b"),
	)

	var (
		databaseURL string
		force       bool
		filter      string
		count       int
		other       string
		input       []string
	)

	conf.Var(environment, &databaseURL, "DATABASE_URL")
	conf.Var(environment, &force, "FORCE")
	conf.Var(environment, &filter, "FILTER")
	conf.Var(environment, &count, "COUNT")
	conf.Var(environment, &other, "OTHER")
	conf.Var(environment, &input, "INPUT")

	require.NoError(t, environment.Parse())

	require.True(t, force)
	require.Equal(t, "db", databaseURL)
	require.Equal(t, "*.sql", filter)
	require.Equal(t, 42, count)
	require.Equal(t, "", other)
	require.Equal(t, []string{"a", "b"}, input)
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

	conf.Var(environment, &max, "MAX")
	conf.Var(environment, &name, "NAME")

	require.NoError(t, environment.Parse())

	require.Equal(t, 42, max)
	require.Equal(t, "bob", name)
}

func TestFileSystem(t *testing.T) {
	require.NoError(t, os.RemoveAll("./test_output"))
	require.NoError(t, os.MkdirAll("./test_output", 0o755))

	require.NoError(t, os.WriteFile("./test_output/secret.txt", []byte("hello world"), 0o644))

	fs := conf.MakeParser(conf.FileSystem(conf.FileSystemOptions{Base: "./test_output"}))

	var secret string
	conf.Var(fs, &secret, "secret.txt")

	require.NoError(t, fs.Parse())
	require.Equal(t, "hello world", secret)
}

func TestInvalidDestination(t *testing.T) {
	var invalid struct{ string }
	parser := conf.MakeParser(func(s string) (string, bool) { return "VALUE", true })

	conf.Var(parser, &invalid, "NAME")

	require.EqualError(t, parser.Parse(), "failed to parse variable(s): NAME: destination type not supported: struct { string }")
}
