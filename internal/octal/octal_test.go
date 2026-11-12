package octal_test

import (
	"errors"
	"strconv"
	"testing"

	"github.com/go-kanna/config/internal/octal"
)

func TestMisread(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		err  error
		want bool
	}{
		{err: parseInt("08080"), want: true},
		{err: parseInt("-09"), want: true},
		{err: parseInt("+0099999999999999999999"), want: true},
		{err: parseUint("018"), want: true},
		{err: parseInt("0755x")},
		{err: parseInt("0_8")},
		{err: parseInt("0x")},
		{err: parseInt("8x")},
		{err: parseInt("0o8")},
		{err: parseUint("-1")},
		{err: parseInt("0777777777777777777777777")},
		{err: errors.New("08080")},
		{err: &strconv.NumError{Func: "ParseFloat", Num: "08", Err: strconv.ErrSyntax}},
	} {
		if got := octal.Misread(tt.err); got != tt.want {
			t.Errorf("Misread(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}

func parseInt(text string) error {
	_, err := strconv.ParseInt(text, 0, 64)
	return err //nolint:wrapcheck // Misread reads it as strconv returns it
}

func parseUint(text string) error {
	_, err := strconv.ParseUint(text, 0, 64)
	return err //nolint:wrapcheck // Misread reads it as strconv returns it
}
