# Prism

A Go library and toolset for working with LUTs (Look-Up Tables) for video and photo color grading.

> **Status**: ⚠️ Work in Progress - This project is early-stage and under active development.

## About

Prism started as a personal project to fill a gap in Linux tooling for reliable LUT manipulation. Created to work with LUTs for my Panasonic Lumix S9 mirrorless camera, it aims to be a general-purpose tool and library for color grading workflows on Linux.

## Features

- **CUBE LUT Support**: Read, write, and manipulate 3D LUT files in the CUBE format
- **HALD CLUT Support**: Basic support for HALD (Hue Area Locus Descriptor) CLUTs
- **LUT Operations**:
  - Blend multiple LUTs with weighted interpolation
  - Sum LUTs together
  - Scale and clamp color values
  - Rescale LUT ranges
- **Pure Go Implementation**: No external dependencies for core functionality

## Project Structure

- `cube/` - CUBE LUT format library
- `hald/` - HALD CLUT format support
- `cmd/` - Command-line tools and utilities

## Command-Line Tool

The `prism` command-line tool provides easy-to-use utilities for LUT manipulation without writing code.

### Building the Tool

Install the tool directly:
```bash
go install github.com/NicoNex/prism/cmd/prism@latest
```

Or build it locally:
```bash
cd cmd/prism
go build -o prism
```

### Available Commands

#### Blend

Blend multiple LUTs together with weighted interpolation.

**Syntax:**
```bash
prism blend [options] <lut1>:<intensity1> <lut2>:<intensity2> ...
```

**Options:**
- `-o, -out <file>` - Write output to a file (default: stdout)
- `-c, -clamp` - Clamp the blended LUT to the valid range (default: true)

**Examples:**

Blend two LUTs with equal weight:
```bash
prism blend -o BlendedLUT.cube ExampleLUT1.cube ExampleLUT2.cube
```

Blend two LUTs with custom weights (80% LUT1, 20% LUT2):
```bash
prism blend -o BlendedLUT.cube ExampleLUT1.cube:0.8 ExampleLUT2.cube:0.2
```

Output to stdout instead of a file:
```bash
prism blend ExampleLUT1.cube:1.0 ExampleLUT2.cube:0.5
```

## Getting Started

### Installation

```bash
go get github.com/NicoNex/prism
```

### Basic Usage

```go
package main

import (
	"log"
	"github.com/NicoNex/prism/cube"
)

func main() {
	// Load a CUBE LUT file
	lut, err := cube.LoadFile("mylut.cube")
	if err != nil {
		log.Fatal(err)
	}

	// Manipulate the LUT
	lut.Scale(0.8)
	lut.Clamp()

	// Write it back
	err = lut.WriteTo(os.Stdout)
	if err != nil {
		log.Fatal(err)
	}
}
```

## License

Licensed under the GNU General Public License v3.0 - see the [LICENSE](LICENSE) file for details.

## Contributing

This is an early-stage project. Issues, suggestions, and contributions are welcome!
