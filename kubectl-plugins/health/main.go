package main

import (
	"os"

	"github.com/ipfs/go-log/v2"
	"github.com/vacp2p/vaclab-k8s-plugins/kubectl-plugins/health/args"
)

var logger = log.Logger("health")

func main() {
	log.SetAllLoggers(log.LevelInfo)
	log.SetLogLevel("health", "info")
	if err := args.BenchmarkParams.Run(os.Args); err != nil {
		os.Exit(1)
	}

}
