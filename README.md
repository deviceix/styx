# Styx

Styx is a modern, lightweight build system for C and C++ projects. It provides simple configuration, 
fast incremental builds, and supports specialized environments like OSDev and embedded systems.

Styx uses TOML-based configuration files with minimal boilerplates; dependency tracking
and parallel compilation out of the box. Currently, Styx works on macOS and Linux, with
plans to support Cygwin and the Windows platform in the near future.

## Status

There isn't much about Styx yet as it is relatively new, with likely frequent configuration
option changing every update until it is stable. Therefore, please do not expect Styx
to be usable in production environment.

As of the time this is written, the schema is currently going on a redesign to support more
compiler features and QoL without having to break anything in the future.

## Installation

[!INFO]
> Your device is recommended to have GNU Make for ease of installation. 

```shell
git clone https://github.com/deviceix/styx.git
cd styx

make install
```

## Quick Start

```shell
# new project
mkdir -p my-project && cd my-project
styx init

# styx build (optional)
styx run

styx run
```

## Configuration

Styx uses TOML files for configuration. A basic example is provided 
below, see [docs]() for all available configuration options.

```toml
[project]
name = "example"
version = "0.1.0"
language = "c++"
standard = "c++17"

[build]
output_type = "executable"
output_name = "example"
sources = [ "src/*.cpp" ]
include_dirs = [ "include" ]

[toolchain]
compiler = "auto"
cxx_flags = [ "-Wall", "-Wextra" ]
linker_flags = []

[targets.debug]
cxx_flags = [ "-g", "-O0" ]

[targets.release]
cxx_flags = [ "-O2", "-DNDEBUG" ]
```

## Commands

- `styx init`: Creates a new project. The project is named after the root directory
- `styx build`: Build the project.
- `styx clean`: Clean build artifacts.
- `styx run`: Build and run the project.
- `styx compiler`: Show all available compilers and their information

## Contribution

Currently, Styx will not open to contribution until the core is stable.

## License

MIT License. See [license](LICENSE.txt) for more details.
