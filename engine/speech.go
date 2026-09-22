package engine

import (
	"archive/tar"
	"compress/bzip2"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The speech models somebody can install, and installing one.
//
// No model ships with the app. The speech model is 490 MB, which is not an
// installer, and the app fetches it from its own home on first run instead.
// See docs/PACKAGING.md.
//
// There is one model today and the list is written for more, because the
// moment there are two the question becomes a choice somebody has to make
// and the window has to ask it. One is not a choice, so the window says
// what it is about to do rather than asking.

// SpeechModel is one recogniser that can be installed.
type SpeechModel struct {
	// Name is the folder it unpacks to, under the models folder, and the
	// name the engine knows it by.
	Name string `json:"name"`
	// Title is what it is called in the window.
	Title string `json:"title"`
	// About is one line saying what it is for.
	About string `json:"about"`
	// Languages it covers, in words.
	Languages string `json:"languages"`
	// Download is the size of the archive in bytes, Unpacked what it costs
	// on disk once it is there. Both are said before anything starts,
	// because a download nobody agreed to is a download nobody wanted.
	Download int64 `json:"download"`
	Unpacked int64 `json:"unpacked"`
	// URL is where it comes from, its own home rather than ours. Nothing
	// is redistributed, so its licence is between the user and whoever
	// published it.
	URL string `json:"url"`
	// SHA256 of the archive. What arrives over a network is untrusted, and
	// a model that is not what it claims to be is not unpacked.
	SHA256 string `json:"-"`
	// Recommended marks the one the window offers when nobody has chosen.
	Recommended bool `json:"recommended"`
}

// SpeechModels is every model that can be installed, best first.
func SpeechModels() []SpeechModel {
	return []SpeechModel{{
		Name:        "sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8",
		Title:       "Parakeet TDT 0.6B v3",
		About:       "NVIDIA's recogniser, quantised. Fast on any machine and accurate on speech.",
		Languages:   "25 European languages, German and English among them",
		Download:    487_170_055,
		Unpacked:    671_239_000,
		URL:         "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8.tar.bz2",
		SHA256:      "5793d0fd397c5778d2cf2126994d58e9d56b1be7c04d13c7a15bb1b4eafb16bf",
		Recommended: true,
	}}
}

// SpeechModelByName finds one by the name it unpacks to.
func SpeechModelByName(name string) (SpeechModel, bool) {
	for _, m := range SpeechModels() {
		if m.Name == name {
			return m, true
		}
	}
	return SpeechModel{}, false
}

// ModelsDir is where models live, ~/.framefairy/models.
func ModelsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "models"
	}
	return filepath.Join(home, ".framefairy", "models")
}

// Installed says whether this model is there and usable. A folder alone is
// not enough: a download interrupted half way leaves one, and the app would
// then start a transcription that fails on the first second of audio.
// tokens.txt is the file the recogniser opens first.
func (m SpeechModel) Installed(dir string) bool {
	if dir == "" {
		dir = ModelsDir()
	}
	info, err := os.Stat(filepath.Join(dir, m.Name, "tokens.txt"))
	return err == nil && !info.IsDir() && info.Size() > 0
}

