package main

import (
	"os"

	"github.com/zxzharmlesszxz/puppet-module-tooling-launcher/v3/internal/pmtool"
)

func main() { os.Exit(pmtool.Main(os.Args[1:])) }
