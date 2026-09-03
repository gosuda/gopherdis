package commands

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gosuda/gopherdis/cluster"
)

var defaultClusterManager = cluster.NewClusterManager("node_default", "127.0.0.1:6379")

func init() {
	DefaultTable.Register(&Command{
		Name:    "cluster",
		Handler: clusterCommand,
		Arity:   -2,
		Flags:   FlagAdmin,
	})
}

func getClusterManager(ctx *Context) *cluster.ClusterManager {
	if ctx != nil && ctx.Cluster != nil {
		return ctx.Cluster
	}
	return defaultClusterManager
}

func clusterCommand(ctx *Context, argv [][]byte) []byte {
	subCmd := strings.ToUpper(string(argv[1]))
	cm := getClusterManager(ctx)

	switch subCmd {
	case "SLOTS":
		slots := cm.FormatClusterSlots()
		return Array(slots)

	case "NODES":
		nodesStr := cm.FormatClusterNodes()
		return BulkString([]byte(nodesStr))

	case "KEYSLOT":
		if len(argv) < 3 {
			return Error("wrong number of arguments for 'cluster keyslot' command")
		}
		key := string(argv[2])
		slot := cluster.KeySlot(key)
		return Integer(int64(slot))

	case "INFO":
		info := fmt.Sprintf("cluster_state:ok\r\ncluster_slots_assigned:%d\r\ncluster_current_epoch:%d\r\n",
			cluster.NumSlots, cm.CurrentEpoch())
		return BulkString([]byte(info))

	case "MEET":
		if len(argv) < 4 {
			return Error("wrong number of arguments for 'cluster meet' command")
		}
		ip := string(argv[2])
		portStr := string(argv[3])
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return Error("invalid port in cluster meet")
		}
		nodeID := fmt.Sprintf("node_%s_%d", ip, port)
		cm.AddNode(nodeID, fmt.Sprintf("%s:%d", ip, port), cluster.RoleMaster, "")
		return OK()

	case "FAILOVER":
		// Trigger failover
		return OK()

	case "SHADOW":
		if len(argv) < 3 {
			return Error("wrong number of arguments for 'cluster shadow' command")
		}
		failingNodeID := string(argv[2])
		plan, err := cm.CheckAndAutoMitigate(failingNodeID)
		if err != nil {
			return Error(err.Error())
		}
		if plan == nil {
			return BulkString([]byte("NO_MITIGATION_REQUIRED"))
		}
		return BulkString([]byte(fmt.Sprintf("SHADOW_SPAWNED target=%s risk=%.2f", plan.TargetNodeID, plan.EstimatedRiskAfter)))

	case "HANDOVER":
		if len(argv) < 3 {
			return Error("wrong number of arguments for 'cluster handover' command")
		}
		failingNodeID := string(argv[2])
		plan, err := cm.CheckAndAutoMitigate(failingNodeID)
		if err != nil {
			return Error(err.Error())
		}
		if plan != nil {
			_ = cm.LiveHandover(plan)
			return OK()
		}
		return OK()

	default:
		return Error(fmt.Sprintf("unknown subcommand '%s'", subCmd))
	}
}
