# Prism

A Go library and toolset for working with LUTs (Look-Up Tables) for video and photo color grading.

## About

Prism is a comprehensive LUT manipulation tool designed to fill a gap in Linux tooling for reliable LUT processing. Whether you're working with color grades for video production, photography, or camera profiles (like those for the Panasonic Lumix S9), Prism provides a seamless workflow for converting, applying, and blending LUTs.

### Key Capabilities

- **Format Conversion**: Convert freely between CUBE, HALD PNG and Panasonic VLT
- **LUT Application**: Apply one or a chain of LUTs to an image with per-LUT intensity
- **Log Workflows**: Convert log footage to Rec.709, and rebase log LUTs onto Rec.709 input
- **LUT Algebra**: Compose LUTs into one, invert a LUT, blend LUTs with weighted interpolation
- **LUT Extraction**: Recover a LUT from an ungraded/graded image pair
- **Tetrahedral Interpolation**: Keeps the neutral axis exactly neutral
- **16-bit Output**: Images and HALD PNGs are written at 16 bits per channel
- **Pure Go**: No external dependencies

## Features

- **CUBE LUT Support**: Full read/write support for the CUBE LUT format
- **HALD PNG Support**: Full read/write support for HALD CLUT images
- **VLT Support**: Panasonic Varicam/Lumix `.vlt` LUTs (12-bit integer samples)
- **LUT Operations**:
  - Convert between any pair of the three formats
  - Apply a chain of LUTs to an image with variable intensity
  - Compose LUTs, invert LUTs, blend LUTs with weighted interpolation
  - Extract a LUT from a before/after image pair
  - Sum, scale, clamp and rescale LUT values
- **Tetrahedral and Trilinear Interpolation**: Tetrahedral by default
- **Pure Go Implementation**: No external dependencies

## Installation

### Building from Source

Build the command-line tool:
```bash
go build
```

The binary will be available as `./prism` in the current directory.

## Command-Line Tool

The `prism` command-line tool provides a comprehensive interface for LUT manipulation without requiring code.

### Getting Help

Display the general help message:
```bash
prism help
```

Get help for a specific command:
```bash
prism help apply
prism help convert
prism help blend
```

### Available Commands

| Command | Purpose |
|---|---|
| `apply` | Apply one or more LUTs to an image |
| `convert` | Convert between CUBE, HALD PNG and VLT |
| `compose` | Bake a chain of LUTs into a single LUT |
| `invert` | Compute the numerical inverse of a LUT |
| `delog` | Rebase a log-input LUT onto display-referred input |
| `extract` | Derive a LUT from an ungraded/graded image pair |
| `blend` | Blend two LUTs together |
| `identity` | Generate an identity HALD PNG |

Flags may appear anywhere in the command line, before or after the file arguments.

#### Apply

Apply one or more LUTs to an image, in the order given.

```bash
prism apply [OPTIONS] LUT[:INTENSITY]... IMAGE
```

**Options:**
- `-o, -out FILE` — write output to FILE (default: `IMAGE.prism.EXT`)
- `-i, -interp MODE` — `tetra` (default) or `tri`

**Examples:**

```bash
# Convert V-Log footage to Rec.709
prism apply vlog-to-rec709.cube clip.png -o clip709.png

# Convert and grade in one pass, look at 60% strength
prism apply vlog-to-rec709.cube look.cube:0.6 clip.png

# Old-style trilinear interpolation
prism apply -i tri mylut.png photo.jpg -o out.jpg
```

PNG output is written at 16 bits per channel, so a log-to-display conversion
does not band.

#### Convert

```bash
prism convert [OPTIONS] INPUT OUTPUT
```

Any of `.cube`, `.png` (HALD) and `.vlt` converts to any other; the format is
taken from the file extension.

**Options:**
- `-t, -title TITLE` — title metadata for CUBE output
- `-s, -size SIZE` — output lattice size, or HALD level for `.png`. The default
  keeps the source size, falling back to 33 for CUBE and 17 for VLT when the
  source is a dense HALD.

```bash
prism convert mylut.cube mylut.png
prism convert mylut.cube mylut.vlt          # for a Lumix S9
prism convert -t "My Grade" mylut.png mylut.cube
```

#### Compose

Bake a chain of LUTs into one. Each LUT is applied to the output of the
previous, so the result is equivalent to applying them in order.

```bash
prism compose [OPTIONS] LUT... -o OUTPUT
```

