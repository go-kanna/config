// Package octal tells an integer that a leading 0 made octal.
package octal

import (
	"errors"
	"strconv"
	"strings"
)

// Misread reports whether err is strconv failing, with base 0, on an integer
// only because its leading 0 made it octal, as 08080, which reads as a decimal.
func Misread(err error) bool {
	n, ok := errors.AsType[*strconv.NumError](err)
	if !ok || !errors.Is(n.Err, strconv.ErrSyntax) || (n.Func != "ParseInt" && n.Func != "ParseUint") {
		return false
	}
	digits := strings.TrimLeft(n.Num, "+-")
	if len(digits) < 2 || digits[0] != '0' || digits[1] < '0' || digits[1] > '9' {
		return false
	}
	_, err = strconv.ParseUint(digits, 10, 64)
	return err == nil || errors.Is(err, strconv.ErrRange)
}
