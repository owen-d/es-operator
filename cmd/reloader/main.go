package main

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/jessevdk/go-flags"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

var (
	client *http.Client = http.DefaultClient
	opts   Options
)

type Options struct {
	Verbose    bool   `short:"v" long:"verbose" description:"Show verbose debug information"`
	ConfigFile string `short:"c" long:"config-file" required:"true" description:"path to elasticsearch.yml"`
}

const settingsTpl = `
{
  "transient": {
    "discovery.zen.minimum_master_nodes" : %d
  }
}
`

type Config struct {
	Discovery struct {
		Zen struct {
			MininumMasterNodes int `yaml:"minimum_master_nodes"`
		} `yaml:"zen"`
	} `yaml:"discovery"`
}

func main() {
	if opts.ConfigFile == "" {
		bail(errors.New("no config-file specified"))
	}
	log("successfully parsed arguments")

	w, err := fsnotify.NewWatcher()
	if err != nil {
		bail(err)
	}

	if err = w.Add(opts.ConfigFile); err != nil {
		bail(err)
	}

	for {
		select {
		case err = <-w.Errors:
			bail(err)
		case event := <-w.Events:
			log("found event:", event)
			if event.Op == fsnotify.Create ||
				event.Op == fsnotify.Rename ||
				event.Op == fsnotify.Write ||
				event.Op == fsnotify.Remove {

				if err = reload(opts); err != nil {
					log(err)
				}
			}
		}
	}
}

func reload(opts Options) error {
	var conf Config
	b, err := ioutil.ReadFile(opts.ConfigFile)
	if err != nil {
		return err
	}

	if err = yaml.Unmarshal(b, &conf); err != nil {
		return err
	}

	if conf.Discovery.Zen.MininumMasterNodes == 0 {
		return errors.New("unparsed or 0 for [minimum_master_nodes]")
	}

	settings := fmt.Sprintf(settingsTpl, conf.Discovery.Zen.MininumMasterNodes)
	body := ioutil.NopCloser(bytes.NewReader([]byte(settings)))

	log("updating elastic with new minimum_masters", conf.Discovery.Zen.MininumMasterNodes)
	resp, err := client.Post("http://localhost:9200/_cluster/settings", "application/json", body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := ioutil.ReadAll(resp.Body)
		return errors.New(fmt.Sprint("bad status:", resp.StatusCode, "body:", string(body)))
	}
	log("successfully updated elastic with new minimum_masters:", conf.Discovery.Zen.MininumMasterNodes)

	return nil
}

func bail(err error) {
	fmt.Fprint(os.Stderr, err, "\n")
	os.Exit(1)
}

func log(args ...interface{}) {
	if !opts.Verbose {
		return
	}
	fmt.Println(args...)
}

func init() {
	_, err := flags.ParseArgs(&opts, os.Args)
	if err != nil {
		os.Exit(1)
	}
	client.Timeout = time.Second * 3
}
