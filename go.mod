module vargasparse

go 1.25.4

require (
	github.com/fatih/color v1.18.0
	github.com/gen2brain/go-fitz v1.24.15
	github.com/ledongthuc/pdf v0.0.0-20250511090121-5959a4027728
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58
)

require (
	github.com/ebitengine/purego v0.8.4 // indirect
	github.com/jupiterrider/ffi v0.5.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	golang.org/x/sys v0.33.0 // indirect
)

replace github.com/go-skynet/go-llama.cpp => ./lib/go-llama.cpp
