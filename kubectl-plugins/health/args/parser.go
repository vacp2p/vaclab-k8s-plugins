package args

import (
	"context"
	"time"

	"github.com/urfave/cli/v2"
	"github.com/vacp2p/vaclab-k8s-plugins/kubectl-plugins/health/k3sutils"
)

var CurrentConfig k3sutils.BenchmarkConfig
var BenchmarkParams *cli.App = &cli.App{
	EnableBashCompletion: true,
	HideHelp:             false,
	Name:                 "health",
	Usage:                "A kubectl plugin for iperf-based health monitoring in kubernetes clusters.",
	Commands: []*cli.Command{
		{
			Name:     "create",
			Aliases:  []string{"r", "start", "run"},
			Usage:    "launch new iperf benchmark",
			HideHelp: false,

			Flags: []cli.Flag{
				&cli.IntFlag{Name: "conn", Value: 4, Usage: "Number of parallel iperf connections.", Aliases: []string{"parallel", "c"}},
				&cli.IntFlag{Name: "duration", Value: 10, Usage: "Duration of the iperf test in seconds.", Aliases: []string{"d", "seconds"}},
			},
			Action: func(c *cli.Context) error {

				CurrentConfig.NumberOfConnections = c.Int("conn")
				CurrentConfig.DurationSeconds = c.Int("duration")
				return k3sutils.CreateNewBenchmark(CurrentConfig)
			},
		},
		{
			Name:     "get",
			Aliases:  []string{"l", "list", "show"},
			Usage:    "list benchmark results by destination node",
			HideHelp: false,

			Flags: []cli.Flag{
				&cli.BoolFlag{Name: "latest", Usage: "Show only the latest result per destination node.", Aliases: []string{"L"}},
				&cli.BoolFlag{Name: "today", Usage: "Show only today's benchmark results.", Aliases: []string{"t"}},
				&cli.StringFlag{Name: "mode", Usage: "Filter by network mode: 'host' or 'pod'.", Aliases: []string{"m"}},
				&cli.StringFlag{Name: "node", Usage: "Filter by destination node name (partial match).", Aliases: []string{"n"}},
				&cli.IntFlag{Name: "limit", Usage: "Maximum number of results to show.", Value: 0, Aliases: []string{"max"}},
			},
			Action: func(c *cli.Context) error {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()

				// Build filter from flags
				filter := k3sutils.ListFilter{
					Latest: c.Bool("latest"),
					Today:  c.Bool("today"),
					Mode:   c.String("mode"),
					Node:   c.String("node"),
					Limit:  c.Int("limit"),
				}

				return k3sutils.ListBenchmarksWithFilter(ctx, filter)
			},
		},
	},
}
