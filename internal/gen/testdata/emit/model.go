// Package emit holds one of every construct kanna-config generates. Its
// config_gen.go is the golden file of the emit test, and the test calls the
// functions it declares.
package emit

import (
	"log/slog"
	"net/netip"
	"strings"
	"time"

	"github.com/go-kanna/validate"
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
	return "", validate.Inclusion()
}

// Email is a value type as kanna-value generates one, holding a string.
type Email struct{ s string }

func (e Email) String() string { return e.s }

func NewEmail(s string) (Email, error) {
	if !strings.Contains(s, "@") {
		return Email{}, validate.Fail("email")
	}
	return Email{s: s}, nil
}

// Port is a value type holding a number.
type Port struct{ i int }

func (p Port) Int() int { return p.i }

func NewPort(i int) (Port, error) {
	if i < 1 || i > 65535 {
		return Port{}, validate.Fail("port")
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

func (a API) validate(errs *validate.Errors) {
	if a.Alias != "" && a.Alias == a.Name {
		errs.Add("Alias", validate.Fail("same_as_name"))
	}
}
