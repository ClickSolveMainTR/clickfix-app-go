package config

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseQueueQueues parses queue[:positive-weight] entries. Nil selects worker defaults.
func ParseQueueQueues(value string) (map[string]int, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	queues := map[string]int{}
	for _, entry := range strings.Split(value, ",") {
		parts := strings.Split(strings.TrimSpace(entry), ":")
		name := strings.TrimSpace(parts[0])
		if name == "" || len(parts) > 2 || strings.ContainsAny(name, " \t\r\n") {
			return nil, fmt.Errorf("config: invalid QUEUE_QUEUES entry %q", entry)
		}
		weight := 1
		if len(parts) == 2 {
			var err error
			weight, err = strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil || weight <= 0 {
				return nil, fmt.Errorf("config: QUEUE_QUEUES weight must be a positive integer: %q", entry)
			}
		}
		if _, exists := queues[name]; exists {
			return nil, fmt.Errorf("config: duplicate QUEUE_QUEUES queue %q", name)
		}
		queues[name] = weight
	}
	return queues, nil
}
