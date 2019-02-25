package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
)

/*
example:
{
	"transient": {
		"cluster": {
			"routing": {
				"allocation": {
					"exclude": {
						"_name": "mycluster-quorum-master-1"
					}
				}
			}
		}
	}
}
*/

type ElasticAllocationSettings struct {
	Transient struct {
		Cluster struct {
			Routing struct {
				Allocation struct {
					Exclude struct {
						Name string `json:"_name,omitempty"`
					} `json:"exclude,omitempty"`
				} `json:"allocation,omitempty"`
			} `json:"routing,omitempty"`
		} `json:"cluster,omitempty"`
	} `json:"persistent,omitempty"`
}

func ExistsIn(xs []string, val string) bool {
	for _, x := range xs {
		if x == val {
			return true
		}
	}
	return false

}

func (e *ElasticAllocationSettings) Excluded(name string) bool {
	return ExistsIn(strings.Split(e.Transient.Cluster.Routing.Allocation.Exclude.Name, ","), name)
}

func (e *ElasticAllocationSettings) Exclude(name string) {
	if e.Excluded(name) {
		return
	}
	exclusions := strings.Split(e.Transient.Cluster.Routing.Allocation.Exclude.Name, ",")
	exclusions = append(exclusions, name)
	e.Transient.Cluster.Routing.Allocation.Exclude.Name = strings.Join(exclusions, ",")
}

func (e *ElasticAllocationSettings) Include(name string) {
	if !e.Excluded(name) {
		return
	}

	exclusions := strings.Split(e.Transient.Cluster.Routing.Allocation.Exclude.Name, ",")
	newExclusions := []string{}

	for _, exclusion := range exclusions {
		if exclusion != name {
			newExclusions = append(newExclusions, exclusion)
		}
	}

	e.Transient.Cluster.Routing.Allocation.Exclude.Name = strings.Join(newExclusions, ",")
}

func GetAllocationSettings(client *http.Client) (settings ElasticAllocationSettings, err error) {
	resp, err := client.Get("http://localhost:9200/_cluster/settings")
	if err != nil {
		return settings, err
	}
	if resp.StatusCode != 200 {
		return settings, fmt.Errorf("bad status code: [%d]", resp.StatusCode)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err == nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return settings, err
	}

	err = json.Unmarshal(body, &settings)
	return settings, err
}

func PutAllocationSettings(client *http.Client, settings ElasticAllocationSettings) (resp *http.Response, err error) {
	b, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}
	body := ioutil.NopCloser(bytes.NewReader(b))
	return Put(
		client,
		"http://localhost:9200/_cluster/settings",
		"application/json",
		body,
	)

}

type Shard struct{}

func GetShardsForNode(client *http.Client, nodeName string) (shards []Shard, err error) {
	return
}

func Put(c *http.Client, url string, contentType string, body io.ReadCloser) (*http.Response, error) {
	req, err := http.NewRequest("PUT", url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return c.Do(req)
}
