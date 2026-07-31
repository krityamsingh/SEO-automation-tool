package godotenv

import (
	"bufio"
	"os"
	"strings"
)

func Load(filenames ...string) error {
	if len(filenames) == 0 {
		filenames = []string{".env"}
	}
	for _, filename := range filenames {
		file, err := os.Open(filename)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				val := strings.Trim(strings.TrimSpace(parts[1]), "\"'")
				if os.Getenv(key) == "" {
					os.Setenv(key, val)
				}
			}
		}
		file.Close()
	}
	return nil
}
