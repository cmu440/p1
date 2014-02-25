package main

import (
	"fmt"
	"os"
)

func main() {
	const numArgs = 2
	if len(os.Args) != numArgs {
		fmt.Println("Usage: ./miner <hostport>")
		return
	}

	// TODO: implement this!
}
