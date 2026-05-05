package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	handlers "codex-service-go/internal/handlers"
)

func main() {
	mode := flag.String("mode", "think-tags", "reasoning compat: think-tags|reasoning|both")
	model := flag.String("model", "gpt-5", "model name for output")
	in := flag.String("in", "", "input SSE file path (default: stdin)")
	flag.Parse()

	var r io.Reader
	if *in == "" {
		r = os.Stdin
	} else {
		f, err := os.Open(*in)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open input: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		r = f
	}
	res, err := handlers.AggregateSSEToChatFromReader(r, *model, *mode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "aggregate: %v\n", err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(res); err != nil {
		fmt.Fprintf(os.Stderr, "encode: %v\n", err)
		os.Exit(1)
	}
}
