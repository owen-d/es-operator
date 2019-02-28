package main

import (
	"bufio"
	"context"
	"fmt"
	"github.com/olivere/elastic"
	"github.com/spf13/cobra"
	"os"
	"time"
)

var (
	n         int
	batchSize int
	index     string
	docType   string
	url       string
	verbose   bool
	fileName  string
)

var rootCmd = &cobra.Command{
	Use:   "es-seed",
	Short: "seed elasticsearch with dummy data",
	RunE:  run,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	if verbose {
		fmt.Println("n:", n)
		fmt.Println("batchSize:", batchSize)
		fmt.Println("index:", index)
		fmt.Println("docType:", docType)
		fmt.Println("url:", url)
		fmt.Println("fileName:", fileName)
	}

	client, err := elastic.NewClient(
		elastic.SetURL(url),
		elastic.SetSniff(false),
		elastic.SetHealthcheck(true),
		elastic.SetHealthcheckTimeoutStartup(time.Minute),
	)

	if err != nil {
		return err
	}

	scanner, closeScanner, err := ScanFile(fileName)
	if err != nil {
		return err
	}
	defer closeScanner()

	svc, err := client.BulkProcessor().
		Name("es-background-worker").
		Workers(1).
		BulkActions(batchSize).
		Do(context.Background())
	if err != nil {
		return err
	}
	defer svc.Close()

	var id int
	for scanner.Scan() {
		// 0 number indicates unspecified number of lines, process until end
		if n != 0 && id >= n {
			fmt.Println("reached n limit:", n)
			break
		}

		doc := mkDoc(scanner.Text(), id)

		if id%100 == 0 && verbose {
			fmt.Println("enqueueing document", id)
		}

		r := elastic.NewBulkIndexRequest().
			Index(index).
			Type(docType).
			Id(string(id)).
			Doc(doc)
		svc.Add(r)

		id += 1
	}

	if err = scanner.Err(); err != nil {
		return err
	}

	// force indexing of any remaining docs
	if err = svc.Flush(); err != nil {
		return err
	}

	return nil
}

type Line struct {
	Line       string `json:"line"`
	LineNumber int    `json:"line_number"`
}

func mkDoc(text string, id int) Line {
	return Line{
		Line:       text,
		LineNumber: id,
	}
}

func ScanFile(name string) (*bufio.Scanner, func() error, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, nil, err
	}
	closer := func() error {
		return file.Close()
	}

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)

	// 1MB max size
	scanner.Buffer(buf, 1024*1024)

	return scanner, closer, nil
}

func init() {
	rootCmd.PersistentFlags().IntVarP(&n, "number", "n", 0, "number of documents to index")
	rootCmd.PersistentFlags().IntVarP(&batchSize, "batch-size", "b", 500, "size of each batch in uploading process")
	rootCmd.PersistentFlags().StringVarP(&index, "index", "i", "dummy", "index to use")
	rootCmd.PersistentFlags().StringVarP(&docType, "type", "t", "sampletype", "type to index")
	rootCmd.PersistentFlags().StringVarP(&fileName, "file", "f", "", "file to split by lines")
	rootCmd.PersistentFlags().StringVarP(&url, "url", "u", "http://localhost:9200", "host for elastic cluster")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
}
