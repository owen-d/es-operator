package main

import (
	"errors"
	"fmt"
	"github.com/jessevdk/go-flags"
	"github.com/owen-d/es-operator/handlers/watcher"
	"gopkg.in/yaml.v2"
	"net/http"
	"os"
	"time"
)

var (
	client *http.Client = http.DefaultClient
	opts   Options
	conf   ConfigFile
)

type Options struct {
	Verbose    bool   `short:"v" long:"verbose" description:"Show verbose debug information"`
	ConfigFile string `short:"c" long:"config-file" required:"true" description:"path to elasticsearch.yml"`
	NodeName   string
}

type ConfigFile struct {
	UnSchedulable bool `yaml:"unschedulable"`
}

// Handler applies shard schedulability based on a config file.
// It also maintains a ticker which exits the current process if the current node is not schedulable and has no shards
func main() {
	files, errs, done, err := watcher.ConfigMapChanges(opts.ConfigFile)
	if err != nil {
		bail(err)
	}

	go func() {
		log("awaiting events")
		for {
			select {
			case <-done:
				log("watcher exhausted")
				os.Exit(1)
			case err = <-errs:
				log(err)
			case changed := <-files:
				log("file changed:\n", string(changed))
				if err = UpdateRouting(changed); err != nil {
					log(err)
				}
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(time.Second * 10)
		for {
			<-ticker.C
			if !conf.UnSchedulable {
				continue
			}

			// check currently allocated shards to this node & if 0, exit
			// TODO: implement (_cat/shards)
			panic("unimplemented")
		}
	}()

}

func UpdateRouting(fileBytes []byte) (err error) {
	if err = yaml.Unmarshal(fileBytes, &conf); err != nil {
		return err
	}

	settings, err := GetAllocationSettings(client)
	if err != nil {
		return err
	}

	if conf.UnSchedulable && !settings.Excluded(opts.NodeName) {
		settings.Exclude(opts.NodeName)
	} else if !conf.UnSchedulable && settings.Excluded(opts.NodeName) {
		settings.Include(opts.NodeName)
	} else {
		return nil
	}

	_, err = PutAllocationSettings(client, settings)

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

	opts.NodeName = os.Getenv("POD_NAME")
	if opts.NodeName == "" {
		bail(errors.New("no NODE_NAME env var"))
	}

	if opts.ConfigFile == "" {
		bail(errors.New("no config-file specified"))
	}
	log(fmt.Sprintf("%s: %+v", "successfully parsed arguments", opts))

	client.Timeout = time.Second * 3
}
