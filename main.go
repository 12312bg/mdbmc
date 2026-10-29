package main

import (
	"context"
	"embed"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/12312bg/mdbmc/internal/app"
	"github.com/12312bg/mdbmc/internal/console"
	"github.com/12312bg/mdbmc/internal/update"
)

//go:embed all:frontend/dist
var assets embed.FS

// dataDir is %APPDATA%\mdbmc unless MDBMC_DATA_DIR overrides it.
func dataDir() (string, error) {
	if d := os.Getenv("MDBMC_DATA_DIR"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "mdbmc"), nil
}

func setupLog(dir string) {
	_ = os.MkdirAll(dir, 0o700)
	path := filepath.Join(dir, "mdbmc.log")
	if fi, err := os.Stat(path); err == nil && fi.Size() > 5<<20 {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	log.SetOutput(io.MultiWriter(f, os.Stderr))
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == console.Flag {
		console.RunWindow()
		return
	}
	if len(os.Args) > 2 && os.Args[1] == update.HelperFlag {
		update.RunHelper(os.Args[2])
		return
	}

	dir, err := dataDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	setupLog(dir)

	var wailsCtx context.Context
	launcher := console.ProcessLauncher
	if runtime.GOOS != "windows" {
		// Development builds: no WebView2 child windows, use the browser.
		launcher = func(req console.Request) error {
			wruntime.BrowserOpenURL(wailsCtx, req.Spec.URL)
			return nil
		}
	}

	a, err := app.New(dir, launcher)
	if err != nil {
		log.Fatalf("初始化失败: %v", err)
	}
	if runtime.GOOS == "windows" {
		// Installing an update exits the program; the update helper starts it again.
		a.SetQuit(func() { wruntime.Quit(wailsCtx) })
	}

	err = wails.Run(&options.App{
		Title:     "明点服务器管理系统",
		Width:     1400,
		Height:    880,
		MinWidth:  1100,
		MinHeight: 680,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 245, G: 247, B: 250, A: 1},
		OnStartup: func(ctx context.Context) {
			wailsCtx = ctx
			a.Startup(ctx, func(name string, data any) { wruntime.EventsEmit(ctx, name, data) })
		},
		OnShutdown: func(context.Context) { a.Shutdown() },
		Bind:       a.Bindings(),
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "c6f1d2a4-mdbmc-desktop",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				if wailsCtx != nil {
					wruntime.WindowUnminimise(wailsCtx)
					wruntime.Show(wailsCtx)
				}
			},
		},
		Windows: &windows.Options{
			WebviewUserDataPath: filepath.Join(dir, "webview"),
			Theme:               windows.SystemDefault,
		},
	})
	if err != nil {
		log.Println("Error:", err)
	}
}
