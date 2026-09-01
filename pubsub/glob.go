package pubsub

import "path"

// matchPattern checks if a channel matches a Redis glob pattern.
func matchPattern(pattern, channel string) bool {
	matched, err := path.Match(pattern, channel)
	if err != nil {
		return false
	}
	return matched
}
