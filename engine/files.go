package engine

import "github.com/spf13/afero"

// The engine reads and writes the files of an episode through an afero.Fs,
// never through os directly. The programs hand it the real disk,
// afero.NewOsFs, and a test hands it memory, afero.NewMemMapFs, because a
// test is about what our code does with a plan or a transcript, not about
// the drive under it. The file system is held by what owns the files, the
// Engine, a Project and the app's store, and a function that has no owner
// takes it as its first argument. The training records and the timings
// of past searches are held the same way, by Records, which the engine and
// the app's store own and hand to the edits that record, see records.go.
// The fonts installed on the machine are the one shelf the whole program
// shares, as the disk is, because a caption style names a face wherever
// it is resolved. The programs put the disk's font folders on it and a
// test puts fonts in memory, see UseFontFolders.
//
// What an outside program reads or writes, ffmpeg, the frames decoder,
// llama-server, is on the real disk whatever the engine is given, because
// a separate process cannot see the memory of this one: the shorts, their
// pictures and caption files, llama-server's log, the models. The episode
// itself is read by ffmpeg from the disk, and by our own code, its size,
// its time and the fingerprint of the training records, through the file
// system given, which in the programs is the same disk. Every other file
// of the work folder, the plans, the saved answers, usage.json, the
// prompts and proof.txt, goes through the file system given.
//
// What a path and a file are in a file system, where a path leads, whether
// a name stays inside its folder, a file replaced in one step and one edit
// of a file at a time, is engine/fileio, under every part of the engine.

// disk is the real disk, for the files that are on it whatever the engine
// is given: the models, which llama-server reads, and the notes about the
// programs this one started. It is never swapped.
var disk afero.Fs = afero.NewOsFs()
