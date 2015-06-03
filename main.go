package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"runtime"
	"time"
)

var (
	port               int
	version            bool
	maxHandleCount     int
	host               string
	uiDir              string
	enableInternalApis bool    // internal APIs are not exposed to public
	syncWords          bool    // when true, sync won't be performed. Useful when running on a top level server where no upstream can be configured
	logToFile          bool    // logs will be written to file when true
	varnamdConfig      *config // config instance used across the application
	startedAt          time.Time
)

// varnamd configurations
// usually resides in $HOME/.varnamd/config on POSIX and APPDATA/.varnamd/config on Windows
type config struct {
	Upstream           string          `json:"upstream"`
	SchemesToSync      map[string]bool `json:"schemesToSync"`
	SyncIntervalInSecs time.Duration   `json:syncIntervalInSecs`
}

func initDefaultConfig() *config {
	c := &config{}
	c.setDefaultsForBlankValues()
	return c
}

func (c *config) setDefaultsForBlankValues() {
	if c.Upstream == "" {
		c.Upstream = "http://api.varnamproject.com"
	}
	if c.SchemesToSync == nil {
		c.SchemesToSync = make(map[string]bool)
	}
	if c.SyncIntervalInSecs == 0 {
		c.SyncIntervalInSecs = 30
	}
}

func getConfigDir() string {
	if runtime.GOOS == "windows" {
		return path.Join(os.Getenv("localappdata"), ".varnamd")
	} else {
		return path.Join(os.Getenv("HOME"), ".varnamd")
	}
}

func getLogsDir() string {
	d := getConfigDir()
	logsDir := path.Join(d, "logs")
	err := os.MkdirAll(logsDir, 0777)
	if err != nil {
		panic(err)
	}

	return logsDir
}

func getConfigFilePath() string {
	configDir := getConfigDir()
	configFilePath := path.Join(configDir, "config.json")
	return configFilePath
}

func loadConfigFromFile() *config {
	configFilePath := getConfigFilePath()
	configFile, err := os.Open(configFilePath)
	if err != nil {
		c := initDefaultConfig()
		c.save()
		return initDefaultConfig()
	}
	defer configFile.Close()

	jsonDecoder := json.NewDecoder(configFile)
	var c config
	err = jsonDecoder.Decode(&c)
	if err != nil {
		log.Printf("%s is malformed. Using default config instead\n", configFilePath)
		return initDefaultConfig()
	}

	c.setDefaultsForBlankValues()
	return &c
}

func (c *config) setSyncStatus(langCode string, status bool) {
	c.SchemesToSync[langCode] = status
}

func (c *config) save() error {
	configFilePath := getConfigFilePath()
	err := os.MkdirAll(path.Dir(configFilePath), 0777)
	if err != nil {
		return err
	}

	configFile, err := os.Create(configFilePath)
	if err != nil {
		return err
	}
	defer configFile.Close()

	b, err := json.MarshalIndent(c, "", "\t")
	if err != nil {
		return err
	}

	_, err = configFile.Write(b)
	if err != nil {
		return err
	}

	return nil
}

func redirectLogToFile() {
	year, month, day := time.Now().Date()
	logfile := path.Join(getLogsDir(), fmt.Sprintf("%d-%d-%d.log", year, month, day))
	f, err := os.OpenFile(logfile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		panic(err)
	}
	log.SetOutput(f)
}

func init() {
	flag.IntVar(&port, "p", 8080, "Run daemon in specified port")
	flag.IntVar(&maxHandleCount, "max-handle-count", 10, "Maximum number of handles can be opened for each language")
	flag.StringVar(&host, "host", "", "Host for the varnam daemon server")
	flag.StringVar(&uiDir, "ui", "", "UI directory path")
	flag.BoolVar(&enableInternalApis, "enable-internal-apis", false, "Enable internal APIs")
	flag.BoolVar(&syncWords, "sync-words", true, "Enable/Disable word synchronization")
	flag.BoolVar(&logToFile, "log-to-file", false, "If true, logs will be written to a file")
	flag.BoolVar(&version, "version", false, "Print the version and exit")
	varnamdConfig = loadConfigFromFile()
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	flag.Parse()
	startedAt = time.Now()
	if version {
		fmt.Println(VERSION)
		os.Exit(0)
	}
	if logToFile {
		redirectLogToFile()
	}
	if syncWords {
		sync := newSyncDispatcher(varnamdConfig.SyncIntervalInSecs * time.Second)
		sync.start()
		sync.runNow() // Run immediatly when starting varnamd
	}
	startDaemon()
}
