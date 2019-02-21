package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"net/http"
	"time"
)

var (
	configFile string
	client     *http.Client = http.DefaultClient
)

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
	if configFile == "" {
		panic("no configFile specified")
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		panic(err)
	}

	if err = w.Add(configFile); err != nil {
		panic(err)
	}

	for {
		select {
		case err = <-w.Errors:
			panic(err)
		case event := <-w.Events:
			if event.Op == fsnotify.Create ||
				event.Op == fsnotify.Write ||
				event.Op == fsnotify.Remove {

				if err = reload(); err != nil {
					panic(err)
				}
			}
		}
	}
}

func reload() error {
	var conf Config
	b, err := ioutil.ReadFile(configFile)
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

	resp, err := client.Post("localhost:9200/_cluster/settings", "application/json", body)
	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := ioutil.ReadAll(resp.Body)
		return errors.New(fmt.Sprint("bad status:", resp.StatusCode, "body:", string(body)))
	}

	return nil
}

func init() {
	flag.StringVar(&configFile, "configFile", "", "config file to watch")
	client.Timeout = time.Second * 3
}
