package main

import (
	"errors"
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
	"github.com/NicoNex/prism/lut"
	"github.com/NicoNex/prism/vlt"
)

var errNoOutput = errors.New("no output file given, use -o")

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

	if err := c1.Blend(c2, opt.ilut1, opt.ilut2); err != nil {
		return err
	}

	if opt.title != "" {
		c1.Title = opt.title
	}

	if opt.output == "" {
		fmt.Println(c1)
		return nil
	}

	f, err := os.Create(opt.output)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = c1.WriteTo(f)
	return err
}

// blendedName derives a default output path for a two-LUT blend.
func blendedName(p1, p2 string) string {
	ext := filepath.Ext(p1)
	b1 := filepath.Base(p1)
	b2 := filepath.Base(p2)

	n1 := b1[:len(b1)-len(ext)]
	n2 := b2[:len(b2)-len(ext)]

	return fmt.Sprintf("%s and %s%s", n1, n2, ext)
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

	if err := h1.Blend(h2, opt.ilut1, opt.ilut2); err != nil {
		return err
	}

	if opt.output == "" {
		opt.output = blendedName(opt.lut1, opt.lut2)
	}

	f, err := os.Create(opt.output)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = h1.WriteTo(f)
	return err
}

func blendVLTs(opt blendOpt) error {
	v1, err := vlt.LoadFile(opt.lut1)
	if err != nil {
		return err
	}

	v2, err := vlt.LoadFile(opt.lut2)
	if err != nil {
		return err
	}

	if err := v1.Blend(v2, opt.ilut1, opt.ilut2); err != nil {
		return err
	}

	if opt.output == "" {
		opt.output = blendedName(opt.lut1, opt.lut2)
	}

	f, err := os.Create(opt.output)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = v1.WriteTo(f)
	return err
}

func blend() error {
	opt := parseBlendOpts()
	if opt.lut1 == "" || opt.lut2 == "" {
		usageBlend()
		return errors.New("blend needs two LUTs")
	}

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
	case ".vlt":
		return blendVLTs(opt)
	default:
		return fmt.Errorf("unsupported LUT format: %q", ext1)
	}
}

func encodeImg(format string, out io.Writer, img image.Image) error {
	switch format {
	case "png":
		return png.Encode(out, img)
	case "jpeg":
		return jpeg.Encode(out, img, &jpeg.Options{Quality: 95})
	default:
		return fmt.Errorf("unsupported output format %s", format)
	}
}

func loadImage(path string) (image.Image, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	return image.Decode(f)
}

func apply() error {
	opt := parseApplyOpts()
	if len(opt.luts) == 0 || opt.imgPath == "" {
		usageApply()
		return errors.New("apply needs at least one LUT and an image")
	}

	interp, err := lut.ParseInterp(opt.interp)
	if err != nil {
		return err
	}

	img, format, err := loadImage(opt.imgPath)
	if err != nil {
		return err
	}

	for _, spec := range opt.luts {
		path, intensity := pathAndIntensity(spec)
		l, err := lut.LoadFile(path)
		if err != nil {
			return err
		}
		img = l.Apply(img, intensity, interp)
	}

	if opt.output == "" {
		imgExt := filepath.Ext(opt.imgPath)
		imgBase := filepath.Base(opt.imgPath)
		imgName := imgBase[:len(imgBase)-len(imgExt)]
		opt.output = fmt.Sprintf("%s.prism%s", imgName, imgExt)
	}

	outf, err := os.Create(opt.output)
	if err != nil {
		return err
	}
	defer outf.Close()

	if err := encodeImg(format, outf, img); err != nil {
		return err
	}
	return outf.Close()
}

func convert() error {
	opt := parseConvertOpts()
	if opt.lut == "" || opt.output == "" {
		usageConvert()
		return errors.New("convert needs an input and an output LUT")
	}

	l, err := lut.LoadFile(opt.lut)
	if err != nil {
		return err
	}
	return lut.SaveFile(opt.output, l, opt.size, opt.title)
}

func compose() error {
	opt := parseComposeOpts()
	if len(opt.luts) < 2 {
		usageCompose()
		return errors.New("compose needs at least two LUTs")
	}
	if opt.output == "" {
		usageCompose()
		return errNoOutput
	}

	acc, err := lut.LoadFile(opt.luts[0])
	if err != nil {
		return err
	}
	for _, path := range opt.luts[1:] {
		next, err := lut.LoadFile(path)
		if err != nil {
			return err
		}
		acc = lut.Compose(acc, next, opt.size)
	}
	return lut.SaveFile(opt.output, acc, opt.size, opt.title)
}

func invert() error {
	opt := parseInvertOpts()
	if opt.lut == "" {
		usageInvert()
		return errors.New("invert needs a LUT")
	}
	if opt.output == "" {
		usageInvert()
		return errNoOutput
	}

	l, err := lut.LoadFile(opt.lut)
	if err != nil {
		return err
	}
	return lut.SaveFile(opt.output, lut.Invert(l, opt.size), opt.size, opt.title)
}

func delog() error {
	opt := parseDelogOpts()
	if opt.lut == "" || opt.conv == "" {
		usageDelog()
		return errors.New("delog needs a LUT and a conversion LUT (-c)")
	}
	if opt.output == "" {
		usageDelog()
		return errNoOutput
	}

	look, err := lut.LoadFile(opt.lut)
	if err != nil {
		return err
	}
	conv, err := lut.LoadFile(opt.conv)
	if err != nil {
		return err
	}

	// The look wants log input, so feed it display-to-log first.
	return lut.SaveFile(opt.output, lut.Compose(lut.Invert(conv, opt.size), look, opt.size), opt.size, opt.title)
}

func extract() error {
	opt := parseExtractOpts()
	if opt.source == "" || opt.graded == "" {
		usageExtract()
		return errors.New("extract needs a source and a graded image")
	}
	if opt.output == "" {
		usageExtract()
		return errNoOutput
	}

	src, _, err := loadImage(opt.source)
	if err != nil {
		return err
	}
	graded, _, err := loadImage(opt.graded)
	if err != nil {
		return err
	}

	// For HALD output -s is a level, so the lattice it stands for is level².
	fit, save := opt.size, opt.size
	if strings.ToLower(filepath.Ext(opt.output)) == ".png" {
		if save <= 0 {
			save = 8
		}
		fit = save * save
	} else if fit <= 0 {
		fit = 33
	}

	l, err := lut.Extract(src, graded, fit, opt.smoothing)
	if err != nil {
		return err
	}
	return lut.SaveFile(opt.output, l, save, opt.title)
}

func identity() error {
	opt := parseIdentityOpts()
	return lut.SaveFile(opt.output, lut.New(2), opt.level, "")
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
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
	case "compose":
		usageCompose()
	case "invert":
		usageInvert()
	case "delog":
		usageDelog()
	case "extract":
		usageExtract()
	case "blend":
		usageBlend()
	case "identity":
		usageIdentity()
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
	case "compose":
		check(compose())
	case "invert":
		check(invert())
	case "delog":
		check(delog())
	case "extract":
		check(extract())
	case "identity":
		check(identity())
	case "help":
		check(help())
	default:
		fmt.Fprintf(os.Stderr, "unsupported command %q\n", cmd)
		usageGeneral()
		os.Exit(1)
	}
}
