package commands

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

func init() {
	DefaultTable.Register(&Command{
		Name:    "sentinel",
		Handler: sentinelCommand,
		Arity:   -2,
		Flags:   FlagAdmin | FlagReadOnly,
	})
}

func sentinelCommand(ctx *Context, argv [][]byte) []byte {
	subCmd := strings.ToLower(string(argv[1]))

	// Determine current master address
	masterHost := "127.0.0.1"
	masterPort := "6379"

	if ctx != nil && ctx.Cluster != nil {
		// Use self addr or cluster manager topology info
		if isLocal, _, addr := ctx.Cluster.IsSlotLocal("0"); isLocal || addr != "" {
			if h, p, err := net.SplitHostPort(addr); err == nil {
				masterHost = h
				masterPort = p
			}
		}
	}

	switch subCmd {
	case "get-master-addr-by-name":
		if len(argv) < 3 {
			return Error("wrong number of arguments for 'sentinel get-master-addr-by-name' command")
		}
		// Returns 2-element array: [ip, port]
		return Array([][]byte{
			BulkString([]byte(masterHost)),
			BulkString([]byte(masterPort)),
		})

	case "masters":
		// Returns list of monitored masters
		masterInfo := formatSentinelMasterInfo("mymaster", masterHost, masterPort, 1)
		return Array([][]byte{masterInfo})

	case "master":
		if len(argv) < 3 {
			return Error("wrong number of arguments for 'sentinel master' command")
		}
		name := string(argv[2])
		return formatSentinelMasterInfo(name, masterHost, masterPort, 1)

	case "replicas", "slaves":
		if len(argv) < 3 {
			return Error("wrong number of arguments for 'sentinel replicas' command")
		}
		// Return replicas array
		return Array(nil)

	case "is-master-down-by-addr":
		// Format: [is_down (0 or 1), leader_runid, leader_epoch]
		epoch := int64(1)
		if ctx != nil && ctx.Cluster != nil {
			epoch = int64(ctx.Cluster.CurrentEpoch())
		}
		return Array([][]byte{
			Integer(0), // Not down
			BulkString([]byte("*")),
			Integer(epoch),
		})

	case "failover":
		if len(argv) < 3 {
			return Error("wrong number of arguments for 'sentinel failover' command")
		}
		masterName := string(argv[2])

		// Proactively trigger handover and broadcast +switch-master via PubSub
		if ctx != nil && ctx.PubSub != nil {
			msg := fmt.Sprintf("%s %s %s %s %s", masterName, masterHost, masterPort, masterHost, masterPort)
			ctx.PubSub.Publish("+switch-master", []byte(msg))
		}
		return OK()

	case "ckquorum":
		return BulkString([]byte("OK 1 usable Sentinels. Quorum and failover authorization can be reached"))

	case "reset":
		return Integer(1)

	default:
		return Error(fmt.Sprintf("unknown subcommand '%s'", subCmd))
	}
}

func formatSentinelMasterInfo(name, host, port string, numSlaves int) []byte {
	portNum, _ := strconv.Atoi(port)
	pairs := [][]string{
		{"name", name},
		{"ip", host},
		{"port", strconv.Itoa(portNum)},
		{"runid", "0000000000000000000000000000000000000001"},
		{"flags", "master"},
		{"link-pending-commands", "0"},
		{"link-refcount", "1"},
		{"last-ping-sent", "0"},
		{"last-ok-ping-reply", "500"},
		{"last-ping-reply", "500"},
		{"down-after-milliseconds", "5000"},
		{"info-refresh", "1000"},
		{"role-reported", "master"},
		{"role-reported-time", "1000"},
		{"config-epoch", "1"},
		{"num-slaves", strconv.Itoa(numSlaves)},
		{"num-other-sentinels", "0"},
		{"quorum", "1"},
		{"failover-timeout", "60000"},
		{"parallel-syncs", "1"},
	}

	result := make([][]byte, 0, len(pairs)*2)
	for _, p := range pairs {
		result = append(result, BulkString([]byte(p[0])), BulkString([]byte(p[1])))
	}
	return Array(result)
}
