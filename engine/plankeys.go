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
	// keyRemoved, under planned_with, is the parts of the window a person
	// took out of the search. keyFrom and keyTo are the edges of the window
	// under planned_with, and of each part removed.
	keyRemoved = "removed"
	keyFrom    = "from"
	keyTo      = "to"

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
	keyThumbnails   = "thumbnails"

	// A segment of a clip.
	keyStart = "start"
	keyEnd   = "end"
	keyCropX = "crop_x"
	// keyCropXAuto is where the crop was placed before a person moved it,
	// kept so it can be put back.
	keyCropXAuto = "crop_x_auto"
)
