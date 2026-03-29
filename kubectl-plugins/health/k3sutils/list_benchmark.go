package k3sutils

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func getAllDestNodeResults(ctx context.Context) ([]DestNodeTestResult, error) {
	configMaps, err := K3sClient.CoreV1().ConfigMaps(DEFAULT_NAMESPACE).List(ctx, metav1.ListOptions{
		LabelSelector: "app=iperf-benchmark",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list benchmark results: %v", err)
	}

	var results []DestNodeTestResult

	for _, cm := range configMaps.Items {
		if !strings.HasPrefix(cm.Name, CONFIG_MAP_PREFIX) {
			continue
		}

		reportJSON, ok := cm.Data["report.json"]
		if !ok {
			continue
		}

		var report BenchmarkReport
		if err := json.Unmarshal([]byte(reportJSON), &report); err != nil {
			continue
		}

		timestamp, err := time.Parse(time.RFC3339, report.Timestamp)
		if err != nil {
			timestamp = cm.CreationTimestamp.Time
		}

		// Extract each test result
		for _, r := range report.Results {
			results = append(results, DestNodeTestResult{
				Timestamp:     timestamp,
				SourceNode:    report.SourceNode,
				DestNode:      r.DestNode,
				BenchmarkMode: report.BenchmarkMode,
				Protocol:      r.Protocol,
				Direction:     r.Direction,
				ThroughputBps: r.ThroughputBps,
				RTTAvg:        r.RTTAvg,
				JitterMs:      r.JitterMs,
				PacketLossPct: r.PacketLossPct,
				Retransmits:   r.Retransmits,
			})
		}
	}

	return results, nil
}

// ListAllBenchmarks lists all benchmark results
func ListAllBenchmarks(ctx context.Context) error {
	return ListBenchmarksWithFilter(ctx, ListFilter{})
}

// ListBenchmarksWithFilter lists benchmark results with full details
func ListBenchmarksWithFilter(ctx context.Context, filter ListFilter) error {
	tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)

	results, err := getAllDestNodeResults(ctx)
	if err != nil {
		return err
	}

	if len(results) == 0 {
		fmt.Fprintln(tw, "No benchmark results found.")
		return tw.Flush()
	}

	// Apply filters
	var filtered []DestNodeTestResult
	for _, r := range results {
		// Today filter
		if filter.Today {
			now := time.Now()
			startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			if r.Timestamp.Before(startOfDay) {
				continue
			}
		}

		// Mode filter
		if filter.Mode != "" {
			if filter.Mode == "host" && r.BenchmarkMode != "host_network" {
				continue
			}
			if filter.Mode == "pod" && r.BenchmarkMode != "pod_network" {
				continue
			}
			if filter.Mode == "intra" && r.BenchmarkMode != "intra_node" {
				continue
			}
		}

		// Node filter (destination node)
		if filter.Node != "" && !strings.Contains(strings.ToLower(r.DestNode), strings.ToLower(filter.Node)) {
			continue
		}

		filtered = append(filtered, r)
	}

	if len(filtered) == 0 {
		fmt.Fprintln(tw, "No benchmark results match the filter criteria.")
		return tw.Flush()
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp.After(filtered[j].Timestamp)
	})

	// If latest flag, keep only the single most recent result per dest node + mode + protocol + direction
	if filter.Latest {
		seen := make(map[string]bool)
		var latestOnly []DestNodeTestResult
		for _, r := range filtered {
			// Key: dest node + mode + protocol + direction
			key := r.DestNode + "|" + r.BenchmarkMode + "|" + r.Protocol + "|" + r.Direction
			if !seen[key] {
				seen[key] = true
				latestOnly = append(latestOnly, r)
			}
		}
		filtered = latestOnly
	}

	// Sort by dest node, then mode, then protocol, then direction for clean grouped output
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].DestNode != filtered[j].DestNode {
			return filtered[i].DestNode < filtered[j].DestNode
		}
		if filtered[i].BenchmarkMode != filtered[j].BenchmarkMode {
			return filtered[i].BenchmarkMode < filtered[j].BenchmarkMode // host before pod
		}
		if filtered[i].Protocol != filtered[j].Protocol {
			return filtered[i].Protocol < filtered[j].Protocol // tcp before udp
		}
		return filtered[i].Direction < filtered[j].Direction // DL before UL
	})

	// Apply limit
	if filter.Limit > 0 && len(filtered) > filter.Limit {
		filtered = filtered[:filter.Limit]
	}

	// Print header
	fmt.Fprintln(tw, "DEST\tMODE\tPROTOCOL\tDIR\tTHROUGHPUT\tRTT\tJITTER\tLOSS\tRETRANS\tAGE")

	for _, r := range filtered {
		age := humanDuration(time.Since(r.Timestamp))
		throughputStr := formatThroughput(r.ThroughputBps)

		modeStr := "host"
		if r.BenchmarkMode == "pod_network" {
			modeStr = "pod"
		} else if r.BenchmarkMode == "intra_node" {
			modeStr = "intra"
		}

		rttStr := "-"
		jitterStr := "-"
		pktLossStr := "-"
		retransStr := "-"

		if r.Protocol == "tcp" {
			if r.RTTAvg > 0 {
				rttStr = fmt.Sprintf("%.2fms", r.RTTAvg)
			}
			retransStr = fmt.Sprintf("%d", r.Retransmits)
		} else {
			if r.JitterMs > 0 {
				jitterStr = fmt.Sprintf("%.2fms", r.JitterMs)
			}
			pktLossStr = fmt.Sprintf("%.2f%%", r.PacketLossPct)
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.DestNode,
			modeStr,
			strings.ToUpper(r.Protocol),
			r.Direction,
			throughputStr,
			rttStr,
			jitterStr,
			pktLossStr,
			retransStr,
			age,
		)
	}

	return tw.Flush()
}

