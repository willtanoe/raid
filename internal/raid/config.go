package raid

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type config struct {
	AdditionalCacheDirs []string
}

func loadConfig(home string) config {
	cfg := config{}
	path := filepath.Join(home, ".config", "raid", "config")
	file, err := os.Open(path)
	if err != nil {
		return cfg
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
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
		switch key {
		case "additional-cache-dir":
			cfg.AdditionalCacheDirs = append(cfg.AdditionalCacheDirs, value)
		}
	}
	return cfg
}