Use it when a look **outputs** log and you want it to output Rec.709:

```bash
prism compose look-vlog-out.cube vlog-to-rec709.cube -o look-rec709.cube
```

#### Invert

Compute the LUT that undoes another LUT.

```bash
prism invert [OPTIONS] LUT -o OUTPUT
```

```bash
prism invert vlog-to-rec709.cube -o rec709-to-vlog.cube
```

A LUT that clips or crushes is not invertible in those regions — the inverse
settles on the closest preimage there. In practice this means deep sub-black
and out-of-gamut colours will not round-trip.

#### Delog

Rebase a LUT that **expects** log input so that it expects Rec.709 input
instead, given the camera's log-to-display conversion LUT.

```bash
prism delog -c CONVERSION [OPTIONS] LUT -o OUTPUT
```

```bash
prism delog -c vlog-to-rec709.cube look-for-vlog.cube -o look-for-rec709.cube
```

It is shorthand for inverting the conversion LUT and composing it in front:

```bash
prism invert vlog-to-rec709.cube -o inv.cube
prism compose inv.cube look-for-vlog.cube -o look-for-rec709.cube
```

#### Extract

Derive the LUT that turns one image into another. Both images must be the same
size and the same frame, one ungraded and one graded.

```bash
prism extract [OPTIONS] SOURCE GRADED -o OUTPUT
```

**Options:**
- `-s, -size SIZE` — output lattice size, HALD level for `.png` (default 33)
- `-r, -smooth W` — smoothing weight, higher is smoother (default 0.05)

The cleanest source is an identity HALD, because it covers the entire colour
cube:

```bash
prism identity -s 8 -o identity.png
# grade identity.png in any image editor, save it as graded.png
prism extract identity.png graded.png -o look.cube
```

A photograph works too, but it only constrains the colours it actually
contains; the rest of the cube is filled in smoothly and left near-neutral.
Raise `-r` for noisy or heavily compressed pairs.

#### Blend

Blend two LUTs of the same format with weighted interpolation. Blending
averages two looks — to stack them instead, use `compose`.

```bash
prism blend [OPTIONS] LUT1[:INTENSITY1] LUT2[:INTENSITY2]
```

**Options:**
- `-c, -clamp` — clamp output LUT to valid range (default: true)
- `-o, -out FILE` — write output to a file (default: stdout)
- `-t, -title TITLE` — title metadata for CUBE output

```bash
prism blend lut1.cube:0.7 lut2.cube:0.3 -o blended.cube
```

#### Identity

Generate an identity HALD PNG, the starting point for `extract`.

```bash
prism identity [-s LEVEL] [-o FILE]
```

`-s` is the HALD level; the lattice is level² per axis and the image is
level³ pixels square. The default is 12.

## Library Usage

The `lut` package holds a format-agnostic 3D LUT that CUBE, HALD PNG and VLT
all convert to and from losslessly. It is the easiest entry point.

```go
package main

import (
	"log"

	"github.com/NicoNex/prism/lut"
)

func main() {
	conv, err := lut.LoadFile("vlog-to-rec709.cube")
	if err != nil {
		log.Fatal(err)
	}
	look, err := lut.LoadFile("look-for-vlog.cube")
	if err != nil {
		log.Fatal(err)
	}

	// Rebase the look onto Rec.709 input and save it as a VLT.
	rebased := lut.Compose(lut.Invert(conv, 0), look, 0)
	if err := lut.SaveFile("look-rec709.vlt", rebased, 0, "Look"); err != nil {
		log.Fatal(err)
	}
}
```

Applying a LUT to an image:

```go
l, err := lut.LoadFile("mylut.cube")
if err != nil {
	log.Fatal(err)
}

f, err := os.Open("photo.png")
if err != nil {
	log.Fatal(err)
}
defer f.Close()

src, _, err := image.Decode(f)
if err != nil {
	log.Fatal(err)
}

// Full strength, tetrahedral. Returns a 16-bit *image.NRGBA64.
out := l.Apply(src, 1.0, lut.Tetrahedral)

// Or half strength with the classic trilinear interpolation.
out = l.Apply(src, 0.5, lut.Trilinear)
```

Extracting a LUT from an image pair:

```go
before, _, _ := image.Decode(f1)
after, _, _ := image.Decode(f2)

// 0 selects the default lattice size (33) and smoothing.
l, err := lut.Extract(before, after, 0, 0)
if err != nil {
	log.Fatal(err)
}
lut.SaveFile("recovered.cube", l, 0, "Recovered")
```

