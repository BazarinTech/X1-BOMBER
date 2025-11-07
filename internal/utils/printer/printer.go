package printer

import (
	"fmt"

	"github.com/fatih/color"
)

var (
	info    = color.New(color.FgCyan).SprintFunc()
	success = color.New(color.FgGreen).SprintFunc()
	errMsg  = color.New(color.FgRed).SprintFunc()
	action  = color.New(color.FgYellow).SprintFunc()
	title   = color.New(color.Bold, color.FgHiBlue).SprintFunc()
)

func PrintHeader() {
	fmt.Println(title("=== X1 CLI Tool ==="))
	fmt.Println(info("Interactive request testing made simple.\n"))
}

func PrintInfo(msg string) {
	fmt.Println(info("[i] " + msg))
}

func PrintSuccess(msg string) {
	fmt.Println(success("✔ " + msg))
}

func PrintError(msg string) {
	fmt.Println(errMsg("✘ " + msg))
}

func PrintAction(msg string) {
	fmt.Println(action("... " + msg))
}

func PrintMenuTitle(msg string) {
	fmt.Println(title("\n" + msg))
}

func PrintMenuOption(key, desc string) {
	fmt.Printf("%s %s\n", info(key+")"), desc)
}