// InstallSpeechModel fetches a model and unpacks it, reporting as it goes.
//
// It is written to be interrupted. The download lands beside the folder
// under a part name, the unpacking goes to a folder of its own, and only a
// complete model is moved into place, so a machine that loses power or a
// person who presses cancel is left with no model rather than half of one.
func InstallSpeechModel(ctx context.Context, log *Log, m SpeechModel, dir string) error {
	if dir == "" {
		dir = ModelsDir()
	}
	if m.Installed(dir) {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	part := filepath.Join(dir, m.Name+".part")
	staging := filepath.Join(dir, m.Name+".unpacking")
	final := filepath.Join(dir, m.Name)
	defer os.Remove(part)
	defer os.RemoveAll(staging)

	return log.Step("speech model", func() error {
		sum, err := download(ctx, log, m, part)
		if err != nil {
			return err
		}
		if m.SHA256 != "" && !strings.EqualFold(sum, m.SHA256) {
			return renderErr("%s did not arrive as expected. It should be %s and came to %s.",
				m.Title, m.SHA256, sum)
		}

		// Unpacking has no share to report, so it says what it is doing
		// and the window shows work in hand without a number, which it
		// already knows how to do.
		log.Progress("unpacking " + m.Title)
		if err := os.RemoveAll(staging); err != nil {
			return err
		}
		if err := unpackTarBz2(ctx, part, staging); err != nil {
			return err
		}

		// The archive holds one folder named after the model. Take what is
		// inside it rather than nesting it a second time.
		inner := filepath.Join(staging, m.Name)
		if _, err := os.Stat(inner); err != nil {
			inner = staging
		}
		if _, err := os.Stat(filepath.Join(inner, "tokens.txt")); err != nil {
			return renderErr("%s unpacked without a tokens.txt, so it is not the model it claims to be.",
				m.Title)
		}
		if err := os.RemoveAll(final); err != nil {
			return err
		}
		if err := os.Rename(inner, final); err != nil {
			return err
		}
		log.ClearProgress()
		return nil
	})
}

// download writes the archive to path and returns its checksum. The
// checksum is taken as the bytes go past rather than by reading the file
// again afterwards.
func download(ctx context.Context, log *Log, m SpeechModel, path string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.URL, nil)
	if err != nil {
		return "", err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", renderErr("could not reach %s: %s", m.Title, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", renderErr("%s answered %s", m.URL, res.Status)
	}

	file, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	total := res.ContentLength
	if total <= 0 {
		total = m.Download
	}
	sum := sha256.New()
	done, err := copyWithProgress(ctx, log, io.MultiWriter(file, sum), res.Body, total,
		"fetching "+m.Title)
	if err != nil {
		return "", err
	}
	if total > 0 && done < total {
		return "", renderErr("%s stopped after %s of %s.", m.Title, inMB(done), inMB(total))
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// copyWithProgress is io.Copy that says how far it has got, about once a
// second. More often than that is a job event a second for no reason, and
// the window redraws for every one of them.
func copyWithProgress(ctx context.Context, log *Log, dst io.Writer, src io.Reader,
	total int64, what string) (int64, error) {
	buf := make([]byte, 256*1024)
	var done int64
	began := time.Now()
	last := began
	for {
		if err := ctx.Err(); err != nil {
			return done, ErrCancelled
		}
		n, err := src.Read(buf)
		if n > 0 {
			written, werr := dst.Write(buf[:n])
			done += int64(written)
			if werr != nil {
				return done, werr
			}
		}
		if time.Since(last) > time.Second {
			last = time.Now()
			if total > 0 {
				share := float64(done) / float64(total)
				left := Unknown
				if rate := float64(done) / time.Since(began).Seconds(); rate > 0 {
					left = float64(total-done) / rate
				}
				log.ProgressOf(fmt.Sprintf("%s, %s of %s", what, inMB(done), inMB(total)),
					share, left)
			} else {
				log.Progress(fmt.Sprintf("%s, %s", what, inMB(done)))
			}
		}
		if err == io.EOF {
			return done, nil
		}
		if err != nil {
			if ctx.Err() != nil {
				return done, ErrCancelled
			}
			return done, err
		}
	}
}

func inMB(n int64) string {
	if n >= 1<<30 {
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	}
	return fmt.Sprintf("%d MB", n/(1<<20))
}

// unpackTarBz2 writes an archive into a folder, refusing anything that
// would land outside it.
//
// What arrives over a network is untrusted. A tar can name ../../../ and
// a link can point anywhere, and both are how an archive writes over
// something it was never given. Every name goes through SafeChild, and no
// link of either kind is written at all: the models do not have any, so
// accepting one would only ever be somebody else's idea.
func unpackTarBz2(ctx context.Context, archive, into string) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := os.MkdirAll(into, 0o755); err != nil {
		return err
	}

	reader := tar.NewReader(bzip2.NewReader(file))
	for {
		if err := ctx.Err(); err != nil {
			return ErrCancelled
		}
		head, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return renderErr("the archive could not be read: %s", err)
		}
		switch head.Typeflag {
		case tar.TypeDir, tar.TypeReg:
		default:
			// Links, devices, anything else. Skipped rather than refused,
			// so one odd entry does not throw away a good model.
			continue
		}
		target, err := SafeChild(into, head.Name)
		if err != nil {
			return err
		}
		if head.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		// Bounded by what the header says, so a stream that claims one size
		// and keeps going cannot fill the disk.
		if _, err := io.Copy(out, io.LimitReader(reader, head.Size)); err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
	}
}
