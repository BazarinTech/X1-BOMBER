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

	// Payload template example:
	// Email:/path/to/emails.txt, Password:/path/to/passwords.txt
	payloadTpl := input.ReadLine("Enter payload template (e.g. Email:/path/emails.txt, Password:/path/passwords.txt):")
	if payloadTpl == "" || payloadTpl == "exit" {
		printer.PrintInfo("Cancelled.")
		return
	}

	// parse payload template into map[field]filepath
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
	useTor := false
	ans := input.ReadLine("Use Tor (SOCKS5 at 127.0.0.1:9050)? (y/N)")
	if strings.EqualFold(ans, "y") || strings.EqualFold(ans, "yes") {
		useTor = true
	}

	// optional headers
	printer.PrintInfo("Default header: Content-Type: application/json")
	headers := input.ReadHeaders()

	printer.PrintAction("Starting to send requests...")
	// build payload generator closure
	payloadGen := func(i int) map[string]string {
		// for each field pick line i % len(list)
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

	stats, err := httpclient.SendMultipleRequests("POST", url, headers, payloadGen, count, concurrency, useTor)
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
	// tpl: "Email:/path/a.txt, Password:/path/b.txt"
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
