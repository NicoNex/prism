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

func blendCubes(opt blendOpt) error {
	c1, err := cube.LoadFile(opt.lut1)
	if err != nil {
		return err
	}

	c2, err := cube.LoadFile(opt.lut2)
	if err != nil {
		return err
	}

	blended, err := c1.Blend(c2, opt.ilut1, opt.ilut2)
	if err != nil {
		return err
	}

	if opt.title != "" {
		blended.Title = opt.title
	}

	if opt.output == "" {
		fmt.Println(blended)
		return nil
	}

	f, err := os.Create(opt.output)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = blended.WriteTo(f)
	return err
}

func blendHALDs(opt blendOpt) error {
	h1, err := hald.LoadFile(opt.lut1)
	if err != nil {
		return err
	}

	h2, err := hald.LoadFile(opt.lut2)
	if err != nil {
		return err
	}

	blended, err := h1.Blend(h2, opt.ilut1, opt.ilut2)
	if err != nil {
		return err
	}

	if opt.output == "" {
		ext := filepath.Ext(opt.lut1)
		b1 := filepath.Base(opt.lut1)
		b2 := filepath.Base(opt.lut2)

		n1 := b1[:len(b1)-len(ext)]
		n2 := b2[:len(b2)-len(ext)]

		opt.output = fmt.Sprintf("%s and %s%s", n1, n2, ext)
	}

	f, err := os.Create(opt.output)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = blended.WriteTo(f)
	return err
}

func blend() error {
	opt := parseBlendOpts()
	ext1 := strings.ToLower(filepath.Ext(opt.lut1))
	ext2 := strings.ToLower(filepath.Ext(opt.lut2))

	if ext1 != ext2 {
		return fmt.Errorf("cannot blend different extensions: %q, %q", ext1, ext2)
	}

	switch ext1 {
	case ".cube":
		return blendCubes(opt)
	case ".png":
		return blendHALDs(opt)
	default:
		return fmt.Errorf("unsupported LUT format: %q", ext1)
	}
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
	switch lutExt := strings.ToLower(filepath.Ext(path)); lutExt {
	case ".cube":
		return cube.LoadFile(path)

	case ".png":
		return hald.LoadFile(path)

	default:
		return nil, fmt.Errorf("unsupported lut type: %q", lutExt)
	}
}

func apply() error {
	opt := parseApplyOpts()
	lut, err := loadLut(opt.lut)
	if err != nil {
		return err
	}

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
		imgBase := filepath.Base(opt.imgPath)
		imgName := imgBase[:len(imgBase)-len(imgExt)]
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

func cubeToHald(lutPath, outPath string) error {
	c, err := cube.LoadFile(lutPath)
	if err != nil {
		return err
	}

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, c.Apply(hald.Identity(12)))
}

func haldToCube(title, lutPath, outPath string) error {
	hld, err := hald.LoadFile(lutPath)
	if err != nil {
		return err
	}

	const (
		lutSize  = 33
		lutSizeF = float64(lutSize - 1)
	)

	c := cube.Cube{
		Title:     title,
		LUT3Dsize: lutSize,
		DomainMin: cube.Sample{R: 0, G: 0, B: 0},
		DomainMax: cube.Sample{R: 1, G: 1, B: 1},
		Samples:   make([]cube.Sample, lutSize*lutSize*lutSize),
	}

	if c.Title == "" {
		lutExt := filepath.Ext(lutPath)
		c.Title = lutPath[:len(lutPath)-len(lutExt)]
	}

	// Sample the HALD at each CUBE position
	for b := range lutSize {
		for g := range lutSize {
			for r := range lutSize {
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

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = c.WriteTo(f)
	return err
}

func convert() error {
	opt := parseConvertOpts()
	lutExt := strings.ToLower(filepath.Ext(opt.lut))
	outExt := strings.ToLower(filepath.Ext(opt.output))

	switch {
	case lutExt == ".cube" && outExt == ".png":
		return cubeToHald(opt.lut, opt.output)

	case lutExt == ".png" && outExt == ".cube":
		return haldToCube(opt.title, opt.lut, opt.output)

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
