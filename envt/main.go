package envt

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Config holds the configuration for locating and parsing an env file.
type Config struct {
	// EnvPath is the path to the .env file.
	// - If it is a relative path, it will be resolved relative to the project root
	//   (the directory containing go.mod or go.sum).
	// - If it is an absolute path, it will be used as-is.
	// - If left empty, it defaults to ".env" at the project root.
	EnvPath string

	// PanicErr, if true, will cause the function to panic instead of returning
	// an error when the env file cannot be found or parsed.
	PanicErr bool
}

// Map locates and parses the specified env file, returning its contents as a map.
// It automatically resolves relative paths against the project root (where go.mod/go.sum is located),
// making it reliable regardless of the current working directory (e.g., when running tests from an IDE).
func Map(cfg *Config) (map[string]string, error) {
	if cfg == nil {
		cfg = &Config{}
	}

	envPath := cfg.EnvPath
	if envPath == "" {
		envPath = ".env"
	}

	// If the path is not absolute, resolve it relative to the project root
	if !filepath.IsAbs(envPath) {
		root, err := findProjectRoot()
		if err != nil {
			if cfg.PanicErr {
				panic("envt: " + err.Error())
			}
			return nil, err
		}
		envPath = filepath.Join(root, envPath)
	}

	// Verify the file exists and is not a directory
	info, err := os.Stat(envPath)
	if err != nil {
		errMsg := "envt: env file not found: " + envPath
		if cfg.PanicErr {
			panic(errMsg)
		}
		return nil, errors.New(errMsg)
	}

	if info.IsDir() {
		errMsg := "envt: path is a directory, not a file: " + envPath
		if cfg.PanicErr {
			panic(errMsg)
		}
		return nil, errors.New(errMsg)
	}

	// Parse the env file using our custom implementation
	envMap, err := parseEnvFile(envPath)
	if err != nil {
		errMsg := "envt: failed to parse env file " + envPath + ": " + err.Error()
		if cfg.PanicErr {
			panic(errMsg)
		}
		return nil, errors.New(errMsg)
	}

	return envMap, nil
}

// findProjectRoot walks up the directory tree from the caller's file location
// to find the root of the Go module (indicated by the presence of a go.mod or go.sum file).
func findProjectRoot() (string, error) {
	_, filename, _, ok := runtime.Caller(1)
	if !ok {
		return "", errors.New("envt: cannot get caller info to determine project root")
	}

	dir := filepath.Dir(filename)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		if _, err := os.Stat(filepath.Join(dir, "go.sum")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("envt: go.mod/go.sum not found; are you inside a Go module?")
		}
		dir = parent
	}
}

// WriteFile writes data to the specified file.
// If name is a relative path, it is resolved relative to the project root
// (the directory containing go.mod or go.sum), making it an absolute path.
// If name is an absolute path, it is used as-is.
// It also ensures that the parent directory exists before writing.
func WriteFile(name string, data []byte, perm os.FileMode) error {
	if !filepath.IsAbs(name) {
		root, err := findProjectRoot()
		if err != nil {
			return err
		}
		name = filepath.Join(root, name)
	}

	// Ensure the parent directory exists
	dir := filepath.Dir(name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(name, data, perm)
}

// parseEnvFile reads and parses a .env file into a map of key-value pairs.
func parseEnvFile(filePath string) (map[string]string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	envMap := make(map[string]string)
	lines := strings.Split(string(content), "\n")

	for _, line := range lines {
		// Trim leading and trailing whitespace
		line = strings.TrimSpace(line)

		// Ignore empty lines and full-line comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Ignore 'export ' prefix if present (common in bash-exported env files)
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(line[7:])
		}

		// Find the first '=' to split key and value
		eqIndex := strings.Index(line, "=")
		if eqIndex == -1 {
			// Lines without '=' are ignored (standard .env behavior)
			continue
		}

		key := strings.TrimSpace(line[:eqIndex])
		value := strings.TrimSpace(line[eqIndex+1:])

		// Process quoted values
		if len(value) >= 2 {
			firstChar := value[0]
			lastChar := value[len(value)-1]
			if (firstChar == '"' && lastChar == '"') || (firstChar == '\'' && lastChar == '\'') {
				// Remove the surrounding quotes
				value = value[1 : len(value)-1]
				// Only unescape double-quoted strings (single quotes are literal)
				if firstChar == '"' {
					value = unescapeDoubleQuotedString(value)
				}
			}
		}

		envMap[key] = value
	}

	return envMap, nil
}

// unescapeDoubleQuotedString processes escape sequences in double-quoted .env values.
func unescapeDoubleQuotedString(s string) string {
	var result strings.Builder
	result.Grow(len(s))

	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				result.WriteByte('\n')
				i++
			case 'r':
				result.WriteByte('\r')
				i++
			case 't':
				result.WriteByte('\t')
				i++
			case '\\':
				result.WriteByte('\\')
				i++
			case '"':
				result.WriteByte('"')
				i++
			default:
				// If it's an unrecognized escape, keep the backslash and the character
				result.WriteByte(s[i])
			}
		} else {
			result.WriteByte(s[i])
		}
	}

	return result.String()
}
