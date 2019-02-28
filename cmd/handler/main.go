package main

import (
	"errors"
	"fmt"
	"github.com/jessevdk/go-flags"
	"github.com/olivere/elastic"
	"github.com/owen-d/es-operator/handlers/watcher"
	"github.com/owen-d/es-operator/pkg/controller/util"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

var (
	client   *http.Client = http.DefaultClient
	opts     Options
	conf     util.SchedulableConfig
	esClient *elastic.Client
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
	setup()
	files, errs, configDone, err := watcher.ConfigMapChanges(opts.ConfigFile)
	if err != nil {
		bail(err)
	}

	done := make(chan struct{})

	go func() {
		// create a ticker to ensure an initial routing update is performed.
		// This is helpful when scaling back up after a node has been marked unschedulable.
		// It allows the node to remark itself as schedulable in accordance with it's configmap on an initial mount.

		ticker := time.NewTicker(time.Second * 10)

		log("awaiting events")

		for {

			select {
			case <-configDone:
				log("watcher exhausted")
				done <- struct{}{}
			case err = <-errs:
				log(err)
			case <-ticker.C:
				log(fmt.Sprintf("ensuring initial routing for node [%s]", opts.NodeName))
				if err = UpdateRouting(); err != nil {
					log(err)
				} else {
					// once the ticker-spawned update completes once, close it.
					// Successive updates only need be performed in response
					// to configmap changes
					ticker.Stop()
				}
			case changed := <-files:
				log("file changed:\n", string(changed))
				if err = yaml.Unmarshal(changed, &conf); err != nil {
					log(err)
				}
				if err = UpdateRouting(); err != nil {
					log(err)
				}
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(time.Second * 10)
		log("ticker started")
		for {
			<-ticker.C
			if Schedulable(conf, opts.NodeName) {
				continue
			}

			// check currently allocated shards to this node & if 0, exit
			numShards, err := GetShardsForNode(esClient, opts.NodeName)
			log(fmt.Sprintf("found [%d] shards for unschedulable node [%s]", numShards, opts.NodeName))
			if err != nil {
				log(err)
			}
			if numShards == 0 {
				log(fmt.Sprintf("no shards remaining on this unschedulable node [%s], terminating...", opts.NodeName))
				done <- struct{}{}
			}
		}
	}()

	<-done
}

func UpdateRouting() (err error) {
	settings, err := GetAllocationSettings(client)
	if err != nil {
		return err
	}

	schedulable := Schedulable(conf, opts.NodeName)
	if !schedulable && !settings.Excluded(opts.NodeName) {
		settings.Exclude(opts.NodeName)
		log(fmt.Sprintf(
			"[%s] marked as unschedulable but not marked as such in elastic, updating elastic with new settings:\n%+v",
			opts.NodeName,
			settings,
		))
	} else if schedulable && settings.Excluded(opts.NodeName) {
		settings.Include(opts.NodeName)
		log(fmt.Sprintf(
			"[%s] marked as schedulable but not marked as such in elastic, updating elastic with new settings:\n%+v",
			opts.NodeName,
			settings,
		))
	} else {
		return nil
	}

	_, err = PutAllocationSettings(client, settings)

	return err
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

func setup() {
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

	esClient, err = elastic.NewClient(
		elastic.SetHttpClient(client),
		elastic.SetSniff(false),
		elastic.SetHealthcheck(true),
		elastic.SetHealthcheckTimeoutStartup(time.Minute),
	)

	if err != nil {
		bail(err)
	}
}
