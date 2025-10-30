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
	a1, a2, cfg := parseBlendFlags()
	p1, i1 := pathAndIntensity(a1)
	p2, i2 := pathAndIntensity(a2)

	c1, err := cube.LoadFile(p1)
	if err != nil {
		return err
	}

	c2, err := cube.LoadFile(p2)
	if err != nil {
		return err
	}

	c1.MustBlend(c2, i1, i2)

	if cfg.output == "" {
		fmt.Println(c1)
		return nil
	}

	f, err := os.Create(cfg.output)
	if err != nil {
		return err
	}
	defer f.Close()
	c1.WriteTo(f)

	return nil
}

type applier interface {
	Apply(image.Image) *image.RGBA
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

func loadLut(path string, intensity float64) (func(image.Image) *image.RGBA, error) {
	switch lutExt := filepath.Ext(path); lutExt {
	case ".cube":
		qbe, err := cube.LoadFile(path)
		if err != nil {
			return nil, err
		}
		return func(img image.Image) *image.RGBA {
			return qbe.ApplyScaled(img, intensity)
		}, nil

	case ".png":
		fallthrough

	default:
		return nil, fmt.Errorf("unsupported lut type: %s", lutExt[1:])
	}
}

func apply() error {
	lutAndIntensity, imgPath, cfg := parseApplyFlags()
	lutPath, lutIntensity := pathAndIntensity(lutAndIntensity)
	lut, err := loadLut(lutPath, lutIntensity)

	f, err := os.Open(imgPath)
	if err != nil {
		return err
	}
	defer f.Close()

	img, format, err := image.Decode(f)
	if err != nil {
		return err
	}

	if cfg.output == "" {
		imgExt := filepath.Ext(imgPath)
		imgName := imgPath[:len(imgPath)-len(imgExt)]
		cfg.output = fmt.Sprintf("%s.prism%s", imgName, imgExt)
	}

	res := lut.Apply(img)
	outf, err := os.Create(cfg.output)
	if err != nil {
		return err
	}
	defer outf.Close()
	return encodeImg(format, outf, res)
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
	default:
		fmt.Printf("unsupported command %q", cmd)
	}
}

type applycfg struct {
	output string
}

func parseApplyFlags() (lut, image string, cfg applycfg) {
	cmd := flag.NewFlagSet("apply", flag.ExitOnError)
	cmd.StringVar(&cfg.output, "o", "", "Write the output in the given file")
	cmd.StringVar(&cfg.output, "out", "", "Write the output in the given file")
	cmd.Parse(os.Args[2:])

	return cmd.Arg(0), cmd.Arg(1), cfg
}

type blendcfg struct {
	clamp  bool
	output string
}

func parseBlendFlags() (lut1, lut2 string, cfg blendcfg) {
	cmd := flag.NewFlagSet("blend", flag.ExitOnError)
	cmd.BoolVar(&cfg.clamp, "c", true, "Clamp the blended LUT")
	cmd.BoolVar(&cfg.clamp, "clamp", true, "Clamp the blended LUT (same as -c)")
	cmd.StringVar(&cfg.output, "o", "", "Write the output in the given file")
	cmd.StringVar(&cfg.output, "out", "", "Write the output in the given file")
	cmd.Parse(os.Args[2:])

	return cmd.Arg(0), cmd.Arg(1), cfg
}

func usage() {
	fmt.Println(`Usage coming soon!`)
}
