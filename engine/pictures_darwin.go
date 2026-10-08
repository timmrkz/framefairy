//go:build darwin

package engine

/*
#cgo LDFLAGS: -framework VideoToolbox -framework CoreMedia -framework CoreVideo -framework CoreFoundation
#include <stdlib.h>
#include <string.h>
#include <VideoToolbox/VideoToolbox.h>

// One decoder: a VideoToolbox session that puts out every frame scaled to
// width by height in 8-bit NV12, the scaling and the step from 10-bit
// colour done by VideoToolbox, on the graphics chip where there is one.
typedef struct {
	VTDecompressionSessionRef session;
	CMVideoFormatDescriptionRef format;
	int width;
	int height;
	// What the last decode put out: whether a frame came, its status.
	int came;
	OSStatus status;
} ffPictures;

// Called by VideoToolbox for every frame, before the decode that made it
// returns, since nothing asks it to decode later. The frame is copied out
// only when the caller passed somewhere to copy it to.
static void ffOutput(void *ref, void *frameRef, OSStatus status, VTDecodeInfoFlags flags,
		CVImageBufferRef image, CMTime pts, CMTime duration) {
	ffPictures *d = (ffPictures *)ref;
	d->status = status;
	if (status != noErr || image == NULL) return;
	d->came = 1;
	if (frameRef == NULL) return;
	// A decoder that would not scale is no use: the frame would be cut, not
	// made smaller. Said as a failure, so the preview takes ffmpeg's.
	if ((int)CVPixelBufferGetWidth(image) != d->width || (int)CVPixelBufferGetHeight(image) != d->height ||
			CVPixelBufferGetPlaneCount(image) != 2) {
		d->came = 0;
		d->status = -2;
		return;
	}
	uint8_t *dst = (uint8_t *)frameRef;
	if (CVPixelBufferLockBaseAddress(image, kCVPixelBufferLock_ReadOnly) != kCVReturnSuccess) {
		d->came = 0;
		return;
	}
	size_t w = (size_t)d->width;
	size_t h = (size_t)d->height;
	// The light, a byte a pixel, then the colour, two bytes for every two
	// by two pixels, each row as wide as the picture.
	for (size_t p = 0; p < 2; p++) {
		const uint8_t *src = CVPixelBufferGetBaseAddressOfPlane(image, p);
		size_t stride = CVPixelBufferGetBytesPerRowOfPlane(image, p);
		size_t rows = p == 0 ? h : h / 2;
		size_t have = CVPixelBufferGetHeightOfPlane(image, p);
		size_t wide = CVPixelBufferGetWidthOfPlane(image, p) * (p == 0 ? 1 : 2);
		size_t n = w < wide ? w : wide;
		for (size_t r = 0; r < rows && r < have; r++) {
			memcpy(dst + r * w, src + r * stride, n);
		}
		dst += w * h;
	}
	CVPixelBufferUnlockBaseAddress(image, kCVPixelBufferLock_ReadOnly);
}

static CFNumberRef ffNumber(int n) {
	return CFNumberCreate(NULL, kCFNumberIntType, &n);
}

// Opens a decoder for a track described by its avcC or hvcC box, the way
// the file says it, and the picture's own size. Answers a status other
// than noErr when the system has no decoder for it.
static OSStatus ffOpen(ffPictures *d, int hevc, const uint8_t *config, int configSize,
		int codedWidth, int codedHeight, int width, int height) {
	memset(d, 0, sizeof(*d));
	d->width = width;
	d->height = height;
	CFDataRef atom = CFDataCreate(NULL, config, configSize);
	const void *atomKey[] = {hevc ? CFSTR("hvcC") : CFSTR("avcC")};
	const void *atomValue[] = {atom};
	CFDictionaryRef atoms = CFDictionaryCreate(NULL, atomKey, atomValue, 1,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	const void *extKey[] = {kCMFormatDescriptionExtension_SampleDescriptionExtensionAtoms};
	const void *extValue[] = {atoms};
	CFDictionaryRef extensions = CFDictionaryCreate(NULL, extKey, extValue, 1,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	OSStatus st = CMVideoFormatDescriptionCreate(NULL,
		hevc ? kCMVideoCodecType_HEVC : kCMVideoCodecType_H264, codedWidth, codedHeight,
		extensions, &d->format);
	CFRelease(extensions);
	CFRelease(atoms);
	CFRelease(atom);
	if (st != noErr) return st;

	// Full range, which VideoToolbox converts to from whatever the file
	// is in. WebKit draws a frame made from a buffer as full range
	// whatever the frame says, so a picture in video range came out pale,
	// its black a grey of 17, see native.ts.
	CFNumberRef pixel = ffNumber(kCVPixelFormatType_420YpCbCr8BiPlanarFullRange);
	CFNumberRef w = ffNumber(width);
	CFNumberRef h = ffNumber(height);
	const void *imageKey[] = {kCVPixelBufferPixelFormatTypeKey, kCVPixelBufferWidthKey, kCVPixelBufferHeightKey};
	const void *imageValue[] = {pixel, w, h};
	CFDictionaryRef image = CFDictionaryCreate(NULL, imageKey, imageValue, 3,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	VTDecompressionOutputCallbackRecord out = {ffOutput, d};
	st = VTDecompressionSessionCreate(NULL, d->format, NULL, image, &out, &d->session);
	CFRelease(image);
	CFRelease(pixel);
	CFRelease(w);
	CFRelease(h);
	if (st != noErr) {
		CFRelease(d->format);
		d->format = NULL;
		d->session = NULL;
	}
	return st;
}

// Decodes one sample, the bytes as they lie in the file, and copies its
// frame to dst when dst is not NULL. Answers whether a frame came, or a
// status below zero when the decoder failed.
static int ffDecode(ffPictures *d, const uint8_t *data, int size, int64_t microseconds, uint8_t *dst) {
	void *copy = malloc((size_t)size);
	if (copy == NULL) return -1;
	memcpy(copy, data, (size_t)size);
	CMBlockBufferRef block = NULL;
	OSStatus st = CMBlockBufferCreateWithMemoryBlock(NULL, copy, (size_t)size, kCFAllocatorMalloc,
		NULL, 0, (size_t)size, 0, &block);
	if (st != noErr) {
		free(copy);
		return st < 0 ? st : -1;
	}
	CMSampleBufferRef sample = NULL;
	CMSampleTimingInfo timing = {kCMTimeInvalid, CMTimeMake(microseconds, 1000000), kCMTimeInvalid};
	size_t sizes[] = {(size_t)size};
	st = CMSampleBufferCreateReady(NULL, block, d->format, 1, 1, &timing, 1, sizes, &sample);
	CFRelease(block);
	if (st != noErr) return st < 0 ? st : -1;
	d->came = 0;
	d->status = noErr;
	VTDecodeInfoFlags info = 0;
	st = VTDecompressionSessionDecodeFrame(d->session, sample, 0, dst, &info);
	CFRelease(sample);
	if (st != noErr) return st < 0 ? st : -1;
	if (d->status != noErr) return d->status < 0 ? d->status : -1;
	return d->came;
}

static void ffClose(ffPictures *d) {
	if (d->session != NULL) {
		VTDecompressionSessionInvalidate(d->session);
		CFRelease(d->session);
		d->session = NULL;
	}
	if (d->format != NULL) {
		CFRelease(d->format);
		d->format = NULL;
	}
}
*/
import "C"

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"
)

