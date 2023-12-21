# conf - a configuration parsing lib for Go

**Description**: A Go library for parsing environment variables (and any other source) and setting corresponding flags.

## Why

Why another package for parsing the environment? Currently, most popular environment parsing libraries depend on struct tags to map the environment to a structure and provide options like flag requirement or default values when absent.

With `conf` and the use of Go Generics, we can now have a type-safe API that doesn't depend on struct tags and can take advantage of strong typing.

Let's contrast `davidmdm/conf` with a popular environment parsing library `github.com/kelseyhightower/envconfig`:

The envconfig approach is convenient but very sensitive to typos, and the defaults need to be encoded in their string format, which can be error-prone.

```go
package main

import (
    "time"
    "github.com/kelseyhightower/envconfig"
)

type Config struct {
    DatabaseURL string        `envconfig:"DATABASE_URL" required:"true"`
    Timeout     time.Duration `envconfig:"TIMEMOUT" default:"5m"`
}

func main() {
    var cfg Config
    envconfig.Process("", &cfg)
}
```

On the other hand, `davidmdm/conf` does not suffer from these problems. It also has the added benefit of being programmatic instead of static. If we need, environment variable names and options could be determined at runtime instead of statically typed into a struct definition.

```go
package main

import (
    "time"
    "github.com/davidmdm/conf"
)

type Config struct {
    DatabaseURL string
    Timeout     time.Duration
}

func main() {
    var cfg Config
    conf.Var(conf.Environ, &cfg.DatabaseURL, "DATABASE_URL", env.Options[string]{Required: true})
    conf.Var(conf.Environ, &cfg.Timeout, "TIMEOUT", env.Options[time.Duration]{Default: 5 * time.Minute})

    conf.Environ.MustParse()
}
```

## Overview

This Go package provides a flexible and extensible configuration parser that allows you to easily manage configuration settings in your Go applications. The parser supports environment variables, command line arguments, and file system-based configuration, making it adaptable to various deployment scenarios.

## Installation

```bash
go get -u github.com/davidmdm/conf
```

## Features

Environment Variables: Retrieve configuration values from environment variables with the ability to set default values and mark certain configurations as required.

Command Line Arguments: Easily map command line flags to configuration variables, supporting case-insensitivity and automatic conversion of underscores to dashes.

File System Configuration: Load configuration settings from files in the file system, providing a convenient way to manage configuration files.

Multiple Sources: Combine any of the above sources or your own custom functions to lookup strings.

## Usage

Creating a Parser
To get started, create a configuration parser using the MakeParser function:

```go
import "github.com/davidmdm/conf"

// Create a configuration parser with optional lookup functions. By default if no lookup funcs are provided
// the parser will use os.Lookupenv
parser := conf.MakeParser()
```

You can provide one or more lookup functions to the MakeParser function, which will be used to retrieve configuration values.

Defining Configuration Variables
Define your configuration variables using the Var function:

```go
var (
    yourStringVar string
    yourIntVar    int
)

conf.Var(parser, &yourStringVar, "YOUR_STRING_VAR", conf.Options[string]{Required: true})
conf.Var(parser, &yourIntVar, "YOUR_INT_VAR", conf.Options[int]{Required: false, DefaultValue: 42})

// In this example, YOUR_STRING_VAR is a required string variable, and YOUR_INT_VAR is an optional integer variable with a default value of 42.
```

Parsing Configuration
Parse the configuration using the Parse or MustParse methods:

```go
if err := parser.Parse(); err != nil {
// Handle configuration parsing errors
}

// Alternatively, use MustParse to panic on errors
parser.MustParse()
```

## Configuring Lookup Functions

### Environment Variables

The package provides a default parser for environment variables `conf.Environ`.
You can create one yourself:

```go
environ := conf.MakeParser(os.Lookupenv)
```

### Command Line Arguments

Create a lookup function for command line arguments:

```go

var (
    path string
    max  int
)

args := conf.MakeParser(conf.CommandLineArgs())

conf.Var(args, "path", &path)
conf.Var(args, "max", &max)

args.MustParse()
```

### File System Configuration

Create a lookup function for file system-based configuration:

```go
var (
    secret string
)

fs := conf.MakeParser(conf.FileSystem(conf.FileSystemOptions{Base: "/path/to/config/files"}))
conf.Var(fs, "secret.txt", &secret)

fs.MustParse()
```

### Multi Source Lookups

You can pass more than one lookup function when creating a parser. It will look search each source in turn and attempt to use the first value it finds.

```go
// First Lookup command line args, then fallback to os.Lookupenv
sources := conf.MakeParser(conf.CommandLineArgs(), os.Lookupenv)

var max int

// Note that commandline args automatically lower-cases and converts underscores to dashes before performing a lookup. This allows it to play nicely os.Lookupenv and allow you to override environment variables via command line args.
conf.Var(sources, &max, "MAX") // Can be configured via --max flag or MAX environment variable
sources.MustParse()
```

## License

This project is licensed under the MIT License - see the LICENSE file for details
