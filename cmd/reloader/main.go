package main

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/jessevdk/go-flags"
	"github.com/owen-d/es-operator/handlers/watcher"
	"gopkg.in/yaml.v2"
	"io"
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
	log(fmt.Sprintf("%s: %+v", "successfully parsed arguments", opts))

	files, errs, done, err := watcher.ConfigMapChanges(opts.ConfigFile)
	if err != nil {
		bail(err)
	}

	log("awaiting events")
	for {
		select {
		case <-done:
			log("watcher exhausted")
			break
		case err = <-errs:
			log(err)
		case changed := <-files:
			log("file changed:\n", string(changed))
			if err = reload(changed); err != nil {
				log(err)
			}
		}
	}
}

func reload(fileBytes []byte) (err error) {
	var conf Config

	if err = yaml.Unmarshal(fileBytes, &conf); err != nil {
		return err
	}

	if conf.Discovery.Zen.MininumMasterNodes == 0 {
		return errors.New("unparsed or 0 for [minimum_master_nodes]")
	}

	settings := fmt.Sprintf(settingsTpl, conf.Discovery.Zen.MininumMasterNodes)
	body := ioutil.NopCloser(bytes.NewReader([]byte(settings)))

	log("updating elastic with new minimum_masters", conf.Discovery.Zen.MininumMasterNodes)
	resp, err := Put(
		client,
		"http://localhost:9200/_cluster/settings",
		"application/json",
		body,
	)
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

func Put(c *http.Client, url string, contentType string, body io.ReadCloser) (*http.Response, error) {
	req, err := http.NewRequest("PUT", url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return c.Do(req)
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