// DescribeNodeBenchmarks shows detailed benchmark results for a specific destination node
func DescribeNodeBenchmarks(ctx context.Context, nodeName string, mode string, latest bool) error {
	tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)

	results, err := getAllDestNodeResults(ctx)
	if err != nil {
		return err
	}

	// Filter by destination node name
	var nodeResults []DestNodeTestResult
	for _, r := range results {
		if !strings.Contains(strings.ToLower(r.DestNode), strings.ToLower(nodeName)) {
			continue
		}
		if mode != "" {
			if mode == "host" && r.BenchmarkMode != "host_network" {
				continue
			}
			if mode == "pod" && r.BenchmarkMode != "pod_network" {
				continue
			}
		}
		nodeResults = append(nodeResults, r)
	}

	if len(nodeResults) == 0 {
		fmt.Fprintf(tw, "No benchmark results found for destination node '%s'\n", nodeName)
		return tw.Flush()
	}

	// Sort by timestamp descending
	sort.Slice(nodeResults, func(i, j int) bool {
		return nodeResults[i].Timestamp.After(nodeResults[j].Timestamp)
	})

	// Get the actual dest node name from first result
	actualDestNode := nodeResults[0].DestNode

	// If latest, filter to only the most recent timestamp
	if latest && len(nodeResults) > 0 {
		latestTime := nodeResults[0].Timestamp
		var latestResults []DestNodeTestResult
		for _, r := range nodeResults {
			// Allow results within same minute as "latest"
			if r.Timestamp.Sub(latestTime).Abs() < time.Minute {
				latestResults = append(latestResults, r)
			}
		}
		nodeResults = latestResults
	}

	// Calculate summary stats
	var totalThroughput, totalRTT, totalJitter, totalPacketLoss float64
	var totalRetrans int64
	var tcpCount, udpCount int

	for _, r := range nodeResults {
		totalThroughput += r.ThroughputBps
		totalRetrans += r.Retransmits
		if r.Protocol == "tcp" && r.RTTAvg > 0 {
			totalRTT += r.RTTAvg
			tcpCount++
		}
		if r.Protocol == "udp" {
			if r.JitterMs > 0 {
				totalJitter += r.JitterMs
			}
			totalPacketLoss += r.PacketLossPct
			udpCount++
		}
	}

	avgThroughput := totalThroughput / float64(len(nodeResults))
	avgRTT := float64(0)
	if tcpCount > 0 {
		avgRTT = totalRTT / float64(tcpCount)
	}
	avgJitter := float64(0)
	avgPacketLoss := float64(0)
	if udpCount > 0 {
		avgJitter = totalJitter / float64(udpCount)
		avgPacketLoss = totalPacketLoss / float64(udpCount)
	}

	// Print header
	fmt.Fprintf(tw, "\nBenchmark Results for Destination Node: %s\n", actualDestNode)
	fmt.Fprintf(tw, "==========================================\n")
	fmt.Fprintf(tw, "Total Tests:\t%d\n", len(nodeResults))
	fmt.Fprintf(tw, "Avg Throughput:\t%s\n", formatThroughput(avgThroughput))
	if avgRTT > 0 {
		fmt.Fprintf(tw, "Avg RTT:\t%.2fms\n", avgRTT)
	}
	if avgJitter > 0 {
		fmt.Fprintf(tw, "Avg Jitter:\t%.2fms\n", avgJitter)
	}
	if avgPacketLoss > 0 {
		fmt.Fprintf(tw, "Avg Packet Loss:\t%.2f%%\n", avgPacketLoss)
	}
	if totalRetrans > 0 {
		fmt.Fprintf(tw, "Total Retransmits:\t%d\n", totalRetrans)
	}
	tw.Flush()

	// Group by source node for detailed view
	sourceResults := make(map[string][]DestNodeTestResult)
	for _, r := range nodeResults {
		sourceResults[r.SourceNode] = append(sourceResults[r.SourceNode], r)
	}

	// Get sorted source nodes
	var sourceNodes []string
	for sn := range sourceResults {
		sourceNodes = append(sourceNodes, sn)
	}
	sort.Strings(sourceNodes)

	fmt.Fprintln(tw, "\nDetailed Results by Source Node:")
	fmt.Fprintln(tw, "SOURCE NODE\tMODE\tPROTOCOL\tDIRECTION\tTHROUGHPUT\tRTT\tJITTER\tPKT LOSS\tRETRANS\tTIME")

	for _, sourceNode := range sourceNodes {
		results := sourceResults[sourceNode]
		// Sort by timestamp desc within source
		sort.Slice(results, func(i, j int) bool {
			return results[i].Timestamp.After(results[j].Timestamp)
		})

		for _, r := range results {
			throughputStr := formatThroughput(r.ThroughputBps)
			rttStr := "-"
			jitterStr := "-"
			pktLossStr := "-"
			retransStr := "-"

			modeShort := "host"
			if r.BenchmarkMode == "pod_network" {
				modeShort = "pod"
			}

			if r.Protocol == "tcp" {
				if r.RTTAvg > 0 {
					rttStr = fmt.Sprintf("%.2fms", r.RTTAvg)
				}
				retransStr = fmt.Sprintf("%d", r.Retransmits)
			} else {
				if r.JitterMs > 0 {
					jitterStr = fmt.Sprintf("%.2fms", r.JitterMs)
				}
				pktLossStr = fmt.Sprintf("%.2f%%", r.PacketLossPct)
			}

			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				sourceNode,
				modeShort,
				strings.ToUpper(r.Protocol),
				r.Direction,
				throughputStr,
				rttStr,
				jitterStr,
				pktLossStr,
				retransStr,
				r.Timestamp.Format("15:04:05"),
			)
		}
	}

	return tw.Flush()
}

