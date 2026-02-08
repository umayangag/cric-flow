package indexer

import (
	"bufio"
	"os"
	"regexp"
)

var (
	pyClassRegex = regexp.MustCompile(`^\s*class\s+([a-zA-Z0-9_]+)`)
	pyDefRegex   = regexp.MustCompile(`^\s*def\s+([a-zA-Z0-9_]+)`)
)

func ParsePy(path string) []Symbol {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var symbols []Symbol
	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if matches := pyClassRegex.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name: matches[1],
				Kind: "class",
				Line: lineNum,
			})
		} else if matches := pyDefRegex.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name: matches[1],
				Kind: "function",
				Line: lineNum,
			})
		}
	}
	return symbols
}
