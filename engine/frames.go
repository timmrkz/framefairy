package engine

import (
	"os"
	"path/filepath"

	"github.com/spf13/afero"

	"framefairy/engine/runlog"
)

// Log is the log the project's steps write to.
func (p *Project) Log() *runlog.Log { return p.engine.Log }

// Engine is the engine the project's steps run on.
func (p *Project) Engine() *Engine { return p.engine }

// DeleteWork removes everything made for an episode, folder and all, so
// that letting go of a video really does leave nothing behind: transcript,
// clip sets, captions, previews and renders. The training records are not
// in there, they live in one folder of their own, so they outlive it. The
// episode file itself is never touched. No caption file of the video
// preview is written into it while it goes, see HoldPreviews.
func DeleteWork(fsys afero.Fs, source string) error {
	work := WorkDir(source)
	if filepath.Ext(work) != ".framefairy" || filepath.Clean(work) == filepath.Clean(source) {
		return renderErr("refusing to delete %s", work)
	}
	defer HoldPreviews(work)()
	if err := fsys.RemoveAll(work); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
