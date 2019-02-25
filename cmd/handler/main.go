package main

import (
	"errors"
	"fmt"
	"github.com/jessevdk/go-flags"
	"github.com/owen-d/es-operator/handlers/watcher"
	"github.com/owen-d/es-operator/pkg/controller/util"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

var (
	client *http.Client = http.DefaultClient
	opts   Options
	conf   util.SchedulableConfig
)

type Options struct {
	Verbose    bool   `short:"v" long:"verbose" description:"Show verbose debug information"`
	ConfigFile string `short:"c" long:"config-file" required:"true" description:"path to elasticsearch.yml"`
	NodeName   string
}

func Schedulable(conf util.SchedulableConfig, nodeName string) bool {
	return ExistsIn(conf.SchedulableNodes, opts.NodeName)
}

// Handler applies shard schedulability based on a config file.
// It also maintains a ticker which exits the current process if the current node is not schedulable and has no shards
func main() {
	files, errs, configDone, err := watcher.ConfigMapChanges(opts.ConfigFile)
	if err != nil {
		bail(err)
	}

	done := make(chan struct{})

	go func() {
		log("awaiting events")
		for {
			select {
			case <-configDone:
				log("watcher exhausted")
				done <- struct{}{}
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
			if Schedulable(conf, opts.NodeName) {
				continue
			}

			// check currently allocated shards to this node & if 0, exit
			shards, err := GetShardsForNode(client, opts.NodeName)
			if err != nil {
				log(err)
			}
			if len(shards) == 0 {
				log(fmt.Sprintf("no shards remaining on this unschedulable node [%s], terminating...", opts.NodeName))
				done <- struct{}{}
			}
		}
	}()

	<-done
}

func UpdateRouting(fileBytes []byte) (err error) {
	if err = yaml.Unmarshal(fileBytes, &conf); err != nil {
		return err
	}

	settings, err := GetAllocationSettings(client)
	if err != nil {
		return err
	}

	schedulable := Schedulable(conf, opts.NodeName)
	if !schedulable && !settings.Excluded(opts.NodeName) {
		settings.Exclude(opts.NodeName)
	} else if schedulable && settings.Excluded(opts.NodeName) {
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
		bail(errors.New("no POD_NAME env var"))
	}

	if opts.ConfigFile == "" {
		bail(errors.New("no config-file specified"))
	}
	log(fmt.Sprintf("%s: %+v", "successfully parsed arguments", opts))

	// populate initial config from file
	b, err := ioutil.ReadFile(opts.ConfigFile)
	if err != nil {
		bail(err)
	}

	if err = yaml.Unmarshal(b, &conf); err != nil {
		bail(err)
	}

	client.Timeout = time.Second * 3
}
