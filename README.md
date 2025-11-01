# Prism

A Go library and toolset for working with LUTs (Look-Up Tables) for video and photo color grading.

## About

Prism is a comprehensive LUT manipulation tool designed to fill a gap in Linux tooling for reliable LUT processing. Whether you're working with color grades for video production, photography, or camera profiles (like those for the Panasonic Lumix S9), Prism provides a seamless workflow for converting, applying, and blending LUTs.

### Key Capabilities

- **Format Conversion**: Seamlessly convert between CUBE and HALD PNG LUT formats
- **LUT Application**: Apply LUTs to images with adjustable intensity blending
- **LUT Blending**: Combine multiple LUTs with precise weighted interpolation
- **High Quality**: Supports up to 2025×2025 HALD PNG resolution for maximum fidelity
- **Pure Go**: No external dependencies for core functionality

## Features

- **CUBE LUT Support**: Full read/write support for the CUBE LUT format (3D color lookup tables)
- **HALD PNG Support**: Complete support for HALD (Hue Area Locus Descriptor) CLUT in PNG format
- **LUT Operations**:
  - Convert between CUBE and HALD PNG formats
  - Apply LUTs to images with variable intensity
  - Blend multiple LUTs with weighted interpolation
  - Sum LUTs together
  - Scale and clamp color values
  - Rescale LUT ranges
- **Trilinear Interpolation**: Smooth color transformations between LUT sample points
- **Pure Go Implementation**: No external dependencies for core functionality

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

#### Convert

Convert between CUBE and HALD PNG LUT formats. This is useful for:
- Converting camera-specific CUBE LUTs to HALD format for use in software that expects HALD
- Converting HALD LUTs back to CUBE format for compatibility with other tools
- Quality-preserving format conversion with high-resolution output

**Syntax:**
```bash
prism convert [OPTIONS] INPUT OUTPUT
```

**Options:**
- `-t, -title TITLE` - Set the title metadata in the output LUT (HALD→CUBE only)

**Supported Conversions:**

CUBE to HALD PNG (produces 2025×2025 high-quality output):
```bash
prism convert mylut.cube mylut.png
```

HALD PNG to CUBE (generates 33-point CUBE):
```bash
prism convert mylut.png mylut.cube
```

HALD PNG to CUBE with custom title:
```bash
prism convert -t "My Color Grade" mylut.png mylut.cube
```

#### Apply

Apply a LUT to an image with optional intensity blending. Supports both CUBE and HALD PNG formats.

**Syntax:**
```bash
prism apply [OPTIONS] LUT IMAGE
```

**Options:**
- `-o, -out FILE` - Write output to a specific file (default: creates `IMAGE.prism.EXT`)

**Examples:**

Apply a CUBE LUT to an image:
```bash
prism apply mylut.cube photo.png
```

Apply a HALD PNG LUT and save to a specific location:
```bash
prism apply -o output.jpg mylut.png photo.png
```

Apply multiple LUTs sequentially by chaining commands:
```bash
prism apply lut1.cube photo.png
prism apply -o final.png lut2.cube photo.prism.png
```

#### Blend

Blend two CUBE LUTs together with weighted interpolation. Create custom color grades by mixing existing LUTs.

**Syntax:**
```bash
prism blend [OPTIONS] LUT1[:INTENSITY1] LUT2[:INTENSITY2]
```

**Options:**
- `-c, -clamp` - Clamp output LUT to valid range (default: true)
- `-o, -out FILE` - Write output to a file (default: stdout)
- `-t, -title TITLE` - Set the title metadata in the output LUT

**Examples:**

Blend two LUTs with equal weight:
```bash
prism blend -o blended.cube lut1.cube lut2.cube
```

Blend with custom weights (70% first, 30% second):
```bash
prism blend -o blended.cube lut1.cube:0.7 lut2.cube:0.3
```

Blend and set custom title:
```bash
prism blend -t "Custom Grade" -o blended.cube lut1.cube lut2.cube
```

Blend three LUTs by chaining commands:
```bash
prism blend -o temp.cube lut1.cube lut2.cube
prism blend -o final.cube temp.cube:0.8 lut3.cube:0.2
```

