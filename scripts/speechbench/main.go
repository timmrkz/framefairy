// speechbench measures how fast the speech model hears on this machine, and
// what the ways of running it do to the words. It is for deciding how the
// app runs the model, not part of the app:
//
//	go run ./scripts/speechbench -audio episode.mp4
//
// It reads a 16 kHz mono WAV by itself and anything else through ffmpeg,
// and uses the speech model in ~/.framefairy/models unless -model says
// otherwise.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"framefairy/asr"
	"framefairy/engine"
)

func main() {
	model := flag.String("model", filepath.Join(engine.ModelsDir(), "sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8"), "speech model folder")
	audio := flag.String("audio", "", "speech to hear: a 16 kHz mono WAV, or anything ffmpeg reads")
	seconds := flag.Float64("seconds", 180, "how much of it to hear in each run")
	providers := flag.String("providers", "cpu,coreml", "ways to run the model")
	threads := flag.String("threads", "4,8", "threads for the model")
	pieces := flag.String("pieces", "15,30", "piece lengths in seconds")
	batches := flag.String("batch", "1,2", "pieces heard in one pass")
	flag.Parse()
	samples, err := load(*audio)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	n := min(int(*seconds*16000), len(samples))
	samples = samples[:n]
	length := float64(n) / 16000
	fmt.Printf("%.0f s of speech, hearing it in fixed pieces\n\n", length)
	fmt.Println("| provider | threads | piece | at once | first piece | time | real time | words | words unlike cpu |")
	fmt.Println("|---|---|---|---|---|---|---|---|---|")
	reference := map[string][]string{}
	for _, provider := range list(*providers) {
		for _, t := range list(*threads) {
			count, _ := strconv.Atoi(t)
			began := time.Now()
			rec, err := asr.OpenWith(*model, asr.Options{Provider: provider, Threads: count})
			if err != nil {
				fmt.Printf("| %s | %s | cannot load: %v |\n", provider, t, err)
				continue
			}
			loaded := time.Since(began)
			// The first piece pays for setting the model up, which for
			// coreml is compiling it, so it is timed on its own.
			began = time.Now()
			rec.Recognize(samples[:min(5*16000, n)], 16000)
			first := time.Since(began) + loaded
			for _, p := range list(*pieces) {
				size, _ := strconv.ParseFloat(p, 64)
				var parts [][]float32
				for at := 0; at < n; at += int(size * 16000) {
					parts = append(parts, samples[at:min(at+int(size*16000), n)])
				}
				for _, b := range list(*batches) {
					batch, _ := strconv.Atoi(b)
					var words []string
					began := time.Now()
					for i := 0; i < len(parts); i += batch {
						for _, tokens := range rec.RecognizeMany(parts[i:min(i+batch, len(parts))], 16000) {
							for _, w := range engine.TokensToWords(tokens, 0) {
								words = append(words, strings.ToLower(strings.Trim(w.Text, ".,!?;:\"'")))
							}
						}
					}
					took := time.Since(began)
					key := p
					unlike := "-"
					if ref, ok := reference[key]; ok {
						unlike = strconv.Itoa(distance(ref, words))
					} else if provider == "cpu" {
						reference[key] = words
					}
					fmt.Printf("| %s | %s | %s s | %s | %.1f s | %.1f s | %.1fx | %d | %s |\n",
						provider, t, p, b, first.Seconds(), took.Seconds(), length/took.Seconds(), len(words), unlike)
				}
			}
			rec.Close()
		}
	}
}

func list(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// load reads speech as 16 kHz mono samples.
func load(path string) ([]float32, error) {
	if path == "" {
		return nil, fmt.Errorf("-audio is needed")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if samples, ok := wav(data); ok {
		return samples, nil
	}
	ffmpeg := "ffmpeg"
	if local, err := filepath.Abs("bin/ffmpeg"); err == nil {
		if _, err := os.Stat(local); err == nil {
			ffmpeg = local
		}
	}
	raw, err := exec.Command(ffmpeg, "-loglevel", "error", "-i", path, "-map", "0:a:0",
		"-ac", "1", "-ar", "16000", "-f", "f32le", "-").Output()
	if err != nil {
		return nil, fmt.Errorf("reading %s through ffmpeg: %w", path, err)
	}
	out := make([]float32, len(raw)/4)
	binary.Read(bytes.NewReader(raw), binary.LittleEndian, out)
	return out, nil
}

// wav reads a 16 kHz mono 16 bit WAV, and says false for anything else.
func wav(data []byte) ([]float32, bool) {
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, false
	}
	ok := false
	for at := 12; at+8 <= len(data); {
		id := string(data[at : at+4])
		size := int(binary.LittleEndian.Uint32(data[at+4 : at+8]))
		body := data[at+8 : min(at+8+size, len(data))]
		switch id {
		case "fmt ":
			ok = len(body) >= 16 && binary.LittleEndian.Uint16(body[2:4]) == 1 &&
				binary.LittleEndian.Uint32(body[4:8]) == 16000 && binary.LittleEndian.Uint16(body[14:16]) == 16
		case "data":
			if !ok {
				return nil, false
			}
			out := make([]float32, len(body)/2)
			for i := range out {
				out[i] = float32(int16(binary.LittleEndian.Uint16(body[2*i:]))) / 32768
			}
			return out, true
		}
		at += 8 + size + size%2
	}
	return nil, false
}

// distance is how many words must change to turn a into b.
func distance(a, b []string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := range a {
		cur := make([]int, len(b)+1)
		cur[0] = i + 1
		for j := range b {
			c := 1
			if a[i] == b[j] {
				c = 0
			}
			cur[j+1] = min(prev[j]+c, prev[j+1]+1, cur[j]+1)
		}
		prev = cur
	}
	return prev[len(b)]
}