type vtPictures struct {
	mu     sync.Mutex
	d      *C.ffPictures
	width  int
	height int
}

func openPictures(codec string, config []byte, codedWidth, codedHeight, width, height int) (Pictures, error) {
	hevc := 0
	switch codec {
	case "hvc1", "hev1":
		hevc = 1
	case "avc1", "avc3":
	default:
		return nil, ErrNoPictureDecoder
	}
	if len(config) == 0 || width < 2 || height < 2 || width%2 != 0 || height%2 != 0 {
		return nil, ErrNoPictureDecoder
	}
	d := (*C.ffPictures)(C.malloc(C.size_t(unsafe.Sizeof(C.ffPictures{}))))
	cfg := C.CBytes(config)
	defer C.free(cfg)
	st := C.ffOpen(d, C.int(hevc), (*C.uint8_t)(cfg), C.int(len(config)),
		C.int(codedWidth), C.int(codedHeight), C.int(width), C.int(height))
	if st != 0 {
		C.free(unsafe.Pointer(d))
		return nil, fmt.Errorf("%w: VideoToolbox said %d", ErrNoPictureDecoder, int(st))
	}
	p := &vtPictures{d: d, width: width, height: height}
	runtime.SetFinalizer(p, (*vtPictures).Close)
	return p, nil
}

func (p *vtPictures) Size() (int, int) { return p.width, p.height }

func (p *vtPictures) Decode(data []byte, microseconds int64, keep bool) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.d == nil {
		return nil, ErrNoPictureDecoder
	}
	if len(data) == 0 {
		return nil, nil
	}
	src := C.CBytes(data)
	defer C.free(src)
	var dst unsafe.Pointer
	if keep {
		dst = C.malloc(C.size_t(p.width * p.height * 3 / 2))
		defer C.free(dst)
	}
	got := C.ffDecode(p.d, (*C.uint8_t)(src), C.int(len(data)), C.int64_t(microseconds), (*C.uint8_t)(dst))
	// A frame not kept may fail on its own: decoding that starts at a key
	// frame of an open group cannot make the frames shown before it, which
	// refer to frames before the key frame. They are never drawn.
	if got < 0 && !keep {
		return nil, nil
	}
	if got < 0 {
		return nil, fmt.Errorf("VideoToolbox could not decode a frame, it said %d", int(got))
	}
	if got == 0 || !keep {
		return nil, nil
	}
	return C.GoBytes(dst, C.int(p.width*p.height*3/2)), nil
}

func (p *vtPictures) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.d == nil {
		return
	}
	C.ffClose(p.d)
	C.free(unsafe.Pointer(p.d))
	p.d = nil
}
