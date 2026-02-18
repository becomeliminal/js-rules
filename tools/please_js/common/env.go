package common

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// LoadEnvFiles loads .env variants in Vite priority order and returns
// defines for variables matching the prefix.
// Priority: .env < .env.local < .env.[mode] < .env.[mode].local
func LoadEnvFiles(basePath, mode, prefix string) (map[string]string, error) {
	variants := []string{
		basePath,
		basePath + ".local",
		basePath + "." + mode,
		basePath + "." + mode + ".local",
	}

	result := make(map[string]string)
	for _, path := range variants {
		defs, err := parseEnvFile(path, prefix)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		for k, v := range defs {
			result[k] = v
		}
	}
	return result, nil
}

// parseEnvFile reads a single .env file, filters by prefix, returns
// map like {"import.meta.env.PLZ_API_URL": `"https://..."`}
func parseEnvFile(path, prefix string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	result := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if !strings.HasPrefix(key, prefix) {
			continue
		}

		// Strip surrounding quotes from value
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		// JSON-quote for esbuild defines
		result["import.meta.env."+key] = fmt.Sprintf(`"%s"`, value)
	}
	return result, scanner.Err()
}
