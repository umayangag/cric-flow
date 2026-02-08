package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"io"
	"log"
	"os"

	"github.com/umayangag/cric-info-scrapers/context-provider/server"
)

func main() {
	rootFlag := flag.String("root", "", "Path to project root")
	flag.Parse()

	// Logging to stderr so it doesn't interfere with stdout JSON-RPC
	log.SetOutput(os.Stderr)
	log.Println("Starting Context MCP Server...")

	var projectRoot string
	if *rootFlag != "" {
		projectRoot = *rootFlag
	} else {
		// Determine root
		wd, err := os.Getwd()
		if err != nil {
			log.Fatalf("Failed to get current working directory: %v", err)
		}
		projectRoot = server.FindProjectRoot(wd)
	}
	log.Printf("Project Root: %s", projectRoot)

	srv := server.NewServer(projectRoot)
	_ = srv.LoadContext() // Try to load context, log errors inside

	scanner := bufio.NewScanner(os.Stdin)
	// Increase buffer size just in case
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		var req server.Request
		if err := json.Unmarshal(line, &req); err != nil {
			log.Printf("Error unmarshaling: %v", err)
			continue
		}
		srv.HandleRequest(os.Stdout, &req)
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		log.Printf("Scanner error: %v", err)
	}
}
