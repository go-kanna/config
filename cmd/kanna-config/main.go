// Command kanna-config generates the functions reading the settings of
// structs marked //kanna:config from flags, the environment, and files.
package main

import (
	"os"

	"github.com/go-kanna/config/internal/gen"
)

func main() {
	os.Exit(gen.Command().Main(os.Args[1:]))
}
