package input

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ReadLine prompts and returns a trimmed line. "exit" or "quit" returns "exit".
func ReadLine(prompt string) string {
	fmt.Print(prompt + " ")
	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	text = strings.TrimSpace(text)
	if strings.EqualFold(text, "exit") || strings.EqualFold(text, "quit") {
		return "exit"
	}
	return text
}

func ReadIntWithDefault(prompt string, def int) int {
	fmt.Printf("%s (default: %d) ", prompt, def)
	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	text = strings.TrimSpace(text)
	if text == "" {
		return def
	}
	n, err := strconv.Atoi(text)
	if err != nil {
		return def
	}
	return n
}

func ReadHeaders() map[string]string {
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	fmt.Println("Enter custom headers (key:value) — press Enter when done:")
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("> ")
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			headers[key] = value
		} else {
			fmt.Println("Invalid format. Use key:value")
		}
	}
	return headers
}

// expandPath expands leading ~ to the user's home directory, expands environment
// variables like $HOME, and returns an absolute, cleaned path.
func expandPath(p string) (string, error) {
	if p == "" {
		return p, nil
	}

	// Expand environment variables first ($HOME, ${VAR}, etc)
	p = os.ExpandEnv(p)

	// If path begins with ~ or ~/ expand to user home dir
	if strings.HasPrefix(p, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not determine home directory: %w", err)
		}
		// if "~" or "~/..."
		if p == "~" {
			p = home
		} else if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "~"+string(os.PathSeparator)) {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"+string(os.PathSeparator)))
		} else {
			// handle ~username/... is left as-is for simplicity
			// you could implement lookup of other users if needed
			return "", fmt.Errorf("unsupported path with ~username: %s", p)
		}
	}

	// Make absolute path
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("failed to make absolute path: %w", err)
	}

	// Clean the path
	abs = filepath.Clean(abs)
	return abs, nil
}

// ReadWordlists reads each file path in the map and returns map[field]lines slice and min length.
// If a value is quoted (starts and ends with double quotes), it's treated as a literal constant
// and not interpreted as a filepath. Example:
//
//	fieldFiles["type"] = "\"login\""  -> out["type"] = []string{"login"}
func ReadWordlists(fieldFiles map[string]string) (map[string][]string, int, error) {
	out := map[string][]string{}
	min := -1
	for field, path := range fieldFiles {
		path = strings.TrimSpace(path)
		// If value is a quoted literal, treat as constant
		if strings.HasPrefix(path, "\"") && strings.HasSuffix(path, "\"") && len(path) >= 2 {
			literal := path[1 : len(path)-1]
			out[field] = []string{literal}
			if min == -1 || 1 < min {
				min = 1
			}
			continue
		}

		// Not a literal -> expand and read file
		expanded, err := expandPath(path)
		if err != nil {
			return nil, 0, fmt.Errorf("expanding %s: %w", path, err)
		}
		b, err := os.ReadFile(expanded)
		if err != nil {
			return nil, 0, fmt.Errorf("reading %s: %w", expanded, err)
		}
		lines := []string{}
		scanner := bufio.NewScanner(strings.NewReader(string(b)))
		for scanner.Scan() {
			l := strings.TrimSpace(scanner.Text())
			if l == "" {
				continue
			}
			lines = append(lines, l)
		}
		out[field] = lines
		if min == -1 || len(lines) < min {
			min = len(lines)
		}
	}
	if min == -1 {
		min = 0
	}
	return out, min, nil
}
