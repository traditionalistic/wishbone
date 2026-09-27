// Command wbc is the Wishbone compiler CLI.
//
//	wbc conformance [-contract ../contract]           writes contract/conformance/compiled/*.json
//	wbc compile [-contract ../contract] screen.json   prints the wire document, or diagnostics (exit 1)
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"wishbone/backend/internal/compile"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	dir := fs.String("contract", "../contract", "contract directory")
	fs.Parse(os.Args[2:])
	c, err := compile.LoadContract(*dir)
	if err != nil {
		fail(err)
	}
	switch os.Args[1] {
	case "conformance":
		out, err := compile.Conformance(c, filepath.Join(*dir, "conformance"))
		if err != nil {
			fail(err)
		}
		dst := filepath.Join(*dir, "conformance", "compiled")
		if err := os.MkdirAll(dst, 0o755); err != nil {
			fail(err)
		}
		for _, name := range compile.ConformanceFiles {
			if err := os.WriteFile(filepath.Join(dst, name+".json"), out[name], 0o644); err != nil {
				fail(err)
			}
		}
		fmt.Printf("wrote %d files to %s\n", len(out), dst)
	case "compile":
		if fs.NArg() != 1 {
			usage()
		}
		src, err := os.ReadFile(fs.Arg(0))
		if err != nil {
			fail(err)
		}
		doc, _, diags := compile.Screen(c, src, 0)
		for _, d := range diags {
			fmt.Fprintln(os.Stderr, d)
		}
		if len(diags) > 0 {
			os.Exit(1)
		}
		os.Stdout.Write(append(doc, '\n'))
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: wbc conformance [-contract dir] | wbc compile [-contract dir] screen.json")
	os.Exit(2)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "wbc:", err)
	os.Exit(1)
}
