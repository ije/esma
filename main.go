package main

import (
	"embed"
	"esmd/server"
)

//go:embed embed
var fs embed.FS

func main() {
	server.Serve(&fs)
}
