package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/trestle-dev/trestle/internal/buildinfo"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "version" {
		_ = json.NewEncoder(os.Stdout).Encode(buildinfo.Current())
		return
	}

	fmt.Fprintln(os.Stderr, "trestle: server implementation begins at checkpoint CP01")
	os.Exit(2)
}
