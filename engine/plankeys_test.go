package engine

import (
	"reflect"
	"strings"
	"testing"
)

// The structs that write a new plan and the names an edit reaches it by
// are one set of names. A tag can only be written out, so this test is
// what keeps a renamed key from being written one way and read another.
func TestAPlanIsWrittenAndReadByTheSameNames(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		of     any
		fields map[string]string
	}{
		{PlanFile{}, map[string]string{"PlanID": keyPlanID, "PlannedWith": keyPlannedWith, "Clips": keyClips}},
		{PlanClip{}, map[string]string{"ID": keyID, "Slug": keySlug, "Title": keyTitle, "Reason": keyReason,
			"Keep": keyKeep, "Segments": keySegments}},
		{PlanSegment{}, map[string]string{"Start": keyStart, "End": keyEnd, "CropX": keyCropX}},
		{PlannedWith{}, map[string]string{"From": keyFrom, "To": keyTo}},
	} {
		typ := reflect.TypeOf(c.of)
		for field, key := range c.fields {
			f, ok := typ.FieldByName(field)
			if !ok {
				t.Fatalf("%s has no field %s", typ.Name(), field)
			}
			tag, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if tag != key {
				t.Errorf("%s.%s is written as %q and read as %q", typ.Name(), field, tag, key)
			}
		}
	}
}
