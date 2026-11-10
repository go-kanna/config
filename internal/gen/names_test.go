package gen_test

import (
	"testing"

	"github.com/go-kanna/config/internal/gen"
)

func TestSnakeHint(t *testing.T) {
	t.Parallel()

	const plain = "write it in lower case, its words joined by _"
	tests := map[string]string{
		"AuthToken": plain + `, as config:"auth_token"`,
		"Bad Name":  plain + `, as config:"bad_name"`,
		"a.b":       plain + `, as config:"a_b"`,
		"Bad-Name":  plain + `, as config:"bad_name"`,
		"a__b":      plain + `, as config:"a_b"`,
		"1st":       plain,
		"café":      plain,
	}
	for text, want := range tests {
		if got := gen.SnakeHint(`config:"%s"`, text); got != want {
			t.Errorf("snakeHint(%q) = %q, want %q", text, got, want)
		}
	}
}