// ListNodes lists all destination nodes that have benchmark results
func ListNodes(ctx context.Context) error {
	tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)

	results, err := getAllDestNodeResults(ctx)
	if err != nil {
		return err
	}

	if len(results) == 0 {
		fmt.Fprintln(tw, "No benchmark results found.")
		return tw.Flush()
	}

	// Aggregate by destination node
	nodeData := make(map[string]*DestNodeSummary)
	for _, r := range results {
		if _, ok := nodeData[r.DestNode]; !ok {
			nodeData[r.DestNode] = &DestNodeSummary{
				DestNode: r.DestNode,
			}
		}
		nd := nodeData[r.DestNode]
		nd.TotalTests++
		if r.BenchmarkMode == "host_network" {
			nd.HostModeTests++
		} else {
			nd.PodModeTests++
		}
		if r.Timestamp.After(nd.LatestRun) {
			nd.LatestRun = r.Timestamp
		}
		// Running averages
		nd.AvgThroughput = (nd.AvgThroughput*float64(nd.TotalTests-1) + r.ThroughputBps) / float64(nd.TotalTests)
		if r.Protocol == "tcp" && r.RTTAvg > 0 {
			if nd.AvgRTT == 0 {
				nd.AvgRTT = r.RTTAvg
			} else {
				nd.AvgRTT = (nd.AvgRTT + r.RTTAvg) / 2
			}
		}
		nd.TotalRetrans += r.Retransmits
	}

	// Convert to slice and sort
	var nodes []DestNodeSummary
	for _, nd := range nodeData {
		nodes = append(nodes, *nd)
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].DestNode < nodes[j].DestNode
	})

	// Print
	fmt.Fprintln(tw, "DEST NODE\tTOTAL TESTS\tHOST\tPOD\tAVG THROUGHPUT\tAVG RTT\tLAST TEST\tAGE")

	for _, n := range nodes {
		age := humanDuration(time.Since(n.LatestRun))
		throughputStr := formatThroughput(n.AvgThroughput)
		rttStr := "-"
		if n.AvgRTT > 0 {
			rttStr = fmt.Sprintf("%.2fms", n.AvgRTT)
		}

		fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%s\t%s\t%s\t%s\n",
			n.DestNode,
			n.TotalTests,
			n.HostModeTests,
			n.PodModeTests,
			throughputStr,
			rttStr,
			n.LatestRun.Format("2006-01-02 15:04"),
			age,
		)
	}

	return tw.Flush()
}

// formatThroughput formats throughput in human-readable format
func formatThroughput(bps float64) string {
	if bps >= 1e9 {
		return fmt.Sprintf("%.2f Gbps", bps/1e9)
	} else if bps >= 1e6 {
		return fmt.Sprintf("%.2f Mbps", bps/1e6)
	} else if bps >= 1e3 {
		return fmt.Sprintf("%.2f Kbps", bps/1e3)
	}
	return fmt.Sprintf("%.2f bps", bps)
}

// humanDuration formats duration in human-readable format
func humanDuration(d time.Duration) string {
	sec := int64(d.Seconds())
	switch {
	case sec < 60:
		return fmt.Sprintf("%ds", sec)
	case sec < 3600:
		return fmt.Sprintf("%dm%ds", sec/60, sec%60)
	case sec < 86400:
		return fmt.Sprintf("%dh%dm", sec/3600, (sec%3600)/60)
	default:
		days := sec / 86400
		hours := (sec % 86400) / 3600
		return fmt.Sprintf("%dd%dh", days, hours)
	}
}
