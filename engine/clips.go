package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Segment is one continuous stretch of the source that ends up in a clip.
type Segment struct {
	Start float64
	End   float64
	// CropX is the left edge of the crop in source pixels. Nil means centred.
	CropX *int
	// Moved means a person placed the crop by hand. The automatic placement
	// is kept in the plan as crop_x_auto.
	Moved bool
}

// Duration of the segment in seconds.
func (s Segment) Duration() float64 { return s.End - s.Start }

// Clip is one short, as the plan describes it.
type Clip struct {
	ID       string
	Slug     string
	Title    string
	Segments []Segment
	// The words this clip contains, on the source clock, each with the time
	// it was spoken. They are both the record of what is said and the source
	// of what is shown, which is what keeps the two in step.
	Words []Cue
	// Rejected clips stay in the plan but are left out of a full render.
	Rejected bool
	// CaptionY is where the captions sit in this clip, as the distance from
	// the bottom of a 1080x1920 frame, when it was placed by hand. Nil means
	// the place the caption style gives.
	CaptionY *float64
}

// Duration of the finished clip in seconds.
func (c Clip) Duration() float64 {
	lengths := make([]float64, len(c.Segments))
	for i, s := range c.Segments {
		lengths[i] = s.Duration()
	}
	return pysum(lengths)
}

// Basename is the file name shared by the clip's outputs.
func (c Clip) Basename() string {
	if c.Slug != "" {
		return c.ID + "_" + c.Slug
	}
	return c.ID
}

// Clip ids and slugs become both filenames and ffmpeg filter-graph tokens, so
// they are restricted to a character set that is inert in both. Everything
// else is collapsed to a hyphen rather than rejected, so a plausible-looking
// name still produces a usable file.
var unsafeNameRe = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// SanitiseName restricts a name to letters, digits, hyphen and underscore.
func SanitiseName(value, fallback string) string {
	cleaned := unsafeNameRe.ReplaceAllString(value, "-")
	cleaned = strings.Trim(cleaned, "-_")
	if len(cleaned) > 64 {
		cleaned = cleaned[:64]
	}
	if cleaned == "" {
		return fallback
	}
	return cleaned
}

// ResolvePath makes a path absolute and follows the links in whatever part
// of it exists. Two paths that lead to the same file give the same answer,
// which is what a check that one is inside another has to be made on.
func ResolvePath(path string) string { return resolvePath(path) }

func resolvePath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	// Resolve whatever part of the path exists.
	existing, rest := abs, ""
	for {
		if real, err := filepath.EvalSymlinks(existing); err == nil {
			return filepath.Join(real, rest)
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return abs
		}
		rest = filepath.Join(filepath.Base(existing), rest)
		existing = parent
	}
}

// SafeChild joins a name onto a directory and verifies the result stays
// inside it.
//
// Belt and braces next to SanitiseName. A path that escapes its directory is
// a bug worth failing loudly on, not one worth silently correcting.
func SafeChild(base, name string) (string, error) {
	root := resolvePath(base)
	joined := resolvePath(filepath.Join(base, name))
	rel, err := filepath.Rel(root, joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(rel) {
		return "", renderErr("refusing to write outside %s: %s", base, pyRepr(name))
	}
	return joined, nil
}

// ParseTime accepts seconds as a number, or a timecode string like
// 00:14:23.500.
func ParseTime(value any) (float64, error) {
	bad := func() error { return renderErr("invalid time value: %s", pyReprAny(value)) }
	var seconds float64
	switch v := value.(type) {
	case bool, nil:
		return 0, bad()
	case float64, int, json.Number:
		f, ok := toFloat(v)
		if !ok {
			return 0, bad()
		}
		seconds = f
	default:
		text := strip(pyStr(value))
		if !strings.Contains(text, ":") {
			f, ok := parsePyFloat(text)
			if !ok {
				return 0, fmt.Errorf("could not convert string to float: %s", pyRepr(text))
			}
			seconds = f
		} else {
			parts := strings.Split(strings.ReplaceAll(text, ",", "."), ":")
			if len(parts) > 3 {
				return 0, renderErr("invalid timecode: %s", pyReprAny(value))
			}
			numbers := make([]float64, 0, 3)
			for _, p := range parts {
				f, ok := parsePyFloat(p)
				if !ok {
					return 0, fmt.Errorf("could not convert string to float: %s", pyRepr(p))
				}
				numbers = append(numbers, f)
			}
			for len(numbers) < 3 {
				numbers = append([]float64{0}, numbers...)
			}
			seconds = float64(numbers[0]*3600) + float64(numbers[1]*60) + numbers[2]
		}
	}
	// NaN and infinity survive parsing and then compare false against
	// everything, which would sail straight through the ordering checks.
	if !isFinite(seconds) || seconds < 0 {
		return 0, bad()
	}
	return seconds, nil
}

func pyReprAny(v any) string {
	if s, ok := v.(string); ok {
		return pyRepr(s)
	}
	return pyStr(v)
}

// MaxSegments keeps the filter graph, which travels as a single command line
// argument, well clear of the operating system's argument size limit.
const MaxSegments = 300

// MaxClips is far more clips than a search asks for, and few enough that a
// file claiming a hundred thousand of them does not go on screen as a
// hundred thousand rows.
const MaxClips = 1000

// Plan is a clip plan as read from disk. Raw keeps everything in the file,
// including fields this version does not know about.
type Plan struct {
	Raw map[string]any
}

// CaptionStyle is the plan's caption_style object, or nil.
func (p Plan) CaptionStyle() map[string]any {
	style, _ := p.Raw["caption_style"].(map[string]any)
	out := map[string]any{}
	for k, v := range style {
		out[k] = v
	}
	return out
}

// PlannedWith is the settings stamp a plan was made under, or nil.
func (p Plan) PlannedWith() map[string]any {
	made, _ := p.Raw["planned_with"].(map[string]any)
	return made
}

func decodeJSON(data []byte, into any) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	return decoder.Decode(into)
}

