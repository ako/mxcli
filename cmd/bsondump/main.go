// Command bsondump prints an MPR v2 .mxunit as indented canonical extended
// JSON, so numeric/boolean properties and their BSON types are readable.
// Development helper; not part of the shipped CLI surface.
package main

import (
	"fmt"
	"os"

	"go.mongodb.org/mongo-driver/bson"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: bsondump <file.mxunit>")
		os.Exit(2)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var doc bson.M
	if err := bson.Unmarshal(data, &doc); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	out, err := bson.MarshalExtJSONIndent(doc, true, false, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(out))
}
