package main

import (
	"fmt"
	"os"

	"github.com/similarityyoung/simiclaw/internal/hygiene"
)

func main() {
	docs, err := hygiene.ValidateSkills(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, doc := range docs {
		fmt.Printf("%s\t%s\n", doc.Path, doc.Name)
	}
}
