package server

import (
	"bytes"
	"embed"
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
	lock    sync.RWMutex
	embedFS *embed.FS
	wd      string
	dev     bool
	builds  map[string]FileContent
}

func (app *App) Build(filename string, rebuild bool) (out FileContent, err error) {
	if !rebuild {
		app.lock.RLock()
		record, ok := app.builds[filename]
		app.lock.RUnlock()
		if ok {
			out = record
			return
		}
	}

	var isBuiltin = strings.HasPrefix(filename, "/builtin:")
	var modtime = time.Time{}

	if !isBuiltin {
		var fi os.FileInfo
		fi, err = os.Lstat(filename)
		if err != nil {
			return
		}

		if fi.IsDir() {
			err = errors.New("can't build a directory")
			return
		}

		modtime = fi.ModTime()
	}

	if strings.HasSuffix(filename, "/index.html") {
		var data []byte
		data, err = ioutil.ReadFile(filename)
		if err != nil {
			return
		}
		out = FileContent{
			Modtime: modtime,
			Content: data,
		}
		app.lock.Lock()
		app.builds[filename] = out
		app.lock.Unlock()
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

	start := time.Now()
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
	}
	if isBuiltin {
		var data []byte
		name := strings.TrimPrefix(filename, "/builtin:")
		data, err = app.embedFS.ReadFile("embed/builtin/" + name)
		if err != nil {
			return
		}
		options.Stdin = &api.StdinOptions{
			Contents:   string(data),
			ResolveDir: "embed/builtin/",
			Sourcefile: name,
			Loader:     api.LoaderTS,
		}
	} else {
		options.EntryPoints = []string{filename}
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

	log.Debugf("Build %s in %v", strings.TrimPrefix(strings.TrimPrefix(filename, app.wd), "/"), time.Now().Sub(start))

	if len(result.OutputFiles) > 0 {
		out = FileContent{
			Content: result.OutputFiles[0].Contents,
			Modtime: modtime,
		}
		app.lock.Lock()
		app.builds[filename] = out
		app.lock.Unlock()
		return
	}

	err = fmt.Errorf("Unknown error")
	return
}

func (app *App) Watch() {
	w := &watcher{
		app:      app,
		interval: 50 * time.Millisecond,
	}
	w.start(func(filename string, exists bool) {
		if exists {
			_, err := app.Build(filename, true)
			if err != nil {
				app.removeBuild(filename)
				os.Stdout.WriteString(err.Error())
			}
		} else {
			app.removeBuild(filename)
		}
	})
	log.Debug("Watching file for changes...")
}

func (app *App) Handle() rex.Handle {
	return func(ctx *rex.Context) interface{} {
		// in dev mode, we use `Last-Modified` and `ETag` header to control cache
		if app.dev {
			ctx.SetHeader("Cache-Control", "max-age=0")
		}

		pathname := ctx.R.URL.Path
		if strings.HasPrefix(pathname, "/builtin:") {
			build, err := app.Build(pathname, false)
			if err != nil {
				return err
			}
			return rex.Content("index.js", build.Modtime, bytes.NewReader(build.Content))
		}

		filepath := path.Join(app.wd, pathname)
		if fileExists(filepath) {
			for _, ext := range defaultModuleExts {
				if strings.HasSuffix(filepath, ext) {
					build, err := app.Build(filepath, false)
					if err != nil {
						return err
					}
					return rex.Content("index.js", build.Modtime, bytes.NewReader(build.Content))
				}
			}

			if strings.HasSuffix(filepath, ".css") {
				if _, ok := ctx.R.URL.Query()["module"]; ok {
					build, err := app.Build(filepath, false)
					if err != nil {
						return err
					}

					str, err := json.Marshal(string(build.Content))
					if err != nil {
						return err
					}
					js := fmt.Sprintf(cssLoader, pathname, string(str))
					return rex.Content("index.js", build.Modtime, bytes.NewReader([]byte(js)))
				}
			}

			return rex.File(filepath)
		}

		if pathname == "/favicon.ico" {
			return rex.Status(404, "not found")
		}

		filepath = path.Join(app.wd, pathname, "index.html")
		if !fileExists(filepath) {
			// fallback to root index.html
			filepath = path.Join(app.wd, "index.html")
		}
		if fileExists(filepath) {
			build, err := app.Build(filepath, false)
			if err != nil {
				return err
			}
			return rex.Content("index.html", build.Modtime, bytes.NewReader(build.Content))
		}

		for _, name := range []string{"app", "main", "index"} {
			for _, ext := range defaultModuleExts {
				filename := name + ext
				fi, err := os.Lstat(filename)
				if err == nil && !fi.IsDir() {
					return rex.Content("index.html", fi.ModTime(), bytes.NewReader([]byte(fmt.Sprintf(defaultIndexHtml, "./"+filename))))
				}
			}
		}

		return rex.Status(404, "not found")
	}
}

func (app *App) removeBuild(filename string) {
	app.lock.Lock()
	defer app.lock.Unlock()

	delete(app.builds, filename)
}
