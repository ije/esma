package server

import (
	"embed"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"syscall"

	logx "github.com/ije/gox/log"
	"github.com/ije/gox/utils"
	"github.com/ije/rex"
)

var (
	config Config
	log    *logx.Logger
)

// Serve serves ESMD server
func Serve(fs *embed.FS) {
	var port int
	var httpsPort int
	var logLevel string
	var command string
	var workingDir string
	var err error

	flag.IntVar(&port, "port", 80, "http server port")
	flag.IntVar(&httpsPort, "https-port", 443, "https server port")
	flag.StringVar(&logLevel, "log-level", "info", "log level")
	flag.Parse()

	if l := len(flag.Args()); l > 0 {
		if l > 1 {
			command = flag.Arg(0)
			workingDir, err = filepath.Abs(flag.Arg(1))
		} else {
			workingDir, err = filepath.Abs(flag.Arg(0))
		}
	} else {
		workingDir, err = os.Getwd()
	}
	if err != nil {
		log.Fatal(err)
	}

	if !dirExists(workingDir) {
		log.Fatalf("no such working dir: %s", workingDir)
	}

	configFile := path.Join(workingDir, "esmd.config.json")
	if fileExists(configFile) {
		err := utils.ParseJSONFile(configFile, &config)
		if err != nil {
			log.Fatalf("invalid config file: %v", err)
		}
	}

	if command == "dev" {
		log.SetLevelByName("debug")
	} else {
		log.SetLevelByName(logLevel)
	}

	if config.LogDir != "" {
		log, err = logx.New(fmt.Sprintf("file:%s?buffer=32k", path.Join(config.LogDir, "main.log")))
		if err != nil {
			fmt.Printf("initiate main logger: %v\n", err)
			os.Exit(1)
		}
		defer log.FlushBuffer()

		accessLogger, err := logx.New(fmt.Sprintf("file:%s?buffer=32k&fileDateFormat=20060102", path.Join(config.LogDir, "access.log")))
		if err != nil {
			log.Fatalf("initiate access logger: %v", err)
		}
		accessLogger.SetQuite(true)
		rex.Use(rex.AccessLogger(accessLogger))
		defer accessLogger.FlushBuffer()
	}

	app := &App{
		embedFS: fs,
		wd:      workingDir,
		dev:     command == "dev",
		builds:  map[string]FileContent{},
	}

	rex.Use(
		rex.ErrorLogger(log),
		rex.Header("Server", "esmd"),
		rex.Cors(rex.CORS{
			AllowAllOrigins: true,
			AllowMethods:    []string{"GET"},
			AllowHeaders:    []string{"Origin", "Content-Type", "Content-Length", "Accept-Encoding"},
			MaxAge:          3600,
		}),
		app.Handle(),
	)

	C := rex.Serve(rex.ServerConfig{
		Port: uint16(port),
		TLS: rex.TLSConfig{
			Port: uint16(httpsPort),
			AutoTLS: rex.AutoTLSConfig{
				AcceptTOS: command != "dev",
				Hosts:     config.Autotls.Hosts,
				CacheDir:  config.Autotls.CacheDir,
			},
		},
	})

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGKILL, syscall.SIGHUP)

	if command == "dev" {
		go app.Watch()
		log.Debugf("Server ready on http://localhost:%d", port)
	}

	select {
	case <-c:
	case err := <-C:
		log.Error(err)
	}

	// release resources
}

func init() {
	log = &logx.Logger{}
}
