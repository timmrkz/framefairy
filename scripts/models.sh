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

# The voices, for the dialogue recipe, which tells who speaks when. Two
# small files from the same place as the speech model, checked by checksum.
VOICES="$MODELS/voices"
mkdir -p "$VOICES"
SEG=sherpa-onnx-pyannote-segmentation-3-0
EMB=nemo_en_titanet_small.onnx
RELEASES=https://github.com/k2-fsa/sherpa-onnx/releases/download
check() {
	if command -v shasum >/dev/null 2>&1; then
		echo "$1  $2" | shasum -a 256 -c - >/dev/null
	else
		echo "$1  $2" | sha256sum -c - >/dev/null
	fi
}
if [ -f "$VOICES/$SEG/model.int8.onnx" ] && [ -f "$VOICES/$EMB" ]; then
	echo "Voice models already there"
else
	echo "Downloading the voice models, about 47 MB"
	curl -L --fail -o "$VOICES/$SEG.tar.bz2" "$RELEASES/speaker-segmentation-models/$SEG.tar.bz2"
	check 24615ee884c897d9d2ba09bb4d30da6bb1b15e685065962db5b02e76e4996488 "$VOICES/$SEG.tar.bz2"
	tar -xjf "$VOICES/$SEG.tar.bz2" -C "$VOICES"
	rm "$VOICES/$SEG.tar.bz2"
	curl -L --fail -o "$VOICES/$EMB.part" "$RELEASES/speaker-recongition-models/$EMB"
	check ad4a1802485d8b34c722d2a9d04249662f2ece5d28a7a039063ca22f515a789e "$VOICES/$EMB.part"
	mv "$VOICES/$EMB.part" "$VOICES/$EMB"
fi

echo "Models are in $MODELS"
