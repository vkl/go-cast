package logger

import (
	"io"
	"log"
)

var CastDebug = false

func init() {
	if !CastDebug {
		log.SetOutput(io.Discard)
	}
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
}