The `cube`, `hald` and `vlt` packages remain available for direct access to
each file format, including the blending operations.

## Workflow Examples

### Grading V-Log footage from a Lumix S9

```bash
# 1. See the footage in Rec.709
prism apply vlog-to-rec709.cube clip.png -o clip709.png

# 2. Apply the conversion and a look together
prism apply vlog-to-rec709.cube teal-orange.cube:0.7 clip.png -o graded.png

# 3. Bake that whole chain into one LUT for the camera
prism compose vlog-to-rec709.cube teal-orange.cube -o baked.cube
prism convert baked.cube baked.vlt
```

### Using a V-Log look on Rec.709 footage

A look built for V-Log will not work on footage that is already Rec.709.
Rebase it:

```bash
prism delog -c vlog-to-rec709.cube look-for-vlog.cube -o look-for-rec709.cube
prism apply look-for-rec709.cube already-graded.png
```

### Turning a manual grade into a LUT

Grade an identity HALD in whatever editor you like, then extract the LUT:

```bash
prism identity -s 8 -o identity.png
# open identity.png in your editor, grade it, save as graded.png
prism extract identity.png graded.png -o mylook.cube
prism apply mylook.cube any-photo.jpg
```

This also works from a matched pair of real photographs, though a photograph
only pins down the colours it contains.

### Converting camera LUTs for RawTherapee

```bash
prism convert Panasonic_S9_Profile.cube Panasonic_S9_Profile.png
```

### Batch processing images

```bash
for image in *.png; do
    prism apply mylut.cube "$image"
done
```

## Project Structure

```
.
├── cube/           # CUBE LUT format: parsing, writing, blending
├── hald/           # HALD CLUT format: parsing, writing, blending
├── vlt/            # Panasonic VLT format: parsing, writing, blending
├── lut/            # Format-agnostic LUT: interpolation, compose, invert, extract
├── main.go         # Command-line interface
├── options.go      # Flag parsing and help text
└── README.md       # This file
```

## Technical Details

### Interpolation

`apply` defaults to **tetrahedral** interpolation, which splits each lattice
cell into six tetrahedra and interpolates barycentrically within the one
containing the sample. Its useful property is that it reproduces the neutral
axis exactly: if a LUT maps grey to grey, tetrahedral interpolation keeps
greys neutral, whereas trilinear introduces a small colour cast between
lattice points. Trilinear is still available with `-i tri`.

### Inversion

`invert` seeds the inverse by pushing the source lattice forward and splatting
it into the output lattice, diffuses that into nodes the forward mapping never
reaches, then refines every node with damped Gauss-Newton
(Levenberg-Marquardt) against a finite-difference Jacobian. Accuracy is
limited only by how invertible the input LUT actually is — a LUT that clips
highlights or crushes blacks has destroyed the information there, and no
inverse can bring it back.

### Extraction

`extract` treats the problem as regularised least squares: find the lattice
`L` minimising

```
‖A·L − graded‖² + smoothing·‖∇(L − identity)‖² + mu·‖L − identity‖²
```

where `A` is trilinear interpolation at each source pixel. Because every pixel
touches the 8 corners of a single lattice cell, `AᵀA` is nonzero only between
nodes at most one step apart, so it fits in a 27-point stencil that one pass
over the image fills in; the system is then solved directly by over-relaxed
Gauss-Seidel. The smoothness term is applied to the difference from identity
rather than to the LUT itself — penalising the LUT's own gradient would be
minimised by a constant LUT, flattening the whole cube to one colour.

### File formats

**CUBE** is a text 3D LUT with a title, domain bounds and one RGB triple per
lattice node, red varying fastest.

**HALD PNG** encodes the lattice as an image: a HALD of level *L* carries an
*L*²-per-axis lattice laid out across an *L*³ × *L*³ image. Prism writes them
at 16 bits per channel, since 8 bits quantises a LUT to 256 steps per channel
and visibly bands log-to-display conversions. Both depths are read.

**VLT** is Panasonic's Varicam/Lumix format: 12-bit integer samples in
`[0, 4095]`, typically on a 17³ lattice.

## License

Licensed under the GNU General Public License v3.0 - see the [LICENSE](./LICENSE) file for details.

## Contributing

Contributions are welcome! Whether you have bug reports, feature requests, or code improvements, please feel free to open an issue or submit a pull request.
