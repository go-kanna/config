// Package emit holds one of every construct kanna-config generates. Its
// config_gen.go is the golden file of the emit test, and the test calls the
// functions it declares.
package emit

import (
	"io/fs"
	"log/slog"
	"net/netip"
	"strings"
	"time"

	"github.com/go-kanna/validator"
)

// Mode is an enum as kanna-value generates one, down to what kanna-config
// knows it by: its constants and its constructor.
type Mode string

const (
	ModeFast Mode = "fast"
	ModeSlow Mode = "slow"
)

func NewMode(v string) (Mode, error) {
	switch m := Mode(v); m {
	case ModeFast, ModeSlow:
		return m, nil
	}
	return "", validator.Inclusion()
}

// Email is a value type as kanna-value generates one, holding a string.
type Email struct{ s string }

func (e Email) String() string { return e.s }

func NewEmail(s string) (Email, error) {
	if !strings.Contains(s, "@") {
		return Email{}, validator.Fail("email")
	}
	return Email{s: s}, nil
}

// Port is a value type holding a number.
type Port struct{ i int }

func (p Port) Int() int { return p.i }

func NewPort(i int) (Port, error) {
	if i < 1 || i > 65535 {
		return Port{}, validator.Fail("port")
	}
	return Port{i: i}, nil
}

// DSN is a string of its own.
type DSN string

//kanna:config
type Database struct {
	AdminURL     string
	WriterURL    DSN           `validate:"required"`
	PoolMaxConns int32         `default:"10" validate:"gte=1"`
	DialTimeout  time.Duration `default:"5s"`
	Ratio        float32       `default:"0.5"`
	Debug        bool
	Retries      uint8
	Password     string `config:"-"`
}

//kanna:config prefix=oauth
type OAuth struct {
	ClientID  string `validate:"required"`
	AuthToken string `config:"token"`
}

//kanna:config
type API struct {
	Port   int  `env:"PORT" default:"8080" validate:"gte=1,lte=65535"`
	Mode   Mode `default:"fast"`
	Admin  Email
	Public Port
	Level  slog.Level `default:"info"`
	Listen netip.Addr
	Name   string `json:"name" validate:"required"`
	Alias  string
}

func (a API) validate(errs *validator.Errors) {
	if a.Alias != "" && a.Alias == a.Name {
		errs.Add("Alias", validator.Fail("same_as_name"))
	}
}

// Paths names a field by a JSON name that another's begins with.
//
//kanna:config
type Paths struct {
	Host     string `json:"host"`
	HostName string `json:"host.name"`
}

func (p Paths) validate(errs *validator.Errors) {
	if p.HostName == "-" {
		errs.Add("host.name", validator.Fail("dash"))
	}
}

// Factor is a float of its own.
type Factor float64

// Limits has defaults that no constant holds.
//
//kanna:config
type Limits struct {
	Floor   float64 `default:"-Inf" validate:"lte=0"`
	Ceiling float32 `default:"+Inf"`
	Scale   Factor  `default:"NaN"`
}

// Upload reads integers as Go literals, and its package io/fs keeps its name
// beside the flags parameter.
//
//kanna:config
type Upload struct {
	DirMode fs.FileMode `default:"0755"`
	Offset  int16       `default:"-0x10"`
}

// Fallback has defaults read when the program runs, which it rejects.
//
//kanna:config
type Fallback struct {
	Level  slog.Level `default:"loud"`
	Admin  Email      `default:"ops"`
	Public Port       `default:"port"`
}

// Replica shares the section of Database, whose keys it takes as known.
//
//kanna:config prefix=database
type Replica struct {
	ReplicaURL string
}
