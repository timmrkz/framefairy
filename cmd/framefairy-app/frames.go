package main

import (
	"encoding/binary"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"framefairy/engine"
)

// serveFrames streams the picture of an episode, decoded by ffmpeg, for a
// file the webview cannot decode itself, see engine.PreviewFrames. Each
// frame is the moment of the episode it starts at, 8 bytes, a float64 in
// little endian, then the frame in 8-bit I420 at the size asked for. It
// goes on until the episode ends or the page stops reading, which ends
// the request and with it ffmpeg.
//
//	/frames/?path=<episode>&from=<seconds>&w=<width>&h=<height>
func serveFrames(st *store, w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	path := q.Get("path")
	if path == "" || !filepath.IsAbs(path) || !st.Known(path) {
		http.NotFound(w, r)
		return
	}
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	from, err := strconv.ParseFloat(q.Get("from"), 64)
	if err != nil || math.IsNaN(from) || math.IsInf(from, 0) || from < 0 {
		from = 0
	}
	width, _ := strconv.Atoi(q.Get("w"))
	height, _ := strconv.Atoi(q.Get("h"))
	if width < 2 || height < 2 || width > 7680 || height > 4320 {
		http.Error(w, "a preview needs a width and a height", http.StatusBadRequest)
		return
	}
	width &^= 1
	height &^= 1
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	flush, _ := w.(http.Flusher)
	head := make([]byte, 8)
	e := engine.NewEngine(engine.NewLog(io.Discard, false, false))
	_ = e.PreviewFrames(r.Context(), path, from, width, height, func(at float64, frame []byte) error {
		binary.LittleEndian.PutUint64(head, math.Float64bits(at))
		if _, err := w.Write(head); err != nil {
			return err
		}
		if _, err := w.Write(frame); err != nil {
			return err
		}
		if flush != nil {
			flush.Flush()
		}
		return nil
	})
}
