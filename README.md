# x1 - Interactive HTTP Request CLI

x1 is an interactive terminal tool (written in Go) that helps you send registration and authentication HTTP requests from a menu-driven CLI.

## Features
- Interactive menu (`x1`) with options for registration, authentication and exit
- Prompts for URL, HTTP method (default POST), JSON body, and optional headers
- Default header: `Content-Type: application/json`
- Shows status messages and response body/headers
- Exit via typing `exit` or pressing Ctrl+X (behavior depends on terminal)

## Quick start
1. `go build ./cmd`
2. Run `./cmd` (or `./x1` after renaming the binary)
3. Use the menu to send requests

## Notes
- This is a minimal skeleton using only the Go standard library.
- You can extend it with colors, better menu libraries (promptui), timeout settings, retries, proxy settings, and so on.
