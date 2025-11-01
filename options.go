package main

import (
	"flag"
	"fmt"
	"os"
)

type convertOpt struct {
	lut    string
	output string
	title  string
}

type applyOpt struct {
	imgPath      string
	lut          string
	lutIntensity float64
	output       string
}

type blendOpt struct {
	clamp  bool
	output string
	title  string
	lut1   string
	lut2   string
	ilut1  float64
	ilut2  float64
}

func parseConvertOpts() (opt convertOpt) {
	cmd := flag.NewFlagSet("convert", flag.ExitOnError)
	cmd.StringVar(&opt.title, "t", "", "Specify the title to use for the generated lut")
	cmd.StringVar(&opt.title, "title", "", "Specify the title to use for the generated lut (same as -t)")
	cmd.Usage = usageConvert
	cmd.Parse(os.Args[2:])

	opt.lut = cmd.Arg(0)
	opt.output = cmd.Arg(1)
	return
}

func parseApplyOpts() (opt applyOpt) {
	cmd := flag.NewFlagSet("apply", flag.ExitOnError)
	cmd.StringVar(&opt.output, "o", "", "Write the output in the given file")
	cmd.StringVar(&opt.output, "out", "", "Write the output in the given file")
	cmd.Usage = usageApply
	cmd.Parse(os.Args[2:])

	opt.lut, opt.lutIntensity = pathAndIntensity(cmd.Arg(0))
	opt.imgPath = cmd.Arg(1)
	return
}

func parseBlendOpts() (opt blendOpt) {
	cmd := flag.NewFlagSet("blend", flag.ExitOnError)
	cmd.BoolVar(&opt.clamp, "c", true, "Clamp the blended LUT")
	cmd.BoolVar(&opt.clamp, "clamp", true, "Clamp the blended LUT (same as -c)")
	cmd.StringVar(&opt.output, "o", "", "Write the output in the given file")
	cmd.StringVar(&opt.output, "out", "", "Write the output in the given file (same as -o)")
	cmd.StringVar(&opt.title, "t", "", "Specify the title to use for the generated lut")
	cmd.StringVar(&opt.title, "title", "", "Specify the title to use for the generated lut (same as -t)")
	cmd.Usage = usageBlend
	cmd.Parse(os.Args[2:])

	opt.lut1, opt.ilut1 = pathAndIntensity(cmd.Arg(0))
	opt.lut2, opt.ilut2 = pathAndIntensity(cmd.Arg(1))
	return
}

func usageGeneral() {
	fmt.Fprintf(os.Stderr, `Usage: %s COMMAND [OPTIONS] ARGS

Commands:
  apply    Apply a LUT to an image
  convert  Convert between LUT formats (CUBE <-> PNG HALD)
  blend    Blend two LUTs together
  help     Display help for a command

Use '%s help COMMAND' for more information on a command.
`, os.Args[0], os.Args[0])
}

func usageApply() {
	fmt.Fprintf(os.Stderr, `Usage: %s apply [OPTIONS] LUT IMAGE

Apply a LUT (CUBE or PNG HALD) to an image.

Options:
  -o, --out FILE    Write output to FILE (default: IMAGE.prism.EXT)

Arguments:
  LUT              Path to LUT file (CUBE or PNG HALD)
  IMAGE            Path to input image (PNG or JPEG)

Examples:
  %s apply lut.cube image.png
  %s apply -o output.jpg lut.png image.jpg
`, os.Args[0], os.Args[0], os.Args[0])
}

func usageConvert() {
	fmt.Fprintf(os.Stderr, `Usage: %s convert [OPTIONS] LUT OUTPUT

Convert between LUT formats.

Supported conversions:
  CUBE to PNG HALD    : %s convert lut.cube lut.png
  PNG HALD to CUBE    : %s convert lut.png lut.cube

Options:
  -t, --title TITLE    Specify title for generated LUT (HALD->CUBE only)

Arguments:
  LUT                 Path to input LUT file
  OUTPUT              Path to output LUT file

Examples:
  %s convert input.cube output.png
  %s convert -t "My LUT" input.png output.cube
`, os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}

func usageBlend() {
	fmt.Fprintf(os.Stderr, `Usage: %s blend [OPTIONS] LUT1[:INTENSITY1] LUT2[:INTENSITY2]

Blend two CUBE LUTs together with optional intensity weighting.

Options:
  -c, --clamp         Clamp output LUT to valid range (default: true)
  -o, --out FILE      Write output to FILE
  -t, --title TITLE   Specify title for generated LUT

Arguments:
  LUT1[:INTENSITY1]   First LUT file with optional intensity (0-1)
  LUT2[:INTENSITY2]   Second LUT file with optional intensity (0-1)

Examples:
  %s blend lut1.cube lut2.cube
  %s blend lut1.cube:0.5 lut2.cube:0.5
  %s blend -o output.cube lut1.cube lut2.cube
  %s blend -t "Blended" lut1.cube:0.7 lut2.cube:0.3
`, os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}

func usageHelp() {
	fmt.Fprintf(os.Stderr, `Usage: %s help [COMMAND]

Display help for a command.

Arguments:
  COMMAND    Command to get help for (apply, convert, or blend)

Examples:
  %s help
  %s help apply
  %s help convert
`, os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}
