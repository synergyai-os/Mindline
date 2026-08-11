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
		if name == "connection" {
			// Connection handles are canonical opaque values. Preserve the
			// caller's exact bytes so validation rejects whitespace rather than
			// silently changing the identity before hashing.
			optionValue = args[index]
		}
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

func hasAgentOption(args []string, names ...string) bool {
	allowed := make(map[string]bool, len(names))
	for _, name := range names {
		allowed["--"+name] = true
	}
	for _, value := range args {
		if allowed[value] {
			return true
		}
	}
	return false
}

func hasAgentOptionValue(args []string, name, expected string) bool {
	option := "--" + name
	for index := 0; index+1 < len(args); index++ {
		if args[index] == option && strings.TrimSpace(args[index+1]) == expected {
			return true
		}
	}
	return false
}
