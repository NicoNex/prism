package main

import (
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/NicoNex/prism/cube"
	"github.com/NicoNex/prism/hald"
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

func blend() error {
	opt := parseBlendOpts()

	c1, err := cube.LoadFile(opt.lut1)
	if err != nil {
		return err
	}

	c2, err := cube.LoadFile(opt.lut2)
	if err != nil {
		return err
	}

	c1.MustBlend(c2, opt.ilut1, opt.ilut2)

	if opt.output == "" {
		fmt.Println(c1)
		return nil
	}

	f, err := os.Create(opt.output)
	if err != nil {
		return err
	}
	defer f.Close()
	c1.WriteTo(f)

	return nil
}

type LUTApplicator interface {
	Apply(image.Image) *image.RGBA
	ApplyScaled(image.Image, float64) *image.RGBA
}

func encodeImg(format string, out io.Writer, img *image.RGBA) error {
	switch format {
	case "png":
		return png.Encode(out, img)
	case "jpeg":
		return jpeg.Encode(out, img, &jpeg.Options{Quality: 95})
	default:
		return fmt.Errorf("unsupported output format %s", format)
	}
}

func loadLut(path string) (LUTApplicator, error) {
	switch lutExt := filepath.Ext(path); lutExt {
	case ".cube":
		return cube.LoadFile(path)

	case ".png":
		return hald.LoadFile(path)

	default:
		return nil, fmt.Errorf("unsupported lut type: %s", lutExt[1:])
	}
}

func apply() error {
	opt := parseApplyOpts()
	lut, err := loadLut(opt.lut)

	f, err := os.Open(opt.imgPath)
	if err != nil {
		return err
	}
	defer f.Close()

	img, format, err := image.Decode(f)
	if err != nil {
		return err
	}

	if opt.output == "" {
		imgExt := filepath.Ext(opt.imgPath)
		imgName := opt.imgPath[:len(opt.imgPath)-len(imgExt)]
		opt.output = fmt.Sprintf("%s.prism%s", imgName, imgExt)
	}

	res := lut.ApplyScaled(img, opt.lutIntensity)
	outf, err := os.Create(opt.output)
	if err != nil {
		return err
	}
	defer outf.Close()
	return encodeImg(format, outf, res)
}

func convert() error {
	opt := parseConvertOpts()
	lutExt := filepath.Ext(opt.lut)
	outExt := filepath.Ext(opt.output)

	switch {
	case lutExt == ".cube" && outExt == ".png":
		c, err := cube.LoadFile(opt.lut)
		if err != nil {
			return err
		}

		f, err := os.Create(opt.output)
		if err != nil {
			return err
		}
		defer f.Close()
		return png.Encode(f, c.Apply(hald.Identity(12)))

	case lutExt == ".png" && outExt == ".cube":
		fallthrough
	default:
		return fmt.Errorf("unsupported conversion from %q to %q", lutExt, outExt)
	}
}

func check(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch cmd := os.Args[1]; cmd {
	case "blend":
		check(blend())
	case "apply":
		check(apply())
	case "convert":
		check(convert())
	default:
		fmt.Printf("unsupported command %q", cmd)
	}
}

type convertOpt struct {
	lut    string
	output string
}

func parseConvertOpts() (opt convertOpt) {
	if len(os.Args) < 4 {
		// TODO: print usage.
		os.Exit(1)
	}

	return convertOpt{os.Args[2], os.Args[3]}
}

type applyOpt struct {
	imgPath      string
	lut          string
	lutIntensity float64
	output       string
}

func parseApplyOpts() (opt applyOpt) {
	cmd := flag.NewFlagSet("apply", flag.ExitOnError)
	cmd.StringVar(&opt.output, "o", "", "Write the output in the given file")
	cmd.StringVar(&opt.output, "out", "", "Write the output in the given file")
	cmd.Parse(os.Args[2:])

	opt.lut, opt.lutIntensity = pathAndIntensity(cmd.Arg(0))
	opt.imgPath = cmd.Arg(1)
	return
}

type blendOpt struct {
	clamp  bool
	output string
	lut1   string
	lut2   string
	ilut1  float64
	ilut2  float64
}

func parseBlendOpts() (opt blendOpt) {
	cmd := flag.NewFlagSet("blend", flag.ExitOnError)
	cmd.BoolVar(&opt.clamp, "c", true, "Clamp the blended LUT")
	cmd.BoolVar(&opt.clamp, "clamp", true, "Clamp the blended LUT (same as -c)")
	cmd.StringVar(&opt.output, "o", "", "Write the output in the given file")
	cmd.StringVar(&opt.output, "out", "", "Write the output in the given file")
	cmd.Parse(os.Args[2:])

	opt.lut1, opt.ilut1 = pathAndIntensity(cmd.Arg(0))
	opt.lut2, opt.ilut2 = pathAndIntensity(cmd.Arg(1))
	return
}

func usage() {
	fmt.Println(`Usage coming soon!`)
}
