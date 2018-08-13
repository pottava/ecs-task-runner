package lib

import (
	"encoding/json"
	native "log"
	"os"
)

// Logger is a global logger for this application
var Logger = native.New(os.Stdout, "", 0)

// PrintJSON print JSON marshaled value
func PrintJSON(records interface{}) {
	marshaled, _ := json.MarshalIndent(records, "", "  ") // nolint
	Logger.Println(string(marshaled))
}
