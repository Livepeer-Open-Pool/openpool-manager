package main

import (
	"flag"
	"fmt"

	"github.com/Livepeer-Open-Pool/openpool-plugin/cmd"
)

func main() {
	configFileName := flag.String("config", "/etc/pool/config.json", "Open Pool Configuration file to use")
	flag.Parse()
	fmt.Printf("Using config file: %s\n", configFileName)

	cmd.Run(*configFileName)
}
