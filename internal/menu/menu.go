package menu

import (
	"x1/internal/httpclient"
	"x1/internal/utils/input"
	"x1/internal/utils/printer"
)

func ShowMainMenu() bool {
	printer.PrintMenuTitle("=== X1 BOMBER CLI ===")
	printer.PrintMenuOption("1", "Send Registration Request")
	printer.PrintMenuOption("2", "Send Authentication Request")
	printer.PrintMenuOption("3", "Exit")

	choice := input.ReadLine("Select an option: ")

	switch choice {
	case "1":
		handleRequest("Registration")
	case "2":
		handleRequest("Authentication")
	case "3", "exit", "quit":
		return true
	default:
		printer.PrintError("Invalid option, try again.")
	}
	return false
}

func handleRequest(requestType string) {
	printer.PrintInfo("Preparing to send " + requestType + " request...")

	url := input.ReadLine("Enter target URL: ")
	if url == "" || url == "exit" {
		printer.PrintInfo("Cancelled.")
		return
	}

	body := input.ReadLine("Enter JSON body: ")
	if body == "" || body == "exit" {
		printer.PrintInfo("Cancelled.")
		return
	}

	headers := input.ReadHeaders()

	printer.PrintAction("Sending request...")
	resp, err := httpclient.SendRequest("POST", url, body, headers)

	if err != nil {
		printer.PrintError("Request failed: " + err.Error())
		return
	}

	httpclient.PrintResponse(resp)
}
