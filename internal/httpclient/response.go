package httpclient

import (
	"encoding/json"
	"x1/internal/utils/printer"
)

func PrintResponse(resp *Response) {
	printer.PrintSuccess("Request completed with status: " + resp.Status)

	var pretty map[string]interface{}
	if json.Unmarshal([]byte(resp.Body), &pretty) == nil {
		prettyJSON, _ := json.MarshalIndent(pretty, "", "  ")
		printer.PrintInfo("Response JSON:\n" + string(prettyJSON))
	} else {
		printer.PrintInfo("Raw Response:\n" + resp.Body)
	}
}
