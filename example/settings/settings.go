// Package settings declares the settings the example reads: from flags, the
// environment, and files, and checked by their validate tags.
package settings

import (
	"log/slog"
	"time"
)

//go:generate go tool kanna-config

// Server is read from -server-timeout, SERVER_TIMEOUT, server.timeout, and so
// on, but for its port, which a platform such as Cloud Run gives as PORT.
//
//kanna:config
type Server struct {
	// Port is the port the server listens on. The help of its flag, -h,
	// shows this first sentence.
	Port int `default:"8080" env:"PORT" validate:"gte=1,lte=65535"`
	// Timeout bounds the time a request takes.
	Timeout time.Duration `default:"5s"`
	Level   slog.Level    `default:"info"`
}

// Database leaves out its password, which the program reads from a secret
// manager.
//
//kanna:config
type Database struct {
	WriterURL    string `validate:"required"`
	PoolMaxConns int32  `default:"10"        validate:"gte=1"`
	Password     string `config:"-"`
}
