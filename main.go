package main

import (
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
		hld, err := hald.LoadFile(opt.lut)
		if err != nil {
			return err
		}

		const (
			lutSize  = 33
			lutSizeF = float64(lutSize - 1)
		)

		c := cube.Cube{
			Title:     opt.title,
			LUT3Dsize: lutSize,
			DomainMin: cube.Sample{R: 0, G: 0, B: 0},
			DomainMax: cube.Sample{R: 1, G: 1, B: 1},
			Samples:   make([]cube.Sample, lutSize*lutSize*lutSize),
		}

		if c.Title == "" {
			c.Title = opt.lut[:len(opt.lut)-len(lutExt)]
		}

		// Sample the HALD at each CUBE position
		for b := 0; b < lutSize; b++ {
			for g := 0; g < lutSize; g++ {
				for r := 0; r < lutSize; r++ {
					idx := r + g*lutSize + b*lutSize*lutSize

					s := &c.Samples[idx]
					s.R, s.G, s.B = hld.Interpolate(
						float64(r)/lutSizeF,
						float64(g)/lutSizeF,
						float64(b)/lutSizeF,
					)
				}
			}
		}

		f, err := os.Create(opt.output)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = c.WriteTo(f)
		return err

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

func help() error {
	if len(os.Args) < 3 {
		usageHelp()
		return nil
	}

	switch cmd := os.Args[2]; cmd {
	case "apply":
		usageApply()
	case "convert":
		usageConvert()
	case "blend":
		usageBlend()
	case "help":
		usageHelp()
	default:
		usageGeneral()
	}
	return nil
}

func main() {
	if len(os.Args) < 2 {
		usageGeneral()
		os.Exit(1)
	}

	switch cmd := os.Args[1]; cmd {
	case "blend":
		check(blend())
	case "apply":
		check(apply())
	case "convert":
		check(convert())
	case "help":
		check(help())
	default:
		fmt.Fprintf(os.Stderr, "unsupported command %q\n", cmd)
		usageGeneral()
		os.Exit(1)
	}
}
