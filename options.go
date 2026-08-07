package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

type convertOpt struct {
	lut    string
	output string
	title  string
	size   int
}

type applyOpt struct {
	imgPath string
	luts    []string
	interp  string
	output  string
}

type identityOpt struct {
	output string
	level  int
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

type composeOpt struct {
	luts   []string
	output string
	title  string
	size   int
}

type invertOpt struct {
	lut    string
	output string
	title  string
	size   int
}

type delogOpt struct {
	lut    string
	conv   string
	output string
	title  string
	size   int
}

type extractOpt struct {
	source    string
	graded    string
	output    string
	title     string
	size      int
	smoothing float64
}

// permute reorders args so that flags come before positional arguments.
// Go's flag package stops parsing at the first positional, which would make
// 'prism invert lut.cube -o out.cube' silently ignore -o. Everything after a
// literal "--" is left alone.
func permute(cmd *flag.FlagSet, args []string) []string {
	var flags, rest []string

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "--" {
			rest = append(rest, args[i+1:]...)
			break
		}
		if len(arg) < 2 || arg[0] != '-' {
			rest = append(rest, arg)
			continue
		}

		flags = append(flags, arg)
		name := strings.TrimLeft(arg, "-")
		if strings.ContainsRune(name, '=') {
			continue // value is already attached
		}

		f := cmd.Lookup(name)
		if f == nil {
			continue // unknown flag: let flag.Parse report it
		}
		if b, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && b.IsBoolFlag() {
			continue // takes no value
		}
		if i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}

	return append(flags, rest...)
}

// parse applies the flag set to the command's arguments, flags in any position.
func parse(cmd *flag.FlagSet) {
	cmd.Parse(permute(cmd, os.Args[2:]))
}

// stringVar registers the same target under a short and a long flag name.
func stringVar(cmd *flag.FlagSet, p *string, short, long, def, usage string) {
	cmd.StringVar(p, short, def, usage)
	cmd.StringVar(p, long, def, usage+" (same as -"+short+")")
}

func intVar(cmd *flag.FlagSet, p *int, short, long string, def int, usage string) {
	cmd.IntVar(p, short, def, usage)
	cmd.IntVar(p, long, def, usage+" (same as -"+short+")")
}

func parseConvertOpts() (opt convertOpt) {
	cmd := flag.NewFlagSet("convert", flag.ExitOnError)
	stringVar(cmd, &opt.title, "t", "title", "", "Specify the title to use for the generated lut")
	intVar(cmd, &opt.size, "s", "size", 0, "Output lattice size (HALD level for .png); 0 picks a default")
	cmd.Usage = usageConvert
	parse(cmd)

	opt.lut = cmd.Arg(0)
	opt.output = cmd.Arg(1)
	return
}

func parseApplyOpts() (opt applyOpt) {
	cmd := flag.NewFlagSet("apply", flag.ExitOnError)
	stringVar(cmd, &opt.output, "o", "out", "", "Write the output in the given file")
	stringVar(cmd, &opt.interp, "i", "interp", "tetra", "Interpolation: tetra or tri")
	cmd.Usage = usageApply
	parse(cmd)

	args := cmd.Args()
	if len(args) < 2 {
		return
	}
	opt.luts = args[:len(args)-1]
	opt.imgPath = args[len(args)-1]
	return
}

func parseBlendOpts() (opt blendOpt) {
	cmd := flag.NewFlagSet("blend", flag.ExitOnError)
	cmd.BoolVar(&opt.clamp, "c", true, "Clamp the blended LUT")
	cmd.BoolVar(&opt.clamp, "clamp", true, "Clamp the blended LUT (same as -c)")
	stringVar(cmd, &opt.output, "o", "out", "", "Write the output in the given file")
	stringVar(cmd, &opt.title, "t", "title", "", "Specify the title to use for the generated lut")
	cmd.Usage = usageBlend
	parse(cmd)

	opt.lut1, opt.ilut1 = pathAndIntensity(cmd.Arg(0))
	opt.lut2, opt.ilut2 = pathAndIntensity(cmd.Arg(1))
	return
}

func parseComposeOpts() (opt composeOpt) {
	cmd := flag.NewFlagSet("compose", flag.ExitOnError)
	stringVar(cmd, &opt.output, "o", "out", "", "Write the output in the given file")
	stringVar(cmd, &opt.title, "t", "title", "", "Specify the title to use for the generated lut")
	intVar(cmd, &opt.size, "s", "size", 0, "Output lattice size (HALD level for .png); 0 picks a default")
	cmd.Usage = usageCompose
	parse(cmd)

	opt.luts = cmd.Args()
	return
}

func parseInvertOpts() (opt invertOpt) {
	cmd := flag.NewFlagSet("invert", flag.ExitOnError)
	stringVar(cmd, &opt.output, "o", "out", "", "Write the output in the given file")
	stringVar(cmd, &opt.title, "t", "title", "", "Specify the title to use for the generated lut")
	intVar(cmd, &opt.size, "s", "size", 0, "Output lattice size (HALD level for .png); 0 picks a default")
	cmd.Usage = usageInvert
	parse(cmd)

	opt.lut = cmd.Arg(0)
	return
}

