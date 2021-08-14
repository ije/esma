package server

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/ije/rex"
)

type App struct {
	lock      sync.RWMutex
	wd        string
	dev       bool
	indexHTML *FileContent
	builds    map[string]*FileContent
}

func (app *App) Build(filename string) (out *FileContent, err error) {
	if app.builds == nil {
		app.builds = map[string]*FileContent{}
	}

	app.lock.RLock()
	out, ok := app.builds[filename]
	app.lock.RUnlock()
	if ok {
		return
	}

	fi, err := os.Lstat(filename)
	if err != nil {
		return
	}

	if fi.IsDir() {
		return nil, errors.New("can't build a directory")
	}

	minify := !app.dev
	options := api.BuildOptions{
		Outdir:            "/esbuild",
		Write:             false,
		Bundle:            false,
		Target:            api.ES2020,
		Format:            api.FormatESModule,
		Platform:          api.PlatformBrowser,
		MinifyWhitespace:  minify,
		MinifyIdentifiers: minify,
		MinifySyntax:      minify,
		Plugins:           []api.Plugin{},
		EntryPoints:       []string{filename},
	}
	result := api.Build(options)
	if l := len(result.Errors); l > 0 {
		texts := make([]string, l)
		for i, e := range result.Errors {
			texts[i] = e.Text
		}
		return nil, errors.New(strings.Join(texts, "\n"))
	}

	if len(result.OutputFiles) > 0 {
		output := result.OutputFiles[0]
		out = &FileContent{
			Content: output.Contents,
			Modtime: fi.ModTime(),
		}
		app.lock.RLock()
		app.builds[filename] = out
		app.lock.RUnlock()
		return
	}

	return nil, fmt.Errorf("Unknown error")
}

func (app *App) Watch() {
	log.Print("Watching file changes...")
}

func (app *App) Handle() rex.Handle {
	app.createIndexHtml()

	return func(ctx *rex.Context) interface{} {
		filepath := path.Join(app.wd, ctx.R.URL.Path)
		if fileExists(filepath) {
			for _, ext := range defaultModuleExts {
				if strings.HasSuffix(filepath, ext) {
					build, err := app.Build(filepath)
					if err != nil {
						return err
					}
					return rex.Content("index.js", build.Modtime, bytes.NewReader(build.Content))
				}
			}
			return rex.File(filepath)
		}
		return rex.Content("index.html", app.indexHTML.Modtime, bytes.NewReader(app.indexHTML.Content))
	}
}

func (app *App) createIndexHtml() {
	p := path.Join(app.wd, "index.html")
	fi, err := os.Lstat(p)
	if err == nil && !fi.IsDir() {
		data, err := ioutil.ReadFile(p)
		if err == nil {
			app.indexHTML = &FileContent{
				Modtime: fi.ModTime(),
				Content: data,
			}
		}
	} else {
		for _, name := range []string{"app", "main", "index"} {
			for _, ext := range defaultModuleExts {
				filename := name + ext
				if fileExists(path.Join(app.wd, filename)) {
					app.indexHTML = &FileContent{
						Modtime: time.Now(),
						Content: []byte(fmt.Sprintf(defaultIndexHtml, "./"+filename)),
					}
					return
				}
			}
		}
	}
}
