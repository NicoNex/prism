package main

import (
	"fmt"
	"os"

	"github.com/NicoNex/prism/cube"
)

func main() {
	c, err := cube.LoadFile(os.Args[1])
	if err != nil {
		panic(err)
	}

	fmt.Println(c)
}
