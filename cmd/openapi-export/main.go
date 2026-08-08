// Command openapi-export writes the krsec OpenAPI spec to a JSON file.
//
// The HTTP server never writes the spec to disk at runtime; this command is
// the single source of doc/openapi.json (see `make openapi`).
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/smallfish06/krsec/internal/server"
)

func main() {
	out := flag.String("out", "doc/openapi.json", "Path to write the OpenAPI JSON spec")
	flag.Parse()

	srv := server.NewWithBrokers("localhost", 8080, nil, nil)
	engine := srv.App().Engine
	engine.OpenAPI.Config.DisableLocalSave = false
	engine.OpenAPI.Config.JSONFilePath = *out
	engine.OutputOpenAPISpec()

	if _, err := os.Stat(*out); err != nil {
		fmt.Fprintf(os.Stderr, "openapi-export: spec not written to %s: %v\n", *out, err)
		os.Exit(1)
	}
	fmt.Printf("openapi-export: wrote %s\n", *out)
}
