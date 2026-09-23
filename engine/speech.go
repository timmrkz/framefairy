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
// installer, and the app fetches it from whoever published it on first run.
// See docs/PACKAGING.md.
//
// There is one model today and the list is written for more, because the
// moment there are two the question becomes a choice somebody has to make
// and the app has to ask it. One is not a choice, so the app says
// what it is about to do rather than asking.

// SpeechModel is one recogniser that can be installed.
type SpeechModel struct {
	// Name is the folder it unpacks to, under the models folder, and the
	// name the engine knows it by.
	Name string `json:"name"`
	// Title is what it is called in the app.
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
	// URL is where it comes from, whoever published it rather than us. Nothing
	// is redistributed, so its licence is between the user and whoever
	// published it.
	URL string `json:"url"`
	// SHA256 of the archive. What arrives over a network is untrusted, and
	// a model that is not what it claims to be is not unpacked.
	SHA256 string `json:"-"`
	// Recommended marks the one the app offers when nobody has chosen.
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
	// Half an unpacking is never worth keeping. Half a download is: it is
	// what a second try carries on from, rather than beginning again at
	// nothing.
	defer os.RemoveAll(staging)

	// Bytes that turned out to be the wrong ones go, so a second try does
	// not carry on from them for ever.
	wrong := func(err error) error {
		os.Remove(part)
		return err
	}

	return log.Step("speech model", func() error {
		sum, err := download(ctx, log, m.URL, m.Download, part, m.Title)
		if err != nil {
			return err
		}
		if m.SHA256 != "" && !strings.EqualFold(sum, m.SHA256) {
			return wrong(renderErr("%s did not arrive as expected. It should be %s and came to %s.",
				m.Title, m.SHA256, sum))
		}

		// Unpacking has no share to report, so it says what it is doing
		// and the app shows work in hand without a number, which it
		// already knows how to do.
		// What is happening, and not what it is happening to: the job
		// carries the model's title, so saying it here as well would put
		// the same name on screen twice in one row.
		log.Progress("unpacking")
		if err := os.RemoveAll(staging); err != nil {
			return err
		}
		if err := unpackTarBz2(ctx, part, staging); err != nil {
			// An archive that will not unpack is an archive that is wrong,
			// whatever its checksum said, and there is nothing in it to
			// carry on from.
			if ctx.Err() == nil {
				return wrong(err)
			}
			return err
		}

		// The archive holds one folder named after the model. Take what is
		// inside it rather than nesting it a second time.
		inner := filepath.Join(staging, m.Name)
		if _, err := os.Stat(inner); err != nil {
			inner = staging
		}
		if _, err := os.Stat(filepath.Join(inner, "tokens.txt")); err != nil {
			return wrong(renderErr("%s unpacked without a tokens.txt, so it is not the model it claims to be.",
				m.Title))
		}
		if err := os.RemoveAll(final); err != nil {
			return err
		}
		if err := os.Rename(inner, final); err != nil {
			return err
		}
		log.ClearProgress()
		// The archive has done its work, so it goes.
		os.Remove(part)
		return nil
	})
}

// download writes what is at url to path and returns its checksum. The
// checksum is taken as the bytes go past rather than by reading the file
// again afterwards.
//
// It carries on where a stopped download left off. A language model is up
// to fifteen gigabytes, and one that drops at nine tenths of that should
// not begin again at nothing. What is already in path is what was already
// fetched, so the rest is asked for with a range and the bytes on disk go
// through the checksum first, in order, which leaves the same sum at the
// end as fetching the whole thing in one go.
//
// A server that will not do ranges answers with the whole file instead,
// and then the file is written again from the start. So resuming is an
// improvement where it works and never a way of going wrong.
//
// expect is how big it should be, used only to report how far the download
// has come when the server does not say, and to know a download that
// stopped early from one that finished. what is the name to say it by.
func download(ctx context.Context, log *Log, url string, expect int64, path, what string) (string, error) {
	have := int64(0)
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
		have = info.Size()
	}
	// A part file as big as the whole thing is a download that finished and
	// was never checked. Ask for the lot rather than for nothing, so there
	// is always a request to answer and always a sum to compare.
	if expect > 0 && have >= expect {
		have = 0
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	if have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", renderErr("could not reach %s: %s", what, err)
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusPartialContent:
		// Carrying on.
	case http.StatusOK:
		// The range was not honoured, so this is the whole file again.
		have = 0
	default:
		return "", renderErr("%s answered %s", url, res.Status)
	}

	sum := sha256.New()
	var file *os.File
	if have > 0 {
		if file, err = os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644); err != nil {
			return "", err
		}
		// The sum is of the whole file, so what is already on disk goes
		// through it before anything new does.
		if err := hashInto(sum, path, have); err != nil {
			file.Close()
			return "", err
		}
		log.Progress(fmt.Sprintf("carrying on with %s from %s", what, inMB(have)))
	} else if file, err = os.Create(path); err != nil {
		return "", err
	}
	defer file.Close()

	total := res.ContentLength + have
	if res.ContentLength <= 0 {
		total = expect
	}
	done, err := copyWithProgress(ctx, log, io.MultiWriter(file, sum), res.Body, have, total, "fetching")
	if err != nil {
		return "", err
	}
	if total > 0 && done < total {
		return "", renderErr("%s stopped after %s of %s.", what, inMB(done), inMB(total))
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// hashInto feeds the first n bytes of a file to a checksum.
func hashInto(sum io.Writer, path string, n int64) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(sum, io.LimitReader(file, n))
	return err
}

// copyWithProgress is io.Copy that says how far it has got, about once a
// second. More often than that is a job event a second for no reason, and
// the app redraws for every one of them.
//
// done starts at what was already there, so a download carrying on shows
// how far the file has come rather than how far this attempt has.
func copyWithProgress(ctx context.Context, log *Log, dst io.Writer, src io.Reader,
	already, total int64, what string) (int64, error) {
	buf := make([]byte, 256*1024)
	done := already
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
				if rate := float64(done-already) / time.Since(began).Seconds(); rate > 0 {
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
