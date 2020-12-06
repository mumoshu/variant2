package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

func main() {
	content, err := ioutil.ReadFile("go.mod")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var replaces []string

	for _, l := range strings.Split(string(content), "\n") {
		if !strings.HasPrefix(l, "replace ") {
			continue
		}

		if !strings.Contains(l, " => ") {
			fmt.Fprintf(os.Stderr, "Unexpected line: ` => ` expected: %s\n", l)
			os.Exit(1)
		}

		l = strings.ReplaceAll(l, "replace ", "")
		l = strings.ReplaceAll(l, " => ", "=")
		l = strings.ReplaceAll(l, " ", "@")
		l = strings.TrimRight(l, "\n")

		replaces = append(replaces, l)
	}

	fmt.Print(strings.Join(replaces, ","))
}
