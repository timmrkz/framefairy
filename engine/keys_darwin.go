//go:build darwin

package engine

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <stdlib.h>
#include <string.h>
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>

// The item a key is kept in: a generic password, found by its service and
// its account.
static CFMutableDictionaryRef ff_item(const char *service, const char *account) {
	CFMutableDictionaryRef q = CFDictionaryCreateMutable(NULL, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFStringRef s = CFStringCreateWithCString(NULL, service, kCFStringEncodingUTF8);
	CFStringRef a = CFStringCreateWithCString(NULL, account, kCFStringEncodingUTF8);
	CFDictionarySetValue(q, kSecClass, kSecClassGenericPassword);
	CFDictionarySetValue(q, kSecAttrService, s);
	CFDictionarySetValue(q, kSecAttrAccount, a);
	CFRelease(s);
	CFRelease(a);
	return q;
}

// Whether the item is there. Only its attributes are asked for, never the
// secret, so the access list is not consulted and nothing is put on screen.
static int ff_has(const char *service, const char *account) {
	CFMutableDictionaryRef q = ff_item(service, account);
	CFDictionarySetValue(q, kSecReturnAttributes, kCFBooleanTrue);
	CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
	CFTypeRef found = NULL;
	OSStatus status = SecItemCopyMatching(q, &found);
	if (found != NULL) CFRelease(found);
	CFRelease(q);
	return (int)status;
}

// The secret, copied into memory the caller frees.
static int ff_get(const char *service, const char *account, char **out, long *length) {
	CFMutableDictionaryRef q = ff_item(service, account);
	CFDictionarySetValue(q, kSecReturnData, kCFBooleanTrue);
	CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
	CFTypeRef found = NULL;
	OSStatus status = SecItemCopyMatching(q, &found);
	CFRelease(q);
	if (status != errSecSuccess) return (int)status;
	if (found == NULL || CFGetTypeID(found) != CFDataGetTypeID()) {
		if (found != NULL) CFRelease(found);
		return (int)errSecItemNotFound;
	}
	CFDataRef data = (CFDataRef)found;
	*length = CFDataGetLength(data);
	*out = malloc(*length + 1);
	memcpy(*out, CFDataGetBytePtr(data), *length);
	CFRelease(found);
	return 0;
}

// The item's comment, which holds the key in short, copied into out. It
// is an attribute, so like ff_has it is read without the access list.
static int ff_hint(const char *service, const char *account, char *out, long size) {
	CFMutableDictionaryRef q = ff_item(service, account);
	CFDictionarySetValue(q, kSecReturnAttributes, kCFBooleanTrue);
	CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
	CFTypeRef found = NULL;
	OSStatus status = SecItemCopyMatching(q, &found);
	CFRelease(q);
	out[0] = 0;
	if (status != errSecSuccess) return (int)status;
	if (found != NULL && CFGetTypeID(found) == CFDictionaryGetTypeID()) {
		CFTypeRef comment = CFDictionaryGetValue((CFDictionaryRef)found, kSecAttrComment);
		if (comment != NULL && CFGetTypeID(comment) == CFStringGetTypeID()) {
			if (!CFStringGetCString((CFStringRef)comment, out, size, kCFStringEncodingUTF8)) {
				out[0] = 0;
			}
		}
	}
	if (found != NULL) CFRelease(found);
	return 0;
}

// Keeps a secret, with the key in short as the item's comment. An item
// that is there has its secret replaced, and one that is not is added,
// with the access list macOS gives an item made by an app: that app, and
// nothing else without asking.
static int ff_set(const char *service, const char *account, const char *secret, long length,
		const char *hint) {
	CFMutableDictionaryRef q = ff_item(service, account);
	CFDataRef data = CFDataCreate(NULL, (const UInt8 *)secret, length);
	CFStringRef comment = CFStringCreateWithCString(NULL, hint, kCFStringEncodingUTF8);
	CFMutableDictionaryRef change = CFDictionaryCreateMutable(NULL, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFDictionarySetValue(change, kSecValueData, data);
	CFDictionarySetValue(change, kSecAttrComment, comment);
	OSStatus status = SecItemUpdate(q, change);
	if (status == errSecItemNotFound) {
		CFDictionarySetValue(q, kSecValueData, data);
		CFDictionarySetValue(q, kSecAttrComment, comment);
		status = SecItemAdd(q, NULL);
	}
	CFRelease(change);
	CFRelease(comment);
	CFRelease(data);
	CFRelease(q);
	return (int)status;
}

static int ff_remove(const char *service, const char *account) {
	CFMutableDictionaryRef q = ff_item(service, account);
	OSStatus status = SecItemDelete(q);
	CFRelease(q);
	return (int)status;
}

static void ff_forget(char *secret, long length) {
	if (secret == NULL) return;
	memset(secret, 0, length);
	free(secret);
}
*/
import "C"

import (
	"context"
	"fmt"
	"unsafe"
)

// keychainKeys keeps keys in the keychain through the Security framework,
// so the key never passes through a command line and the item belongs to
// the app.
type keychainKeys struct{}

func systemKeys() keyStore { return keychainKeys{} }

// legacyKeys reads the items the first keys were kept in, which the
// security command made, with the same command. Reading them that way
// puts nothing secret on its command line: the secret comes back on its
// output. They are never written again.
func legacyKeys() keyStore { return securityKeys{} }

func withNames(item string, do func(service, account *C.char)) {
	service, account := C.CString(item), C.CString(keyAccount)
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))
	do(service, account)
}

