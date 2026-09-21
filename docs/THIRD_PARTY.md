# Third-party notices

`framefairy`, `framefairy-app` and `framefairy-train` are built from the Go standard
library plus the third-party work listed here. All of it allows commercial use and distribution, on the
condition that the notices below ship with every copy. Include this file with
every build you distribute, and show it somewhere in the app's about screen
once there is one.

## Parakeet TDT 0.6B v3 (speech model)

`framefairy` uses NVIDIA's Parakeet TDT 0.6B v3 speech recognition model, in the
int8 ONNX conversion published by the sherpa-onnx project. The model is
downloaded separately and is not part of this source code. If a future build
ships the model, this attribution must ship with it.

- Model: nvidia/parakeet-tdt-0.6b-v3, © NVIDIA, https://huggingface.co/nvidia/parakeet-tdt-0.6b-v3
- Conversion: sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8, https://github.com/k2-fsa/sherpa-onnx
- Licence: Creative Commons Attribution 4.0 International (CC BY 4.0),
  https://creativecommons.org/licenses/by/4.0/
- Changes: the model was converted to ONNX and quantised to int8 by the
  sherpa-onnx project. `framefairy` uses it unmodified.

## sherpa-onnx (speech runtime)

The `asr` package uses the Go bindings of sherpa-onnx by the k2-fsa project,
which bring prebuilt native libraries for each platform.

- Source: https://github.com/k2-fsa/sherpa-onnx and
  https://github.com/k2-fsa/sherpa-onnx-go (version 1.13.8)
- Licence: Apache License 2.0, https://www.apache.org/licenses/LICENSE-2.0
- The full licence text is in the LICENSE file of each Go module
  (`github.com/k2-fsa/sherpa-onnx-go-macos`, `-linux`, `-windows`) and must be
  included with any build that ships those libraries.

## ONNX Runtime

The sherpa-onnx libraries include Microsoft's ONNX Runtime
(`onnxruntime.dll`, `libonnxruntime.dylib`, `libonnxruntime.so`).

- Source: https://github.com/microsoft/onnxruntime
- Licence: MIT License, Copyright (c) Microsoft Corporation

## pigo

The face detector in `engine/faces.go` is adapted from pigo by Endre Simo.
Only the upright detector, the clustering step and the cascade unpacking were
taken, and they were rewritten to fit this code base.

Source: https://github.com/esimov/pigo (version 1.4.6)

```
MIT License

Copyright (c) 2018 Endre Simo

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

## pico

The trained face data in `engine/facefinder` is the `facefinder` cascade from
pico by Nenad Markus, which pigo redistributes. The method is described in
N. Markus, M. Frljak, I. S. Pandzic, J. Ahlberg and R. Forchheimer, "Object
Detection with Pixel Intensity Comparisons Organized in Decision Trees",
http://arxiv.org/abs/1305.4537

Source: https://github.com/nenadmarkus/pico

```
MIT License

Copyright (c) 2013, Nenad Markus

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

## Wails (desktop app)

`framefairy-app` is built with Wails v3, which is compiled into the app together
with its own Go dependencies. The notices of those dependencies are collected
before the first release, as part of the packaging work.

- Source: https://github.com/wailsapp/wails (v3.0.0-beta.23), and its
  JavaScript runtime `@wailsio/runtime` from the same project
- Licence: MIT, © Lea Anthony

## Svelte (app interface)

The app's interface is written with Svelte, which compiles into the built
interface that ships inside the app.

- Source: https://github.com/sveltejs/svelte
- Licence: MIT, © Svelte contributors

Vite, TypeScript and the other packages in `frontend/package.json` under
`devDependencies` are only used to build the interface. Nothing from them
ships with the app.

## Gemma 4 (language model)

The default local planner is tested with Google's Gemma 4 26B A4B, in the
official 4-bit version. The model is downloaded separately and is not part of
this source code.

- Model: google/gemma-4-26B-A4B-it-qat-q4_0-gguf, © Google DeepMind
- Licence: Apache License 2.0, https://ai.google.dev/gemma/docs/gemma_4_license
- A future build that ships the model must include that licence and keep its
  notices.

## Caption fonts, built into the binary

The captions are written in faces that ship inside the programs, so no user
has to install a font and a short looks the same on every machine. Before a
render the face in use is written next to the caption file, together with the
licence text, and libass is pointed at that folder.

All three are under the SIL Open Font Licence 1.1
(https://openfontlicense.org), which allows bundling and selling the software
that carries them, as long as the licence travels with the font and the font
is not sold on its own. The files live in `engine/fonts/`.

- Inter, © 2016 The Inter Project Authors, https://github.com/rsms/inter
- Anton, © 2020 The Anton Project Authors, https://github.com/googlefonts/AntonFont
- Archivo Black, © 2017 The Archivo Black Project Authors,
  https://github.com/Omnibus-Type/ArchivoBlack

## Not included: llama.cpp

`framefairy` runs `llama-server` from llama.cpp as a separate program that you
install yourself. Nothing from llama.cpp is compiled into this binary. If a
future app bundles it, its MIT licence (https://github.com/ggml-org/llama.cpp)
must ship with it.

## Not included: ffmpeg

`framefairy` calls ffmpeg and ffprobe as separate programs that you install
yourself. Nothing from ffmpeg is compiled into this binary or shipped with it,
so none of its licence terms apply to a `framefairy` build as it stands.

That changes if a future app bundles ffmpeg to save its users the install. An
ffmpeg built with libx264, which `framefairy` needs, is GPL licensed, and shipping
it alongside a paid, closed-source app carries obligations that deserve a
careful look before release.
