package menu

import (
	"fmt"
	"path/filepath"
	"strings"

	"x1/internal/httpclient"
	"x1/internal/utils/input"
	"x1/internal/utils/printer"
)

func ShowMainMenu() bool {
	printer.PrintMenuTitle("=== X1 BOMBER CLI ===")
	printer.PrintMenuOption("1", "Send Registration Request (file-based payloads)")
	printer.PrintMenuOption("2", "Send Authentication Request (file-based payloads)")
	printer.PrintMenuOption("3", "Install Tor & proxychains (requires sudo, apt-based systems)")
	printer.PrintMenuOption("4", "Exit")

	choice := input.ReadLine("Select an option: ")

	switch choice {
	case "1":
		handleBulkRequest("Registration")
	case "2":
		handleBulkRequest("Authentication")
	case "3":
		installTorProxychains()
	case "4", "exit", "quit":
		return true
	default:
		printer.PrintError("Invalid option, try again.")
	}
	return false
}

func handleBulkRequest(kind string) {
	printer.PrintInfo("Preparing to send " + kind + " requests...")

	url := input.ReadLine("Enter target URL:")
	if url == "" || url == "exit" {
		printer.PrintInfo("Cancelled.")
		return
	}

	payloadTpl := input.ReadLine("Enter payload template (e.g. Email:/path/emails.txt, Password:/path/passwords.txt, Type:\"login\"):")
	if payloadTpl == "" || payloadTpl == "exit" {
		printer.PrintInfo("Cancelled.")
		return
	}

	fieldFiles, err := parsePayloadTemplate(payloadTpl)
	if err != nil {
		printer.PrintError("Invalid payload template: " + err.Error())
		return
	}

	// normalize file paths (expand ~) only for non-literals
	for k, p := range fieldFiles {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(p, "\"") && strings.HasSuffix(p, "\"") {
			// quoted literal, keep as-is
			fieldFiles[k] = p
			continue
		}
		fieldFiles[k] = filepath.Clean(p)
	}

	// read wordlists
	wordlists, minLen, err := input.ReadWordlists(fieldFiles)
	if err != nil {
		printer.PrintError("Error reading wordlists: " + err.Error())
		return
	}
	if minLen == 0 {
		printer.PrintError("No entries found in wordlists.")
		return
	}

	defaultCount := minLen
	count := input.ReadIntWithDefault("Number of requests to send (default: based on wordlist)", defaultCount)
	if count <= 0 {
		count = defaultCount
	}

	concurrency := input.ReadIntWithDefault("Concurrency (worker count, default 10)", 10)
	if concurrency <= 0 {
		concurrency = 10
	}

	useTor := false
	ans := input.ReadLine("Use Tor (SOCKS5 at 127.0.0.1:9050)? (y/N)")
	if strings.EqualFold(ans, "y") || strings.EqualFold(ans, "yes") {
		useTor = true
	}

	// Payload type selection
	printer.PrintInfo("Select payload type:")
	printer.PrintMenuOption("1", "application/json (default)")
	printer.PrintMenuOption("2", "application/x-www-form-urlencoded")
	printer.PrintMenuOption("3", "multipart/form-data")
	printer.PrintMenuOption("4", "binary (raw file)")
	printer.PrintMenuOption("5", "GraphQL (JSON query)")
	payloadChoice := input.ReadLine("Payload type (1-5, default 1):")
	payloadType := "json"
	switch payloadChoice {
	case "2":
		payloadType = "form"
	case "3":
		payloadType = "multipart"
	case "4":
		payloadType = "binary"
	case "5":
		payloadType = "graphql"
	}

	printer.PrintInfo("Default header: Content-Type will match payload type automatically")
	headers := input.ReadHeaders()

	// chunk size (how many jobs per batch)
	chunkSize := input.ReadIntWithDefault("Chunk size (jobs per batch, default 1000)", 1000)
	if chunkSize <= 0 {
		chunkSize = 1000
	}

	// rate limit (requests per second). 0 = unlimited
	rateLimit := input.ReadIntWithDefault("Rate limit (requests per second, 0 = unlimited)", 0)
	if rateLimit < 0 {
		rateLimit = 0
	}

	// optional per-request log
	logPath := ""
	lAns := input.ReadLine("Write per-request log to CSV? (y/N)")
	if strings.EqualFold(lAns, "y") || strings.EqualFold(lAns, "yes") {
		logPath = input.ReadLine("Enter log file path (e.g. ./results.csv):")
		if logPath == "" || logPath == "exit" {
			printer.PrintInfo("Cancelled logging; continuing without log.")
			logPath = ""
		}
	}

	printer.PrintAction("Starting to send requests...")

	// build payload generator closure
	payloadGen := func(i int) map[string]string {
		payload := map[string]string{}
		for field, list := range wordlists {
			if len(list) == 0 {
				payload[field] = ""
				continue
			}
			idx := i % len(list)
			payload[field] = list[idx]
		}
		return payload
	}

	stats, err := httpclient.SendMultipleRequests(
		"POST",
		url,
		headers,
		payloadGen,
		count,
		concurrency,
		useTor,
		payloadType,
		chunkSize,
		rateLimit,
		logPath,
	)

	if err != nil {
		printer.PrintError("Error sending requests: " + err.Error())
		return
	}

	// print aggregated summary in required format
	total := 0
	successTotal := 0
	failTotal := 0
	for _, c := range stats.TotalPerStatus {
		total += c
	}
	for status, c := range stats.TotalPerStatus {
		msg := stats.ExampleMessage[status]
		if status >= 200 && status < 300 {
			successTotal += c
			printer.PrintInfo(fmt.Sprintf("%d out of %d success status:%d message: %s", c, total, status, msg))
		} else {
			failTotal += c
			printer.PrintInfo(fmt.Sprintf("%d out of %d failed status:%d message: %s", c, total, status, msg))
		}
	}

	printer.PrintSuccess("Finished sending requests")
	printer.PrintInfo(fmt.Sprintf("Summary: total=%d success=%d failed=%d", total, successTotal, failTotal))
}

func parsePayloadTemplate(tpl string) (map[string]string, error) {
	out := map[string]string{}
	parts := strings.Split(tpl, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		kv := strings.SplitN(p, ":", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid token: %s", p)
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])
		if key == "" || val == "" {
			return nil, fmt.Errorf("empty key or path in: %s", p)
		}
		out[key] = val
	}
	return out, nil
}

func installTorProxychains() {
	printer.PrintInfo("This will attempt to install tor and proxychains (Debian/Ubuntu apt).")
	agree := input.ReadLine("Proceed with installation? (requires sudo) (y/N):")
	if !(strings.EqualFold(agree, "y") || strings.EqualFold(agree, "yes")) {
		printer.PrintInfo("Cancelled installation.")
		return
	}
	if err := httpclient.InstallTorProxychains(); err != nil {
		printer.PrintError("Installation failed: " + err.Error())
		return
	}
	printer.PrintSuccess("Installation attempted. Please check logs / system prompts for sudo.")
}
