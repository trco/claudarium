package main

import (
	"flag"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/trco/claudarium/internal/config"
	"github.com/trco/claudarium/internal/server"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	addr := flag.String("addr", "127.0.0.1:8787", "listen address (keep on localhost — this edits real config)")
	scan := flag.String("scan", "", "extra dirs (comma-separated) to scan one level deep for repos with a .claude dir")
	dev := flag.Bool("dev", false, "read templates/static from disk (for air hot-reload) instead of the embedded copy")
	open := flag.Bool("open", false, "open the app in your default browser on start")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	for _, r := range strings.Split(*scan, ",") {
		if r = strings.TrimSpace(r); r != "" {
			config.ExtraRoots = append(config.ExtraRoots, r)
		}
	}

	app := server.New(*dev)
	url := "http://" + strings.Replace(*addr, "127.0.0.1", "localhost", 1)
	log.Printf("Starting server on %s", url)
	if *open {
		go func() { time.Sleep(700 * time.Millisecond); openBrowser(url) }()
	}
	log.Fatal(app.Listen(*addr))
}

// openBrowser launches the OS default browser at url (best-effort).
func openBrowser(url string) {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name, args = "open", []string{url}
	case "windows":
		name, args = "cmd", []string{"/c", "start", url}
	default:
		name, args = "xdg-open", []string{url}
	}
	_ = exec.Command(name, args...).Start()
}
