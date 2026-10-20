package engine

// The names a plan file gives its parts, in one place. A plan is read and
// changed as an ordered object, so that the order of its keys and any key
// this version does not know survive an edit, see edit.go, and that object
// is reached by these names. The structs that write a new plan, PlanFile,
// PlanClip and PlanSegment, name the same keys in their tags, which Go
// only takes as written out, and a test holds the two together, see
// plankeys_test.go.
const (
	// The plan itself.
	keyClips        = "clips"
	keyPlannedWith  = "planned_with"
	keyCaptionStyle = "caption_style"
	keyRevision     = "revision"
	keyPlanID       = "plan_id"
	// keyFrom and keyTo are the edges of the window under planned_with.
	keyFrom = "from"
	keyTo   = "to"
	// keyShorts is where each clip's short was rendered, by the clip's id,
	// see shortOf. It is the plan's and not the clip's, because
	// a render is no edit: an undo compares and puts back clips, and a
	// short written in between would make every edit before it one that
	// cannot be undone.
	keyShorts = "shorts"
	// Under shorts, each clip's short: the folder it went into, its file
	// name, and its size and modification time in nanoseconds when it
	// was written, so a file written over since is not taken for it.
	keyFolder   = "folder"
	keyName     = "name"
	keySize     = "size"
	keyModified = "modified"

	// A clip.
	keyID           = "id"
	keySlug         = "slug"
	keyTitle        = "title"
	keyReason       = "reason"
	keyKeep         = "keep"
	keySegments     = "segments"
	keyRejected     = "rejected"
	keyCaptionY     = "caption_y"
	keyCaptionTimes = "caption_times"
	// keyRemovedWords is the words a clip does not show, see RemovedWords.
	keyRemovedWords = "removed_words"
	// keyCorrections is what the words of a clip read where they were
	// heard wrong, see Clip.Corrections.
	keyCorrections = "corrections"
	keyThumbnails  = "thumbnails"
	keyFound       = "found"
	// keyFoundSegments is the pieces a clip was found with, each framed for
	// the camera it showed. Plans from before #396 have it, and the next
	// change to the clip's pieces takes it away.
	keyFoundSegments = "found_segments"

	// A segment of a clip.
	keyStart = "start"
	keyEnd   = "end"
	keyCropX = "crop_x"
	// keyCropXAuto is where the crop was placed before a person moved it,
	// kept so it can be put back.
	keyCropXAuto = "crop_x_auto"
	// keyCropXTurned is the crop of a piece that is rotated, placed by
	// hand on the picture as it stands rotated. Absent, it stands in the
	// middle. The crop the piece has unrotated stays as it was, so a
	// stretch rotated back has it again.
	keyCropXTurned = "crop_x_turned"

	// keyRotations is the stretches of the video a person rotated, each
	// from, to and deg, the degrees to the left, on top of the turn the
	// file asks for itself, see Rotation. A stretch without from or to
	// runs to that end of the video.
	keyRotations = "rotations"
	keyDeg       = "deg"

	// keyCropMarks is the crops a person placed on frames of a clip, each
	// at, the moment the frame starts, x, the left edge of the crop there,
	// and turn, how far the picture was rotated when it was placed, see
	// CropMark.
	keyCropMarks = "crop_marks"
	keyAt        = "at"
	keyX         = "x"
	keyTurn      = "turn"
)
