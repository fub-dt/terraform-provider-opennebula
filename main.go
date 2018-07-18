package main

import (
	"github.com/fub-dt/terraform-provider-opennebula/opennebula"
	"github.com/hashicorp/terraform/plugin"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: opennebula.Provider,
	})
}
