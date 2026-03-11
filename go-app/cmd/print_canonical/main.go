// Command print_canonical prints canonical config (e.g. format codes) as JSON
// for frontend-backend sync checks. Used by scripts/check-frontend-backend-sync.mjs.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/umayangag/cric-flow/go-app/internal/formats"
)

func main() {
	out := map[string]interface{}{
		"formats": formats.CanonicalCodes(),
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "encode: %v\n", err)
		os.Exit(1)
	}
}
