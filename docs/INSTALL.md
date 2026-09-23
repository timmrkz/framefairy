# Installing what framefairy needs

The command line and the app need the same tools and models. They only need
installing once per machine, and wiping the repository does not remove them.

On macOS, `make` does all of this page by itself, and `make check` shows
what is still missing. The models are not on it at all any more: the app
fetches the speech model and the language model on its first run, the way a
customer gets them, and `make models` is left for the command line, which
has nowhere to ask. The steps below are the same work by hand, for
every system.

## 1. A C compiler and Go

The speech recogniser is a native library, so building needs a C compiler
next to Go 1.27 or newer. The command line, the app and the training tool
share one module and one Go version.

That library travels with the program rather than being found in the Go
module cache, which exists on no machine but the one that built it. Doing
that needs `patchelf` on Linux. macOS uses `install_name_tool`, which comes
with the command line tools, and Windows keeps the DLLs beside the program
and needs no rewriting.

| System | Commands |
| --- | --- |
| macOS | `xcode-select --install` and `brew install go` |
| Windows | `winget install GoLang.Go` and `winget install MSYS2.MSYS2`, then in the MSYS2 UCRT64 shell `pacman -S mingw-w64-ucrt-x86_64-gcc` and add `C:\msys64\ucrt64\bin` to PATH |
| Debian, Ubuntu | `sudo apt install build-essential patchelf`, and Go 1.27 from go.dev, since the distribution's Go is older |

## 2. ffmpeg, with libass and libx264

`framefairy` checks both at startup, before anything is spent.

**macOS.** Homebrew's default `ffmpeg` is a slim build without libass. The
ffmpeg tap includes it:

```
brew tap homebrew-ffmpeg/ffmpeg
brew unlink ffmpeg
brew install homebrew-ffmpeg/ffmpeg/ffmpeg
```

Skip `brew unlink ffmpeg` if the slim one was never installed. Homebrew's
`ffmpeg-full` also works. It installs beside the slim one, so point `framefairy`
at it with `--ffmpeg /opt/homebrew/opt/ffmpeg-full/bin/ffmpeg`.

**Windows.** `winget install Gyan.FFmpeg`

**Debian, Ubuntu.** `sudo apt install ffmpeg`. Fedora's own `ffmpeg-free`
leaves out libx264, so use the RPM Fusion package there.

## 3. The speech model

NVIDIA's Parakeet TDT 0.6B v3, about 490 MB to download and 670 MB unpacked.
It covers 25 European languages, German and English among them.

```
mkdir -p ~/.framefairy/models
curl -L -o /tmp/parakeet.tar.bz2 \
  https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8.tar.bz2
tar -xjf /tmp/parakeet.tar.bz2 -C ~/.framefairy/models
```

That creates `~/.framefairy/models/sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8`,
where the tool looks by default. Put it elsewhere and pass `--asr-model DIR`,
or set `FRAMEFAIRY_ASR_MODEL`.

## 4. llama.cpp and a language model

The moments are chosen by a language model running on your machine through
llama.cpp's `llama-server`.

`make` builds one for you, from source, and puts it in `bin/` beside the
programs, where they look before the search path. That is the one a
customer gets, so it is the one worth running. `make llama` builds it again
from scratch.

If you would rather use one you already have, put it on the search path and
remove `bin/llama-server`, or name it with `--llm-server PATH` on the
command line or in the app's settings.

| System | If you want your own instead |
| --- | --- |
| macOS | `brew install llama.cpp` |
| Windows | `winget install llama.cpp` |
| Linux | a prebuilt `llama-server` from https://github.com/ggml-org/llama.cpp/releases, on PATH |

The tested model is Google's Gemma 4 26B A4B in its official 4-bit version,
a 14.4 GB download that needs about 18 GB of free memory while it runs:

```
mkdir -p ~/.framefairy/models
curl -L -o ~/.framefairy/models/gemma-4-26B_q4_0-it.gguf \
  https://huggingface.co/google/gemma-4-26B-A4B-it-qat-q4_0-gguf/resolve/main/gemma-4-26B_q4_0-it.gguf
```

The tool uses the only `.gguf` file in `~/.framefairy/models`. With several,
choose one with `--llm-model FILE`. Any model llama.cpp can run works, so a
machine with less memory can use a smaller Gemma 4 from Google's collection
at https://huggingface.co/collections/google/gemma-4-qat-q4-0, at some cost in
the quality of the choices.

The Claude API remains available with `--planner api`, see step 6.

## 5. For the app only

The app needs the system web view, which macOS and Windows already
have. Linux needs the development packages for it:

```
sudo apt install libgtk-4-dev libwebkitgtk-6.0-dev gstreamer1.0-libav gstreamer1.0-plugins-good
```

Building the app also needs Node.js, for its interface:

| System | Commands |
| --- | --- |
| macOS | `brew install node` |
| Windows | `winget install OpenJS.NodeJS.LTS` |
| Debian, Ubuntu | `sudo apt install nodejs npm` |

## 6. The API key, only for the API planner

On macOS, store it once in the keychain rather than in a file:

```
security add-generic-password -a framefairy -s anthropic-api-key -w
```

`ANTHROPIC_API_KEY` works on every system and takes priority. On Windows and
Linux it is currently the only option. Get the key from the Claude Console at
console.anthropic.com under Settings, then API keys.

## 7. Before the first build

Once, after unpacking or after a dependency changed, let Go resolve the
modules and write `go.sum`:

```
go mod tidy
```

The first build of either program downloads the speech library for your system into Go's
module cache.

The build needs Go's C support switched on. Go switches it off by itself when
it finds no C compiler, and the build then fails with "build constraints
exclude all Go files" for a `sherpa-onnx-go` package. Check with
`go env CGO_ENABLED`, which must print `1`. If it prints `0`, install the
compiler from step 1, or clear a saved setting with `go env -u CGO_ENABLED`,
or put `CGO_ENABLED=1` in front of the build command once.

The same shows up in an editor on its own. VS Code's Go extension runs the
language server with its own environment, so it can have C support off while
your terminal has it on. `asr.go` is then full of red, every `sherpa` name is
unknown, and `make` in the terminal works. The repository carries
`.vscode/settings.json` with `CGO_ENABLED=1` for that reason. After pulling
it, run "Go: Restart Language Server" from the command palette.

- **macOS and Linux:** a built program loads the library from that cache, so
  it keeps working when you move it, for instance with
  `sudo mv framefairy /usr/local/bin/`. Running `go clean -modcache` breaks it
  until you build again.
- **Windows:** copy `onnxruntime.dll`, `sherpa-onnx-c-api.dll` and
  `sherpa-onnx-cxx-api.dll` from
  `%USERPROFILE%\go\pkg\mod\github.com\k2-fsa\sherpa-onnx-go-windows@v1.13.8\lib\x86_64-pc-windows-gnu`
  next to the `.exe` you built.

Because of the native library, each platform is built on that platform, or
in CI with one machine per platform. Bundling the library and the model with
the app belongs to the packaging step later.
