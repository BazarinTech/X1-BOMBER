package input

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

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
