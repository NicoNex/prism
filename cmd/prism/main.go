package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/NicoNex/prism/cube"
)

func pathAndIntensity(s string) (string, float64) {
	toks := strings.Split(s, ":")
	if len(toks) < 2 {
		return toks[0], 1
	}

	f, err := strconv.ParseFloat(toks[1], 64)
	if err != nil {
		fmt.Println(err)
		return toks[0], 1
	}
	return toks[0], f
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "blend":
		a1, a2, cfg := parseBlendFlags()
		p1, i1 := pathAndIntensity(a1)
		p2, i2 := pathAndIntensity(a2)

		c1, err := cube.LoadFile(p1)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		c2, err := cube.LoadFile(p2)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		c1.MustBlend(c2, i1, i2)

		if cfg.output == "" {
			fmt.Println(c1)
			return
		}

		f, err := os.OpenFile(cfg.output, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		defer f.Close()
		c1.WriteTo(f)
		return
	}
}

type blendFlags struct {
	clamp  bool
	output string
}

func parseBlendFlags() (lut1, lut2 string, b blendFlags) {
	cmd := flag.NewFlagSet("blend", flag.ExitOnError)
	cmd.BoolVar(&b.clamp, "c", true, "Clamp the blended LUT")
	cmd.BoolVar(&b.clamp, "clamp", true, "Clamp the blended LUT (same as -c)")
	cmd.StringVar(&b.output, "o", "", "Write the output in the given file")
	cmd.StringVar(&b.output, "out", "", "Write the output in the given file")
	cmd.Parse(os.Args[2:])

	return cmd.Arg(0), cmd.Arg(1), b
}

func usage() {
	fmt.Println(`Usage coming soon!`)
}
