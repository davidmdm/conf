package conf_test

import (
	"encoding"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"
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
		conf.CommandLineArgs("--database-url", "db", "-force", "--filter=*.sql", "-count", "42", "-input", "a", "--input", "b", "--rare_underscore"),
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
	require.Equal(t, "*.sql", filter)
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