func parseDelogOpts() (opt delogOpt) {
	cmd := flag.NewFlagSet("delog", flag.ExitOnError)
	stringVar(cmd, &opt.conv, "c", "conv", "", "Log-to-display conversion LUT (e.g. V-Log to Rec.709)")
	stringVar(cmd, &opt.output, "o", "out", "", "Write the output in the given file")
	stringVar(cmd, &opt.title, "t", "title", "", "Specify the title to use for the generated lut")
	intVar(cmd, &opt.size, "s", "size", 0, "Output lattice size (HALD level for .png); 0 picks a default")
	cmd.Usage = usageDelog
	parse(cmd)

	opt.lut = cmd.Arg(0)
	return
}

func parseExtractOpts() (opt extractOpt) {
	cmd := flag.NewFlagSet("extract", flag.ExitOnError)
	stringVar(cmd, &opt.output, "o", "out", "", "Write the output in the given file")
	stringVar(cmd, &opt.title, "t", "title", "", "Specify the title to use for the generated lut")
	intVar(cmd, &opt.size, "s", "size", 0, "Output lattice size, or HALD level for .png (default: 33 / level 8)")
	cmd.Float64Var(&opt.smoothing, "r", 0, "Smoothing weight; higher is smoother (default 1)")
	cmd.Float64Var(&opt.smoothing, "smooth", 0, "Smoothing weight; higher is smoother (same as -r)")
	cmd.Usage = usageExtract
	parse(cmd)

	opt.source = cmd.Arg(0)
	opt.graded = cmd.Arg(1)
	return
}

func parseIdentityOpts() (opt identityOpt) {
	cmd := flag.NewFlagSet("identity", flag.ExitOnError)
	stringVar(cmd, &opt.output, "o", "out", "prism-identity.png", "Write the output in the given file")
	intVar(cmd, &opt.level, "s", "size", 12, "HALD level (lattice is level² per axis)")
	cmd.Usage = usageIdentity
	parse(cmd)

	return
}

func usageGeneral() {
	fmt.Fprintf(os.Stderr, `Usage: %s COMMAND [OPTIONS] ARGS

Commands:
  apply     Apply one or more LUTs to an image
  convert   Convert between LUT formats (CUBE, PNG HALD, VLT)
  compose   Bake a chain of LUTs into a single LUT
  invert    Compute the numerical inverse of a LUT
  delog     Rebase a log-input LUT onto display-referred input
  extract   Derive a LUT from an ungraded/graded image pair
  blend     Blend two LUTs together
  identity  Generate an identity PNG HALD LUT
  help      Display help for a command

Use '%s help COMMAND' for more information on a command.
`, os.Args[0], os.Args[0])
}

