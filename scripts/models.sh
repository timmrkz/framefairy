#!/bin/sh
# Downloads the models into ~/.framefairy/models, where both programs look for
# them. Existing models are left alone, and a broken download resumes.
set -e
MODELS="${HOME}/.framefairy/models"
ASR_NAME=sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8
ASR_URL="https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/$ASR_NAME.tar.bz2"
LLM_NAME=gemma-4-26B_q4_0-it.gguf
LLM_URL="https://huggingface.co/google/gemma-4-26B-A4B-it-qat-q4_0-gguf/resolve/main/$LLM_NAME"

mkdir -p "$MODELS"

if [ -f "$MODELS/$ASR_NAME/tokens.txt" ]; then
	echo "Speech model already there"
else
	echo "Downloading the speech model, about 490 MB"
	curl -L --fail --continue-at - -o "$MODELS/$ASR_NAME.tar.bz2" "$ASR_URL"
	tar -xjf "$MODELS/$ASR_NAME.tar.bz2" -C "$MODELS"
	rm "$MODELS/$ASR_NAME.tar.bz2"
fi

if ls "$MODELS"/*.gguf >/dev/null 2>&1; then
	echo "Language model already there"
else
	echo "Downloading the language model, 14.4 GB. It needs about 18 GB of free memory to run."
	curl -L --fail --continue-at - -o "$MODELS/$LLM_NAME.part" "$LLM_URL"
	mv "$MODELS/$LLM_NAME.part" "$MODELS/$LLM_NAME"
fi

echo "Models are in $MODELS"