// MaxEpisodeSeconds is as far into an episode as a plan may point. A
// hundred hours is longer than any episode and short of the numbers a
// broken or invented plan carries.
const MaxEpisodeSeconds = 100 * 3600

// LoadClips reads and validates a clip plan. The plan is treated as untrusted
// input, because it is generated from transcript text.
func LoadClips(path string) (Plan, []Clip, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Plan{}, nil, err
	}
	var top any
	if err := decodeJSON(data, &top); err != nil {
		return Plan{}, nil, err
	}
	raw, ok := top.(map[string]any)
	if !ok {
		return Plan{}, nil, renderErr("clip plan must be a JSON object")
	}
	entriesAny, present := raw["clips"]
	if !present {
		entriesAny = []any{}
	}
	entries, ok := entriesAny.([]any)
	if !ok {
		return Plan{}, nil, renderErr(`"clips" must be a list`)
	}
	if len(entries) > MaxClips {
		return Plan{}, nil, renderErr("the plan holds %d clips, the limit is %d",
			len(entries), MaxClips)
	}

	var clips []Clip
	for i, entryAny := range entries {
		index := i + 1
		entry, ok := entryAny.(map[string]any)
		if !ok {
			return Plan{}, nil, renderErr("clip %d is not an object", index)
		}
		rawSegmentsAny, present := entry["segments"]
		if !present {
			rawSegmentsAny = []any{}
		}
		rawSegments, ok := rawSegmentsAny.([]any)
		if !ok {
			return Plan{}, nil, renderErr(`clip %d: "segments" must be a list`, index)
		}
		if len(rawSegments) > MaxSegments {
			return Plan{}, nil, renderErr("clip %d has %d segments, the limit is %d",
				index, len(rawSegments), MaxSegments)
		}
		var segments []Segment
		for _, segAny := range rawSegments {
			seg, ok := segAny.(map[string]any)
			if !ok {
				return Plan{}, nil, renderErr("clip %d: segment is not an object", index)
			}
			var cropX *int
			if value, present := seg["crop_x"]; present && value != nil {
				text, isText := value.(string)
				lowered := strings.ToLower(text)
				if !(isText && (lowered == "center" || lowered == "centre" || lowered == "auto")) {
					n, ok := toInt(value)
					if !ok {
						return Plan{}, nil, fmt.Errorf("invalid crop_x: %s", pyReprAny(value))
					}
					if !(-1_000_000 < n && n < 1_000_000) {
						return Plan{}, nil, renderErr("crop_x out of range: %d", n)
					}
					cropX = &n
				}
			}
			startAny, okS := seg["start"]
			endAny, okE := seg["end"]
			if !okS {
				return Plan{}, nil, fmt.Errorf("'start'")
			}
			if !okE {
				return Plan{}, nil, fmt.Errorf("'end'")
			}
			start, err := ParseTime(startAny)
			if err != nil {
				return Plan{}, nil, err
			}
			end, err := ParseTime(endAny)
			if err != nil {
				return Plan{}, nil, err
			}
			// A piece that ends where it starts, or before it, cannot be cut
			// out of the episode. ffmpeg would be handed a negative length
			// and fail somewhere far from the cause.
			if end <= start {
				return Plan{}, nil, renderErr("clip %d: a segment ends at %s and starts at %s",
					index, fixed(end, 3), fixed(start, 3))
			}
			// A moment in an episode, not an arbitrary number. Anything
			// outside this is a broken plan, and it would reach ffmpeg as a
			// seek nobody can make.
			if start < 0 || end > MaxEpisodeSeconds {
				return Plan{}, nil, renderErr("clip %d: a segment lies outside the episode, "+
					"which cannot be longer than %d hours", index, MaxEpisodeSeconds/3600)
			}
			_, moved := seg["crop_x_auto"]
			segments = append(segments, Segment{Start: start, End: end, CropX: cropX, Moved: moved})
		}
		if len(segments) == 0 {
			continue
		}

		var spoken []Cue
		if list, ok := entry["words"].([]any); ok {
			for _, itemAny := range list {
				item, ok := itemAny.([]any)
				if !ok || len(item) != 3 {
					continue
				}
				low, ok1 := toFloat(item[0])
				high, ok2 := toFloat(item[1])
				if !ok1 || !ok2 {
					continue
				}
				text := Scrub(pyStr(item[2]), 100)
				if isFinite(low) && isFinite(high) && high >= low && text != "" {
					spoken = append(spoken, Cue{low, high, text})
				}
			}
		}

		fallback := fmt.Sprintf("%02d", index)
		idValue, present := entry["id"]
		idText := fallback
		if present {
			idText = pyStr(idValue)
		}
		slugText := ""
		if value, present := entry["slug"]; present {
			slugText = pyStr(value)
		}
		titleText := ""
		if value, present := entry["title"]; present {
			titleText = pyStr(value)
		}
		rejected, _ := entry["rejected"].(bool)
		var captionY *float64
		if value, present := entry["caption_y"]; present {
			if y, ok := toFloat(value); ok && isFinite(y) {
				snapped := SnapCaptionY(y)
				captionY = &snapped
			}
		}
		clips = append(clips, Clip{
			Rejected: rejected,
			CaptionY: captionY,
			ID:       SanitiseName(idText, fallback),
			Slug:     SanitiseName(slugText, ""),
			Title:    Scrub(titleText, 200),
			Segments: segments,
			Words:    spoken,
		})
	}

	// A clip is addressed by its id: every edit the app makes names one.
	// Two clips with the same id would mean edits landing on whichever comes
	// first, quietly, so a plan like that is not a plan.
	named := map[string]int{}
	for i, clip := range clips {
		if first, dup := named[clip.ID]; dup {
			return Plan{}, nil, renderErr("clips %d and %d are both called %s, "+
				"and an edit names the clip it belongs to", first, i+1, pyRepr(clip.ID))
		}
		named[clip.ID] = i + 1
	}

	// Two clips resolving to the same basename would silently overwrite each
	// other's output, and sanitising makes collisions more likely rather than
	// less. "a/b" and "a-b" both land on "a-b".
	seen := map[string]int{}
	for i, clip := range clips {
		if first, dup := seen[clip.Basename()]; dup {
			return Plan{}, nil, renderErr("clips %d and %d both resolve to the name %s, "+
				"which would overwrite one output", first, i+1, pyRepr(clip.Basename()))
		}
		seen[clip.Basename()] = i + 1
	}
	return Plan{Raw: raw}, clips, nil
}

// --------------------------------------------------------------------------
// geometry
// --------------------------------------------------------------------------

// CropWindow is the source-pixel rectangle that fills the output aspect
// ratio. Height is used in full whenever the source is wider than the target
// ratio, which is always the case cutting 9:16 out of 16:9.
func CropWindow(source SourceInfo, outW, outH int) (int, int) {
	ratio := float64(outW) / float64(outH)
	cropW := pyround(float64(source.Height) * ratio)
	cropH := source.Height
	if cropW > source.Width {
		cropW = source.Width
		cropH = pyround(float64(source.Width) / ratio)
	}
	// yuv420p needs even dimensions.
	cropW -= cropW % 2
	cropH -= cropH % 2
	return cropW, cropH
}

// ClampCropX keeps a crop inside the frame and on an even pixel.
func ClampCropX(cropX *int, cropW, sourceW int) int {
	x := (sourceW - cropW) / 2
	if cropX != nil {
		x = *cropX
	}
	x = max(0, min(x, sourceW-cropW))
	return x - x%2
}

func intPtr(n int) *int { return &n }

func itoa(n int) string { return strconv.Itoa(n) }