func usageApply() {
	fmt.Fprintf(os.Stderr, `Usage: %s apply [OPTIONS] LUT[:INTENSITY]... IMAGE

Apply one or more LUTs to an image, in the order given.

To convert log footage to Rec.709, pass the camera's conversion LUT. To grade
at the same time, pass the conversion LUT first and the look after it.

Options:
  -o, --out FILE      Write output to FILE (default: IMAGE.prism.EXT)
  -i, --interp MODE   tetra (default) or tri

Interpolation:
  tetra  Tetrahedral. Preserves the neutral axis exactly, so greys stay neutral.
  tri    Trilinear. The classic 8-corner method, kept for compatibility.

Arguments:
  LUT[:INTENSITY]     LUT file (.cube, .png HALD, .vlt), optional intensity 0-1
  IMAGE               Input image (PNG or JPEG)

PNG output is written at 16 bits per channel to preserve LUT precision.

Examples:
  %s apply vlog-to-rec709.cube clip.png
  %s apply vlog-to-rec709.cube look.cube:0.6 clip.png
  %s apply -i tri -o out.jpg lut.png photo.jpg
`, os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}

func usageIdentity() {
	fmt.Fprintf(os.Stderr, `Usage: %s identity [OPTIONS]

Generate an identity PNG HALD LUT. Grade it in any image editor, then feed the
original and the graded copy to '%s extract' to turn the grade into a LUT.

Options:
  -o, --out FILE    Write output to FILE (default: prism-identity.png)
  -s, --size LEVEL  HALD level (default: 12, a 144-per-axis lattice)

Examples:
  %s identity
  %s identity -s 8 -o identity.png
`, os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}

func usageConvert() {
	fmt.Fprintf(os.Stderr, `Usage: %s convert [OPTIONS] LUT OUTPUT

Convert between LUT formats. Any of CUBE, PNG HALD and VLT converts to any other.

Options:
  -t, --title TITLE   Specify title for generated LUT (when output is CUBE)
  -s, --size SIZE     Output lattice size, or HALD level when output is .png.
                      0 (default) keeps the source size, falling back to 33 for
                      CUBE and 17 for VLT when the source is a dense HALD.

Arguments:
  LUT                 Path to input LUT file
  OUTPUT              Path to output LUT file

PNG HALD output is written at 16 bits per channel.

Examples:
  %s convert input.cube output.png
  %s convert -s 33 input.png output.vlt
  %s convert -t "My LUT" input.vlt output.cube
`, os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}

func usageCompose() {
	fmt.Fprintf(os.Stderr, `Usage: %s compose [OPTIONS] LUT... -o OUTPUT

Bake a chain of LUTs into a single LUT. Each LUT is applied to the output of the
previous one, so the result is equivalent to applying them in order.

Use it to convert a LUT that OUTPUTS log into one that outputs Rec.709: put the
look first and the log-to-Rec.709 conversion LUT second.

Options:
  -o, --out FILE      Write output to FILE (required)
  -t, --title TITLE   Specify title for generated LUT
  -s, --size SIZE     Output lattice size (HALD level for .png)

Arguments:
  LUT...              Two or more LUT files, in application order

Examples:
  %s compose look-vlog-out.cube vlog-to-rec709.cube -o look-rec709.cube
  %s compose a.cube b.cube c.vlt -o chain.cube
`, os.Args[0], os.Args[0], os.Args[0])
}

func usageInvert() {
	fmt.Fprintf(os.Stderr, `Usage: %s invert [OPTIONS] LUT -o OUTPUT

Compute the numerical inverse of a LUT: the LUT that undoes it.

Inverting a V-Log to Rec.709 conversion gives you Rec.709 to V-Log, which is what
you need to feed a display-referred image into a LUT built for log.

A LUT that clips or crushes is not invertible in those regions; the inverse
settles on the closest preimage there.

Options:
  -o, --out FILE      Write output to FILE (required)
  -t, --title TITLE   Specify title for generated LUT
  -s, --size SIZE     Output lattice size (HALD level for .png)

Examples:
  %s invert vlog-to-rec709.cube -o rec709-to-vlog.cube
`, os.Args[0], os.Args[0])
}

func usageDelog() {
	fmt.Fprintf(os.Stderr, `Usage: %s delog -c CONVERSION [OPTIONS] LUT -o OUTPUT

Rebase a LUT that EXPECTS log input so that it expects display-referred input
instead. Give it the log-to-display conversion LUT for the same camera and the
result can be applied straight to Rec.709 footage.

It is shorthand for inverting the conversion LUT and composing it in front:

  %s invert conv.cube -o inv.cube
  %s compose inv.cube look.cube -o look-rec709.cube

Options:
  -c, --conv FILE     Log-to-display conversion LUT (required)
  -o, --out FILE      Write output to FILE (required)
  -t, --title TITLE   Specify title for generated LUT
  -s, --size SIZE     Output lattice size (HALD level for .png)

Arguments:
  LUT                 The log-input LUT to rebase

Examples:
  %s delog -c vlog-to-rec709.cube look-for-vlog.cube -o look-for-rec709.cube
`, os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}

func usageExtract() {
	fmt.Fprintf(os.Stderr, `Usage: %s extract [OPTIONS] SOURCE GRADED -o OUTPUT

Derive the LUT that turns SOURCE into GRADED. Both images must be the same size
and be the same frame, one ungraded and one graded.

The cleanest source is an identity HALD, because it covers the whole colour cube:

  %s identity -o identity.png
  # grade identity.png in your editor, save it as graded.png
  %s extract identity.png graded.png -o look.cube

A photograph works too, but it only constrains the colours it contains; the rest
of the cube is filled in smoothly and left near-neutral. Raise --smooth for noisy
or heavily compressed pairs.

Options:
  -o, --out FILE       Write output to FILE (required)
  -t, --title TITLE    Specify title for generated LUT
  -s, --size SIZE      Output lattice size, HALD level for .png (default: 33)
  -r, --smooth W       Smoothing weight, higher is smoother (default: 1)

Arguments:
  SOURCE               The ungraded image
  GRADED               The same image after grading

Examples:
  %s extract identity.png graded.png -o look.cube
  %s extract -r 4 before.jpg after.jpg -o look.cube
`, os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}

func usageBlend() {
	fmt.Fprintf(os.Stderr, `Usage: %s blend [OPTIONS] LUT1[:INTENSITY1] LUT2[:INTENSITY2]

Blend two LUTs together with optional intensity weighting.
Both LUTs must be the same format (.cube, .png HALD, or .vlt).

Blending averages two looks. To stack them instead, use '%s compose'.

Options:
  -c, --clamp         Clamp output LUT to valid range (default: true)
  -o, --out FILE      Write output to FILE
  -t, --title TITLE   Specify title for generated LUT (CUBE only)

Arguments:
  LUT1[:INTENSITY1]   First LUT file with optional intensity (0-1)
  LUT2[:INTENSITY2]   Second LUT file with optional intensity (0-1)

Examples:
  %s blend lut1.cube lut2.cube
  %s blend lut1.vlt:0.5 lut2.vlt:0.5
  %s blend -o output.png lut1.png lut2.png
`, os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}

func usageHelp() {
	fmt.Fprintf(os.Stderr, `Usage: %s help [COMMAND]

Display help for a command.

Arguments:
  COMMAND    apply, convert, compose, invert, delog, extract, blend or identity

Examples:
  %s help
  %s help apply
  %s help extract
`, os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}
