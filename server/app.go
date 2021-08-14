package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/ije/gox/utils"
	"github.com/ije/rex"
)

type App struct {
	lock      sync.RWMutex
	wd        string
	dev       bool
	indexHTML *FileContent
	builds    map[string]*ESBulidRecord
}

func (app *App) Build(filename string) (out FileContent, err error) {
	if app.builds == nil {
		app.builds = map[string]*ESBulidRecord{}
	}

	app.lock.RLock()
	record, ok := app.builds[filename]
	app.lock.RUnlock()
	if ok {
		out = record.FileContent
		return
	}

	fi, err := os.Lstat(filename)
	if err != nil {
		return
	}

	if fi.IsDir() {
		err = errors.New("can't build a directory")
		return
	}

	esmaPlugin := api.Plugin{
		Name: "esm-resolver",
		Setup: func(plugin api.PluginBuild) {
			plugin.OnResolve(
				api.OnResolveOptions{Filter: ".*"},
				func(args api.OnResolveArgs) (api.OnResolveResult, error) {
					if args.Path == filename || (strings.HasSuffix(filename, ".css") && strings.HasSuffix(args.Path, ".css")) {
						return api.OnResolveResult{}, nil
					}
					path, qs := utils.SplitByFirstByte(args.Path, '?')
					if strings.HasSuffix(path, ".css") {
						path = path + "?module"
						if qs != "" {
							path += "&" + qs
						}
						return api.OnResolveResult{Path: path, External: true}, nil
					}
					return api.OnResolveResult{External: true}, nil
				},
			)
		},
	}

	minify := !app.dev
	options := api.BuildOptions{
		Outdir:            "/esbuild",
		Write:             false,
		Bundle:            true,
		Target:            api.ES2020,
		Format:            api.FormatESModule,
		Platform:          api.PlatformBrowser,
		MinifyWhitespace:  minify,
		MinifyIdentifiers: minify,
		MinifySyntax:      minify,
		Plugins:           []api.Plugin{esmaPlugin},
		EntryPoints:       []string{filename},
	}
	result := api.Build(options)
	if l := len(result.Errors); l > 0 {
		texts := make([]string, l)
		for i, e := range result.Errors {
			texts[i] = e.Text
		}
		err = errors.New(strings.Join(texts, "\n"))
		return
	}

	if len(result.OutputFiles) > 0 {
		out = FileContent{
			Content: result.OutputFiles[0].Contents,
			Modtime: fi.ModTime(),
		}
		app.lock.RLock()
		app.builds[filename] = &ESBulidRecord{
			FileContent: out,
			BuildResult: result,
		}
		app.lock.RUnlock()
		return
	}

	err = fmt.Errorf("Unknown error")
	return
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

			if strings.HasSuffix(filepath, ".css") {
				if _, ok := ctx.R.URL.Query()["module"]; ok {
					build, err := app.Build(filepath)
					if err != nil {
						return err
					}

					str, err := json.Marshal(string(build.Content))
					if err != nil {
						return err
					}
					js := fmt.Sprintf(cssLoader, ctx.R.URL.Path, string(str))
					return rex.Content("index.js", build.Modtime, bytes.NewReader([]byte(js)))
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
