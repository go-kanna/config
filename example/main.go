// Command example reads the settings of package settings from files, the
// environment, and flags, and prints what each source gives, or what is wrong.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/go-kanna/config/source"
	"github.com/go-kanna/config/toml"
	"github.com/go-kanna/config/yaml"

	"github.com/go-kanna/config/example/settings"
)

// variables are those the settings are read from, which each run of the
// example sets afresh.
var variables = []string{"PORT", "SERVER_TIMEOUT", "SERVER_LEVEL", "DATABASE_WRITER_URL", "DATABASE_POOL_MAX_CONNS"}

func main() {
	unset()
	show("The help of the flags", []string{"-h"})
	settingsYAML := source.File{Path: "settings.yaml", Decode: yaml.Decode}
	show("settings.yaml", nil, settingsYAML)
	show("settings.toml", nil, source.File{Path: "settings.toml", Decode: toml.Decode})

	_ = os.Setenv("DATABASE_POOL_MAX_CONNS", "30")
	show("The environment wins over the file", nil, settingsYAML)
	show("A flag wins over the environment", []string{"-database-pool-max-conns=40", "-server-timeout=1m"}, settingsYAML)

	unset()
	_ = os.Setenv("PORT", "0")
	_ = os.Setenv("SERVER_TIMEOUT", "soon")
	show("What cannot be read", nil)
	_ = os.Setenv("SERVER_TIMEOUT", "")
	show("What breaks a rule", nil)
}

func unset() {
	for _, name := range variables {
		_ = os.Unsetenv(name)
	}
}

// show reads the settings from the flags args, the environment, and files, and
// prints them.
func show(title string, args []string, files ...source.File) {
	fmt.Fprintf(os.Stdout, "=== %s ===\n", title)
	fs := flag.NewFlagSet("example", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	settings.DefineServerFlags(fs)
	settings.DefineDatabaseFlags(fs)
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stdout, "%v\n\n", err)
		return
	}
	src, err := source.Open(source.Sources{Flags: fs, Files: files})
	if err != nil {
		fmt.Fprintf(os.Stdout, "%v\n\n", err)
		return
	}

	if server, err := settings.LoadServer(src); err != nil {
		fmt.Fprintf(os.Stdout, "server: %v\n", err)
	} else {
		fmt.Fprintf(os.Stdout, "server: port %d, timeout %s, level %s\n", server.Port, server.Timeout, server.Level)
	}
	if db, err := settings.LoadDatabase(src); err != nil {
		fmt.Fprintf(os.Stdout, "database: %v\n\n", err)
	} else {
		fmt.Fprintf(os.Stdout, "database: writer %s, pool %d\n\n", db.WriterURL, db.PoolMaxConns)
	}
}
