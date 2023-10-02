package main

import (
	"archive/zip"
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"strings"
	"time"

	ui "github.com/bcicen/termui"
	"github.com/nsf/termbox-go"
)

var (
	wmap        = NewWidgetMap()
	grid        = NewGrid()
	ctx         = context.Background()
	paused      bool
	filter      string
	routines    Routines
	lastRefresh time.Time
)

// parse command line arguments
var (
	helpFlag     = flag.Bool("h", false, "display this help dialog")
	hostFlag     = flag.String("host", "localhost:1234", "target host")
	selfFlag     = flag.Bool("self", false, "monitor grmon itself")
	endpointFlag = flag.String("endpoint", "/debug/pprof", "target path")
	intervalFlag = flag.Int("i", 5, "time in seconds between refresh")
)

var inputBuffer *bytes.Buffer

func Refresh() {
	var err error
	routines, err = poll()

	if err == nil {
		// update widget data
		for _, r := range routines {
			w := wmap.MustGet(r.Num)
			w.SetState(r.State)
			w.desc.Text = r.Trace[0]

			r.Trace[0] = r.State
			w.SetTrace(r.Trace)
		}
		lastRefresh = time.Now()
		RebuildRows()
	}
}

func RebuildRows() {
	routines.Sort()
	grid.Clear()

	for _, r := range routines {
		w := wmap.MustGet(r.Num)

		if filter == "" {
			grid.AddRow(w)
			continue
		}

		for _, l := range w.trace.Items {
			if strings.Contains(l, filter) {
				grid.AddRow(w)
				break
			}
		}
	}

	Render()
}

func Render() {
	grid.footer.Update()
	ui.Clear()
	ui.Render(grid)
}

func Display() bool {
	var next func()

	rctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if *intervalFlag > 0 && !paused {
		go func() {
			for {
				select {
				case <-rctx.Done():
					return
				case <-time.After(500 * time.Millisecond):
					if int(time.Since(lastRefresh).Seconds()) >= *intervalFlag {
						Refresh()
					}
				}
			}
		}()
	}

	ui.Handle("/sys/wnd/resize", func(ui.Event) {
		grid.Align()
		Render()
	})

	HandleKeys("up", func() {
		if grid.CursorUp() {
			Render()
		}
	})

	HandleKeys("down", func() {
		if grid.CursorDown() {
			Render()
		}
	})

	HandleKeys("enter", func() {
		if paused {
			grid.rows[grid.cursorPos].ToggleShowTrace()
			Render()
		}
	})

	ui.Handle("/sys/kbd/f", func(ui.Event) {
		next = FilterDialog
		ui.StopLoop()
	})

	ui.Handle("/sys/kbd/p", func(ui.Event) {
		next = func() { paused = paused != true }
		ui.StopLoop()
	})

	ui.Handle("/sys/kbd/r", func(ui.Event) {
		Refresh()
	})

	ui.Handle("/sys/kbd/s", func(ui.Event) {
		if sortKey == "num" {
			sortKey = "state"
		} else {
			sortKey = "num"
		}
		RebuildRows()
	})

	ui.Handle("/sys/kbd/t", func(ui.Event) {
		if paused {
			next = TraceDialog
			ui.StopLoop()
		}
	})

	HandleKeys("exit", func() {
		ui.StopLoop()
	})

	HandleKeys("help", func() {
		next = HelpDialog
		ui.StopLoop()
	})

	grid.Align()
	Render()
	ui.Loop()

	cancel()
	if next != nil {
		next()
		return false
	}
	return true
}

func main() {
	flag.Parse()

	if args := flag.Args(); len(args) > 0 {
		for _, arg := range args {
			inputBuffer = bytes.NewBuffer(nil)
			err := filepath.Walk(arg, func(path string, info os.FileInfo, err error) error {
				if info.IsDir() {
					return nil
				}
				f, err := os.Open(info.Name())
				if err != nil {
					return err
				}
				defer f.Close()
				if strings.HasSuffix(info.Name(), ".zip") {
					zr, err := zip.NewReader(f, info.Size())
					if err != nil {
						return err
					}
					for _, zf := range zr.File {
						if strings.HasSuffix(zf.Name, "debug=2.txt") {
							f2, err := zf.Open()
							if err != nil {
								return err
							}
							_, err = io.Copy(inputBuffer, f2)
							inputBuffer.WriteString("\n")
							f2.Close()
							if err != nil {
								return err
							}
						}
					}
					return nil
				}
				_, err = io.Copy(inputBuffer, f)
				inputBuffer.WriteString("\n")
				return err
			})
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		}
		paused = true
	}
	if *helpFlag {
		printHelp()
		os.Exit(0)
	}

	if *selfFlag {
		go http.ListenAndServe("localhost:1234", nil)
	}

	if *intervalFlag == 0 {
		paused = true
	}

	if err := ui.Init(); err != nil {
		panic(err)
	}
	defer ui.Close()
	termbox.SetOutputMode(termbox.Output256)

	Refresh()

	var quit bool
	for !quit {
		quit = Display()
	}
}

var helpMsg = `grmon - goroutine monitor

usage: grmon [options]

options:
`

func printHelp() {
	fmt.Println(helpMsg)
	flag.PrintDefaults()
}
