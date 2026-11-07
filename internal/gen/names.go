package gen

import (
	"regexp"
	"strings"
	"unicode"
)

// snakeWords matches the words a setting is named by, as config:"…" and prefix=
// write them: snake case, lower case, such as writer_url.
var snakeWords = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

// snakeCase writes name in snake case, splitting it into words as kanna-orm
// splits a field into the name of its column: a word starts at an upper-case
// letter after a lower-case one or a digit, and at the last of a run of
// upper-case letters that a lower-case one follows, so that WriterURL is
// writer_url and HTTPServer http_server. There is no list of acronyms to
// consult: OAuthToken is o_auth_token.
func snakeCase(name string) string {
	runes := []rune(name)
	var b strings.Builder
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if unicode.IsLower(prev) || unicode.IsDigit(prev) || (unicode.IsUpper(prev) && nextLower) {
				b.WriteByte('_')
			}
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// key is how one setting is named in each place it is looked for.
type key struct {
	env  string
	file []string
	flag string
}

// keyOf names the setting of the words of a field under the words of its
// struct: DATABASE_WRITER_URL in the environment, writer_url in the section
// database of a file, and -database-writer-url as a flag. env, when set, is the
// variable as a tag names it.
func keyOf(prefix, field, env string) key {
	all := prefix + "_" + field
	if env == "" {
		env = strings.ToUpper(all)
	}
	return key{env: env, file: []string{prefix, field}, flag: strings.ReplaceAll(all, "_", "-")}
}
