//go:build windows

package console

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"github.com/wailsapp/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

// Minimal COM bindings for WebView2's ServerCertificateErrorDetected event
// (ICoreWebView2_14). go-webview2's generated pkg/webview2 cannot be used:
// it panics while initializing on amd64, and its vtables leave out
// inherited methods.

var (
	iidIUnknown         = windows.GUID{Data1: 0x00000000, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidCoreWebView2_14  = windows.GUID{Data1: 0x6daa4f10, Data2: 0x4a90, Data3: 0x4753, Data4: [8]byte{0x88, 0x98, 0x77, 0xc5, 0xdf, 0x53, 0x41, 0x65}}
	iidCertErrorHandler = windows.GUID{Data1: 0x969b3a26, Data2: 0xd85e, Data3: 0x4795, Data4: [8]byte{0x81, 0x99, 0xfe, 0xf5, 0x73, 0x44, 0xda, 0x22}}
)

// Vtable slots, from WebView2.h (SDK 1.0.2903.40).
const (
	slotQueryInterface = 0
	slotRelease        = 2
	// ICoreWebView2_14, after the methods inherited from ICoreWebView2..._13.
	slotAddServerCertificateErrorDetected = 106
	// ICoreWebView2ServerCertificateErrorDetectedEventArgs
	slotGetRequestURI        = 4
	slotGetServerCertificate = 5
	slotPutAction            = 7
	// ICoreWebView2Certificate
	slotToPemEncoding = 9
	// ICoreWebView2NavigationCompletedEventArgs
	slotGetIsSuccess      = 3
	slotGetWebErrorStatus = 4
)

// COREWEBVIEW2_SERVER_CERTIFICATE_ERROR_ACTION
const (
	certActionAlwaysAllow = 0
	certActionCancel      = 1
)

const eNoInterface = 0x80004002

// comObject is a COM interface pointer as seen from Go.
type comObject struct {
	vtbl *[512]uintptr
}

// certErrorHandler implements
// ICoreWebView2ServerCertificateErrorDetectedEventHandler. It allows a
// certificate error only for the certificates whose SHA-256 fingerprints it
// was given; everything else is refused.
type certErrorHandler struct {
	vtbl    *[4]uintptr
	allowed []string
	// decided is told about every decision.
	decided func(allowed bool, uri, fingerprint string)
}

// The handler and its vtable are handed to native code and must stay
// reachable for the lifetime of the window.
var (
	handlerVtbl [4]uintptr
	handler     *certErrorHandler
)

func handlerQueryInterface(this *certErrorHandler, riid *windows.GUID, ppv *uintptr) uintptr {
	if *riid == iidIUnknown || *riid == iidCertErrorHandler {
		*ppv = uintptr(unsafe.Pointer(this))
		return 0
	}
	*ppv = 0
	return eNoInterface
}

// The handler is never freed, so reference counting is a no-op.
func handlerAddRef(*certErrorHandler) uintptr  { return 1 }
func handlerRelease(*certErrorHandler) uintptr { return 1 }

func handlerInvoke(this *certErrorHandler, _, args *comObject) uintptr {
	action := uintptr(certActionCancel)
	fp, err := serverFingerprint(args)
	if err != nil {
		fp = "（读取证书失败: " + err.Error() + "）"
	} else {
		for _, a := range this.allowed {
			if a != "" && strings.EqualFold(fp, a) {
				action = certActionAlwaysAllow
			}
		}
	}
	_, _, _ = syscall.SyscallN(args.vtbl[slotPutAction], uintptr(unsafe.Pointer(args)), action)
	if this.decided != nil {
		this.decided(action == certActionAlwaysAllow, comString(args, slotGetRequestURI), fp)
	}
	return 0
}

// comString calls a getter that returns a CoTaskMemAlloc'ed string.
func comString(obj *comObject, slot int) string {
	var text *uint16
	hr, _, _ := syscall.SyscallN(obj.vtbl[slot], uintptr(unsafe.Pointer(obj)), uintptr(unsafe.Pointer(&text)))
	if hr != 0 || text == nil {
		return ""
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(text))
	return windows.UTF16PtrToString(text)
}

// navigationResult reads ICoreWebView2NavigationCompletedEventArgs.
func navigationResult(args *edge.ICoreWebView2NavigationCompletedEventArgs) (ok bool, status int32) {
	obj := (*comObject)(unsafe.Pointer(args))
	var success int32
	_, _, _ = syscall.SyscallN(obj.vtbl[slotGetIsSuccess], uintptr(unsafe.Pointer(obj)), uintptr(unsafe.Pointer(&success)))
	_, _, _ = syscall.SyscallN(obj.vtbl[slotGetWebErrorStatus], uintptr(unsafe.Pointer(obj)), uintptr(unsafe.Pointer(&status)))
	return success != 0, status
}

// serverFingerprint returns the hex SHA-256 of the certificate that caused
// the error, the same form as bmc.Fingerprint.
func serverFingerprint(args *comObject) (string, error) {
	var cert *comObject
	hr, _, _ := syscall.SyscallN(args.vtbl[slotGetServerCertificate], uintptr(unsafe.Pointer(args)), uintptr(unsafe.Pointer(&cert)))
	if hr != 0 || cert == nil {
		return "", fmt.Errorf("get_ServerCertificate: %#x", hr)
	}
	defer syscall.SyscallN(cert.vtbl[slotRelease], uintptr(unsafe.Pointer(cert)))

	pemText := comString(cert, slotToPemEncoding)
	if pemText == "" {
		return "", errors.New("ToPemEncoding failed")
	}
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return "", errors.New("invalid PEM")
	}
	sum := sha256.Sum256(block.Bytes)
	return hex.EncodeToString(sum[:]), nil
}

// installCertPinning registers the certificate error handler on the
// window's WebView2. Certificates with the allowed fingerprints are
// accepted, all others refused; decided hears about each decision.
func installCertPinning(c *edge.Chromium, allowed []string, decided func(allowed bool, uri, fingerprint string)) {
	edgeView, err := c.GetController().GetCoreWebView2()
	if err != nil || edgeView == nil {
		fail("无法获取 WebView2 实例: %v", err)
	}
	view := (*comObject)(unsafe.Pointer(edgeView))
	var view14 *comObject
	hr, _, _ := syscall.SyscallN(view.vtbl[slotQueryInterface], uintptr(unsafe.Pointer(view)),
		uintptr(unsafe.Pointer(&iidCoreWebView2_14)), uintptr(unsafe.Pointer(&view14)))
	if hr != 0 || view14 == nil {
		fail("WebView2 运行时版本过旧，无法校验 BMC 证书。请更新 Microsoft Edge WebView2 Runtime。")
	}

	handlerVtbl = [4]uintptr{
		windows.NewCallback(handlerQueryInterface),
		windows.NewCallback(handlerAddRef),
		windows.NewCallback(handlerRelease),
		windows.NewCallback(handlerInvoke),
	}
	handler = &certErrorHandler{vtbl: &handlerVtbl, allowed: allowed, decided: decided}
	var token int64 // EventRegistrationToken
	hr, _, _ = syscall.SyscallN(view14.vtbl[slotAddServerCertificateErrorDetected], uintptr(unsafe.Pointer(view14)),
		uintptr(unsafe.Pointer(handler)), uintptr(unsafe.Pointer(&token)))
	if hr != 0 {
		fail("注册证书校验失败: %#x", hr)
	}
}
