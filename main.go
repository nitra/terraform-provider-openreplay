package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/nitra/terraform-provider-openreplay/internal/provider"
)

var version = "dev"

// main serves the OpenReplay provider using the OpenTofu protocol v6 interface.
func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "Run with debugger support")
	flag.Parse()
	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address:         "registry.opentofu.org/nitra/openreplay",
		Debug:           debug,
		ProtocolVersion: 6,
	})
	if err != nil {
		log.Fatal(err)
	}
}
