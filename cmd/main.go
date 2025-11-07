package main

import (
	"bufio"
	"os"

	"x1/internal/menu"
	"x1/internal/utils/printer"
)

func main() {
	printer.PrintHeader()

	reader := bufio.NewReader(os.Stdin)

	for {
		exit := menu.ShowMainMenu()
		if exit {
			printer.PrintInfo("Goodbye 👋 Exiting X1.")
			break
		}

		printer.PrintInfo("Press Enter to return to main menu...")
		reader.ReadBytes('\n')
	}
}
