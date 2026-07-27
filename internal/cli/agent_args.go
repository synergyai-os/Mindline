package cli

import (
	"errors"
	"strings"
)

type agentOptions struct {
	configPath  string
	positionals []string
	values      map[string]string
}

func parseAgentOptions(args []string) (agentOptions, error) {
	options := agentOptions{values: map[string]string{}}
	for index := 0; index < len(args); index++ {
		value := args[index]
		if !strings.HasPrefix(value, "--") {
			options.positionals = append(options.positionals, value)
			continue
		}
		name := strings.TrimPrefix(value, "--")
		index++
		if index >= len(args) || strings.TrimSpace(args[index]) == "" {
			return agentOptions{}, errors.New("missing option value")
		}
		optionValue := strings.TrimSpace(args[index])
		if name == "config" {
			if options.configPath != "" {
				return agentOptions{}, errors.New("duplicate config")
			}
			options.configPath = optionValue
			continue
		}
		if _, duplicate := options.values[name]; duplicate {
			return agentOptions{}, errors.New("duplicate option")
		}
		options.values[name] = optionValue
	}
	return options, nil
}

func onlyAgentKeys(values map[string]string, allowed ...string) bool {
	set := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		set[key] = true
	}
	for key := range values {
		if !set[key] {
			return false
		}
	}
	return true
}