## Library Usage

Use Prism as a Go library for programmatic LUT manipulation:

### Working with CUBE LUTs

```go
package main

import (
	"log"
	"os"
	"github.com/NicoNex/prism/cube"
)

func main() {
	// Load a CUBE LUT file
	lut, err := cube.LoadFile("mylut.cube")
	if err != nil {
		log.Fatal(err)
	}

	// Manipulate the LUT and write it back
	err = lut.Scale(0.8).Clamp().WriteTo(os.Stdout)
	if err != nil {
		log.Fatal(err)
	}
}
```

### Working with HALD LUTs

```go
package main

import (
	"log"
	"image/png"
	"os"
	"github.com/NicoNex/prism/hald"
)

func main() {
	// Load a HALD PNG LUT
	lut, err := hald.LoadFile("mylut.png")
	if err != nil {
		log.Fatal(err)
	}

	// Apply the LUT to an image
	img, err := os.Open("photo.png")
	if err != nil {
		log.Fatal(err)
	}
	defer img.Close()

	originalImg, _, err := image.Decode(img)
	if err != nil {
		log.Fatal(err)
	}

	// Apply with full intensity
	result := lut.Apply(originalImg)

	// Or apply with 50% intensity for subtle effect
	result = lut.ApplyScaled(originalImg, 0.5)

	// Save the result
	out, err := os.Create("output.png")
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	png.Encode(out, result)
}
```

### Converting Between Formats Programmatically

```go
package main

import (
	"log"
	"image/png"
	"os"
	"github.com/NicoNex/prism/cube"
	"github.com/NicoNex/prism/hald"
)

func main() {
	// Convert CUBE to HALD
	cubeLut, err := cube.LoadFile("mylut.cube")
	if err != nil {
		log.Fatal(err)
	}

	// Create a high-quality identity HALD and apply the CUBE to it
	identity := hald.IdentityHighQuality()
	result := cubeLut.Apply(identity)

	out, err := os.Create("output.png")
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	png.Encode(out, result)
}
```

## Workflow Examples

### Converting Camera LUTs for RawTherapee

Many cameras come with CUBE LUT profiles. To use them in RawTherapee (which expects HALD format):

```bash
# Convert your camera's CUBE LUT to HALD PNG
prism convert Panasonic_S9_Profile.cube Panasonic_S9_Profile.png

# Use the PNG in RawTherapee's color grading tools
```

### Creating a Custom Grade from Multiple LUTs

```bash
# Start with two existing LUTs
prism blend -o step1.cube vintage.cube:0.6 cinematic.cube:0.4

# Fine-tune by blending with a warm tone LUT
prism blend -o final.cube step1.cube:0.8 warm.cube:0.2

# Convert to HALD if needed for other software
prism convert final.cube final.png
```

### Batch Processing Images

Apply a LUT to all PNG images in a directory:

```bash
for image in *.png; do
    prism apply mylut.cube "$image"
done
```

### Testing LUT Intensity

Apply a LUT with varying intensity to find the right blend:

```bash
# Create multiple versions with different intensities
prism apply lut.cube -o output_50.png original.png  # Full strength
# For 50% intensity, blend the original with the LUT-applied version
```

## Project Structure

```
.
├── cube/           # CUBE LUT format library
├── hald/           # HALD CLUT format support
├── main.go         # Command-line interface
├── usage.go        # Help text and usage documentation
└── README.md       # This file
```

## Technical Details

### CUBE LUT Format

CUBE is a simple text-based 3D LUT format commonly used in color grading software. It consists of:
- A 3D array of RGB color samples
- Metadata (title, domain min/max)
- Linear interpolation between sample points

### HALD PNG Format

HALD (Hue Area Locus Descriptor) is an image-based LUT format where color transformations are encoded as a PNG image:
- Pixel position encodes input color coordinates
- Pixel color is the output color
- Supports trilinear interpolation for smooth transitions
- Higher resolution images provide better quality

## License

Licensed under the GNU General Public License v3.0 - see the [LICENSE](./LICENSE) file for details.

## Contributing

Contributions are welcome! Whether you have bug reports, feature requests, or code improvements, please feel free to open an issue or submit a pull request.
