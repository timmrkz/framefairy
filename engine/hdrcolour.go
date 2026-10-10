package engine

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"framefairy/internal/framewire"
)

// The captions of an HDR short are drawn at the reference white for
// graphics in HDR, BT.2408's 203 nits, so a caption looks on an HDR screen
// the way its colour was picked, and not glaring. A colour picked in the
// app is an sRGB colour, as the app shows it, and in the short it becomes
// the same light in the short's own curve and colours: its sRGB value
// turned into light, white made 203 nits, BT.709's colours turned into
// BT.2020's, and the light written in PQ or HLG. The video preview does
// the same the other way round, see frontend/src/lib/frames/light.ts.
//
// libass then draws the colour into the frame with the frame's own
// matrix, BT.2020's, as the caption file says None for its matrix, see
// assHeader.

// hdrWhite is BT.2408's reference white, in nits.
const hdrWhite = 203

// hlgPeak is the display HLG is written for, BT.2100's reference, at
// which HLG's reference white, 75 percent, is 203 nits.
const hlgPeak = 1000

// from709 turns BT.709's colours into BT.2020's, in linear light, row by
// row, BT.2087.
var from709 = [3][3]float64{
	{0.6274, 0.3293, 0.0433},
	{0.0691, 0.9195, 0.0114},
	{0.0164, 0.0880, 0.8956},
}

// inLight is the style with its colours as they are written into a short
// of this light, unchanged for standard video.
func (s Style) inLight(light framewire.Light) Style {
	if light == framewire.SDR {
		return s
	}
	s.light = light
	s.Primary = hdrColour(s.Primary, light)
	s.OutlineColour = hdrColour(s.OutlineColour, light)
	s.BackColour = hdrColour(s.BackColour, light)
	s.HighlightColour = hdrColour(s.HighlightColour, light)
	return s
}

// hdrColour is an ASS colour, &HAABBGGRR or &HBBGGRR&, with its colour
// written in light's curve and BT.2020's colours and its alpha and its
// form kept. Anything that is not an ASS colour is given back as it is.
func hdrColour(colour string, light framewire.Light) string {
	raw := strings.TrimSuffix(strings.TrimPrefix(colour, "&H"), "&")
	if !strings.HasPrefix(colour, "&H") || len(raw) < 6 || len(raw) > 8 {
		return colour
	}
	bgr := raw[len(raw)-6:]
	v, err := strconv.ParseUint(bgr, 16, 32)
	if err != nil {
		return colour
	}
	r, g, b := hdrSignal(light, float64(v&0xFF)/255, float64(v>>8&0xFF)/255, float64(v>>16&0xFF)/255)
	byte8 := func(x float64) uint64 { return uint64(math.Round(255 * math.Min(math.Max(x, 0), 1))) }
	written := fmt.Sprintf("%02X%02X%02X", byte8(b), byte8(g), byte8(r))
	return "&H" + raw[:len(raw)-6] + written + colour[len("&H")+len(raw):]
}

// hdrSignal is an sRGB colour of 0 to 1 as light's signal of 0 to 1 in
// BT.2020's colours, with white at 203 nits.
func hdrSignal(light framewire.Light, r, g, b float64) (float64, float64, float64) {
	lin := [3]float64{srgbLight(r), srgbLight(g), srgbLight(b)}
	var nits [3]float64
	for i, row := range from709 {
		nits[i] = hdrWhite * (row[0]*lin[0] + row[1]*lin[1] + row[2]*lin[2])
	}
	if light == framewire.PQ {
		return pqSignal(nits[0]), pqSignal(nits[1]), pqSignal(nits[2])
	}
	// HLG's display light back to the scene's, BT.2100's inverse of its
	// OOTF with a system gamma of 1.2, and then its OETF.
	y := (0.2627*nits[0] + 0.6780*nits[1] + 0.0593*nits[2]) / hlgPeak
	if y <= 0 {
		return 0, 0, 0
	}
	gain := math.Pow(y, (1-1.2)/1.2) / hlgPeak
	return hlgSignal(nits[0] * gain), hlgSignal(nits[1] * gain), hlgSignal(nits[2] * gain)
}

// srgbLight is sRGB's curve, a signal of 0 to 1 to light of 0 to 1.
func srgbLight(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

// pqSignal is PQ's inverse curve, nits to a signal of 0 to 1.
func pqSignal(nits float64) float64 {
	const m1, m2 = 0.1593017578125, 78.84375
	const c1, c2, c3 = 0.8359375, 18.8515625, 18.6875
	y := math.Pow(math.Max(nits, 0)/10000, m1)
	return math.Pow((c1+c2*y)/(1+c3*y), m2)
}

// hlgSignal is HLG's OETF, scene light of 0 to 1 to a signal of 0 to 1.
func hlgSignal(e float64) float64 {
	const a, b, c = 0.17883277, 0.28466892, 0.55991073
	if e <= 1.0/12 {
		return math.Sqrt(3 * math.Max(e, 0))
	}
	return a*math.Log(12*e-b) + c
}