func (keychainKeys) has(item string) bool {
	found := false
	withNames(item, func(service, account *C.char) {
		found = C.ff_has(service, account) == 0
	})
	return found
}

// hintRoom is the most a hint is read back with: a licence's description,
// its ID and a name of up to licence.MaxName bytes, fits with room left.
const hintRoom = 256

func (keychainKeys) hint(item string) string {
	hint := ""
	withNames(item, func(service, account *C.char) {
		buf := (*C.char)(C.malloc(hintRoom))
		defer C.free(unsafe.Pointer(buf))
		if C.ff_hint(service, account, buf, hintRoom) == 0 {
			hint = C.GoString(buf)
		}
	})
	return hint
}

func (keychainKeys) get(_ context.Context, item string) (string, bool) {
	secret, ok := "", false
	withNames(item, func(service, account *C.char) {
		var out *C.char
		var length C.long
		if C.ff_get(service, account, &out, &length) != 0 {
			return
		}
		defer C.ff_forget(out, length)
		secret, ok = C.GoStringN(out, C.int(length)), true
	})
	return secret, ok
}

func (keychainKeys) set(item, secret, hinted string) error {
	var status C.int
	withNames(item, func(service, account *C.char) {
		value := C.CString(secret)
		defer C.ff_forget(value, C.long(len(secret)))
		hint := C.CString(hinted)
		defer C.free(unsafe.Pointer(hint))
		status = C.ff_set(service, account, value, C.long(len(secret)), hint)
	})
	if status != 0 {
		return fmt.Errorf("status %d", int(status))
	}
	return nil
}

func (keychainKeys) remove(item string) {
	withNames(item, func(service, account *C.char) {
		C.ff_remove(service, account)
	})
}

// securityKeys are the old items, through the security command.
type securityKeys struct{}

func (securityKeys) has(item string) bool {
	// Without -w only the attributes are printed, which the access list
	// does not guard, so this never asks anybody anything.
	res, _ := keychain(context.Background(), "find-generic-password", "-a", keyAccount, "-s", item)
	return res.Code == 0
}

// The old items were kept without a key in short.
func (securityKeys) hint(string) string { return "" }

func (securityKeys) get(ctx context.Context, item string) (string, bool) {
	res, _ := keychain(ctx, "find-generic-password", "-a", keyAccount, "-s", item, "-w")
	if res.Code != 0 || strip(res.Stdout) == "" {
		return "", false
	}
	return strip(res.Stdout), true
}

func (securityKeys) set(string, string, string) error { return errNoKeychain }

func (securityKeys) remove(item string) {
	keychain(context.Background(), "delete-generic-password", "-a", keyAccount, "-s", item)
}
