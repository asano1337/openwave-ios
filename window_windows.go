//go:build windows

package main

import (
	"context"
	webview "github.com/jchv/go-webview2"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

func runApp(address string, srv *http.Server) {
	if os.Getenv("OPENWAVE_NO_OPEN") == "1" {
		srv.ListenAndServe()
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	user32 := syscall.NewLazyDLL("user32.dll")
	getStyle := user32.NewProc("GetWindowLongPtrW")
	setStyle := user32.NewProc("SetWindowLongPtrW")
	setPos := user32.NewProc("SetWindowPos")
	show := user32.NewProc("ShowWindow")
	isZoomed := user32.NewProc("IsZoomed")
	post := user32.NewProc("PostMessageW")
	release := user32.NewProc("ReleaseCapture")
	w := webview.NewWithOptions(webview.WebViewOptions{DataPath: filepath.Join(os.Getenv("LocalAppData"), "OpenWave", "webview3"), AutoFocus: true, WindowOptions: webview.WindowOptions{Title: "OpenWave", Width: 1280, Height: 850, Center: true, IconId: 2}})
	if w == nil {
		openWindow(address)
		for time.Now().Unix()-last.Load() <= 180 {
			time.Sleep(30 * time.Second)
		}
		return
	}
	hwnd := uintptr(w.Window())
	style, _, _ := getStyle.Call(hwnd, uintptr(0xfffffff0))
	style &^= 0x00C00000
	setStyle.Call(hwnd, uintptr(0xfffffff0), style)
	setPos.Call(hwnd, 0, 0, 0, 0, 0, 0x0027)
	w.SetSize(800, 600, webview.HintMin)
	w.Init("window.NATIVE_WINDOW=true;")
	w.Bind("nativeMin", func() error { w.Dispatch(func() { show.Call(hwnd, 6) }); return nil })
	w.Bind("nativeMax", func() error {
		w.Dispatch(func() {
			z, _, _ := isZoomed.Call(hwnd)
			if z != 0 {
				show.Call(hwnd, 9)
			} else {
				show.Call(hwnd, 3)
			}
		})
		return nil
	})
	w.Bind("nativeClose", func() error { w.Dispatch(func() { post.Call(hwnd, 0x0010, 0, 0) }); return nil })
	w.Bind("nativeDrag", func() error { w.Dispatch(func() { release.Call(); post.Call(hwnd, 0x00A1, 2, 0) }); return nil })
	w.Navigate(address)
	w.Run()
	w.Destroy()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}
