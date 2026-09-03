package commands

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/gosuda/gopherdis/pubsub"
)

func init() {
	DefaultTable.Register(&Command{
		Name:    "publish",
		Handler: publishCommand,
		Arity:   3,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "subscribe",
		Handler: subscribeCommand,
		Arity:   -2,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "unsubscribe",
		Handler: unsubscribeCommand,
		Arity:   -1,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "psubscribe",
		Handler: psubscribeCommand,
		Arity:   -2,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "punsubscribe",
		Handler: punsubscribeCommand,
		Arity:   -1,
		Flags:   FlagFast | FlagReadOnly,
	})
	DefaultTable.Register(&Command{
		Name:    "pubsub",
		Handler: pubsubSubCommands,
		Arity:   -2,
		Flags:   FlagReadOnly,
	})
}

func publishCommand(ctx *Context, argv [][]byte) []byte {
	if ctx.PubSub == nil {
		return Integer(0)
	}
	channel := string(argv[1])
	receivers := ctx.PubSub.Publish(channel, argv[2])
	return Integer(int64(receivers))
}

func subscribeCommand(ctx *Context, argv [][]byte) []byte {
	if ctx.PubSub == nil || ctx.Sub == nil {
		return Error("pubsub is disabled")
	}

	var buf bytes.Buffer
	for i := 1; i < len(argv); i++ {
		channel := string(argv[i])
		count := ctx.PubSub.Subscribe(ctx.Sub, channel)
		buf.Write(pubsub.FormatSubscribeReply("subscribe", channel, count))
	}
	return buf.Bytes()
}

func unsubscribeCommand(ctx *Context, argv [][]byte) []byte {
	if ctx.PubSub == nil || ctx.Sub == nil {
		return Error("pubsub is disabled")
	}

	var buf bytes.Buffer
	if len(argv) == 1 {
		// Unsubscribe from all exact channels
		count := ctx.Sub.SubCount()
		ctx.PubSub.UnsubscribeAll(ctx.Sub)
		buf.Write(pubsub.FormatSubscribeReply("unsubscribe", "", count))
	} else {
		for i := 1; i < len(argv); i++ {
			channel := string(argv[i])
			count := ctx.PubSub.Unsubscribe(ctx.Sub, channel)
			buf.Write(pubsub.FormatSubscribeReply("unsubscribe", channel, count))
		}
	}
	return buf.Bytes()
}

func psubscribeCommand(ctx *Context, argv [][]byte) []byte {
	if ctx.PubSub == nil || ctx.Sub == nil {
		return Error("pubsub is disabled")
	}

	var buf bytes.Buffer
	for i := 1; i < len(argv); i++ {
		pattern := string(argv[i])
		count := ctx.PubSub.PSubscribe(ctx.Sub, pattern)
		buf.Write(pubsub.FormatSubscribeReply("psubscribe", pattern, count))
	}
	return buf.Bytes()
}

func punsubscribeCommand(ctx *Context, argv [][]byte) []byte {
	if ctx.PubSub == nil || ctx.Sub == nil {
		return Error("pubsub is disabled")
	}

	var buf bytes.Buffer
	if len(argv) == 1 {
		count := ctx.Sub.SubCount()
		ctx.PubSub.UnsubscribeAll(ctx.Sub)
		buf.Write(pubsub.FormatSubscribeReply("punsubscribe", "", count))
	} else {
		for i := 1; i < len(argv); i++ {
			pattern := string(argv[i])
			count := ctx.PubSub.PUnsubscribe(ctx.Sub, pattern)
			buf.Write(pubsub.FormatSubscribeReply("punsubscribe", pattern, count))
		}
	}
	return buf.Bytes()
}

func pubsubSubCommands(ctx *Context, argv [][]byte) []byte {
	if ctx.PubSub == nil {
		return Error("pubsub is disabled")
	}

	subCmd := strings.ToLower(string(argv[1]))
	switch subCmd {
	case "channels":
		pat := ""
		if len(argv) >= 3 {
			pat = string(argv[2])
		}
		channels := ctx.PubSub.PubSubChannels(pat)
		replies := make([][]byte, len(channels))
		for i, ch := range channels {
			replies[i] = BulkString([]byte(ch))
		}
		return Array(replies)

	case "numsub":
		channels := make([]string, 0, len(argv)-2)
		for i := 2; i < len(argv); i++ {
			channels = append(channels, string(argv[i]))
		}
		counts := ctx.PubSub.PubSubNumSub(channels)
		replies := make([][]byte, 0, len(channels)*2)
		for _, ch := range channels {
			replies = append(replies, BulkString([]byte(ch)))
			replies = append(replies, Integer(int64(counts[ch])))
		}
		return Array(replies)

	case "numpat":
		return Integer(int64(ctx.PubSub.PubSubNumPat()))

	default:
		return Error(fmt.Sprintf("unknown pubsub subcommand '%s'", subCmd))
	}
}
