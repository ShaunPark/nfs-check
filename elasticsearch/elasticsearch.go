package elasticsearch

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ShaunPark/nfsCheck/types"
	"github.com/dustin/go-humanize"
	elasticsearch "github.com/elastic/go-elasticsearch/v7"
	"github.com/elastic/go-elasticsearch/v7/esapi"
)

type ElasticSearch struct {
	client    *elasticsearch.Client
	indexName string
}

func NewESClient(config *types.Config) *ElasticSearch {
	elHost := config.ElasticSearch.Host
	elPort := config.ElasticSearch.Port
	elPassword := os.Getenv("ES_PASSWORD")
	elId := config.ElasticSearch.Id
	elIndexName := config.ElasticSearch.IndexName
	cfg := elasticsearch.Config{
		Addresses: []string{
			"http://" + strings.TrimSpace(elHost) + ":" + elPort,
		},
		Transport: &http.Transport{
			MaxIdleConnsPerHost:   10,
			ResponseHeaderTimeout: time.Second,
			DialContext:           (&net.Dialer{Timeout: time.Second * 30}).DialContext,
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS11,
			},
		},
	}

	if elId == "" {
		cfg.Username = elId
		cfg.Password = elPassword
	}

	client, err := elasticsearch.NewClient(cfg)
	if err != nil {
		panic(err)
	}
	return &ElasticSearch{client: client, indexName: elIndexName}
}

type bulkResponse struct {
	Errors bool `json:"errors"`
	Items  []struct {
		Index struct {
			ID     string `json:"_id"`
			Result string `json:"result"`
			Status int    `json:"status"`
			Error  struct {
				Type   string `json:"type"`
				Reason string `json:"reason"`
				Cause  struct {
					Type   string `json:"type"`
					Reason string `json:"reason"`
				} `json:"caused_by"`
			} `json:"error"`
		} `json:"index"`
	} `json:"items"`
}

func (e *ElasticSearch) Bulk(v []interface{}) {
	meta := []byte(fmt.Sprintf(`{ "index" : { } }%s`, "\n"))
	var (
		buf        bytes.Buffer
		numItems   int
		numErrors  int
		numIndexed int
		currBatch  int
		batch      int = 255
		res        *esapi.Response
		raw        map[string]interface{}
		blk        *bulkResponse
	)
	start := time.Now().UTC()

	count := len(v)
	for i, d := range v {
		data, err := json.Marshal(d)
		if err != nil {
			log.Fatalf("Cannot encode document %s: %s", e.indexName, err)
		}
		data = append(data, "\n"...) // <-- Comment out to trigger failure for batch

		buf.Grow(len(meta) + len(data))
		buf.Write(meta)
		buf.Write(data)

		if i > 0 && i%batch == 0 || i == count-1 {
			res, err = e.client.Bulk(bytes.NewReader(buf.Bytes()), e.client.Bulk.WithIndex(e.indexName))
			if err != nil {
				log.Fatalf("Failure indexing batch %d: %s", currBatch, err)
			}
			// If the whole request failed, print error and mark all documents as failed
			//
			if res.IsError() {
				numErrors += numItems
				if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
					log.Fatalf("Failure to to parse response body: %s", err)
				} else {
					log.Printf("  Error: [%d] %s: %s",
						res.StatusCode,
						raw["error"].(map[string]interface{})["type"],
						raw["error"].(map[string]interface{})["reason"],
					)
				}
				// A successful response might still contain errors for particular documents...
				//
			} else {
				if err := json.NewDecoder(res.Body).Decode(&blk); err != nil {
					log.Fatalf("Failure to to parse response body: %s", err)
				} else {
					for _, d := range blk.Items {
						// ... so for any HTTP status above 201 ...
						//
						if d.Index.Status > 201 {
							// ... increment the error counter ...
							//
							numErrors++

							// ... and print the response status and error information ...
							log.Printf("  Error: [%d]: %s: %s: %s: %s",
								d.Index.Status,
								d.Index.Error.Type,
								d.Index.Error.Reason,
								d.Index.Error.Cause.Type,
								d.Index.Error.Cause.Reason,
							)
						} else {
							// ... otherwise increase the success counter.
							//
							numIndexed++
						}
					}
				}
			}

			// Close the response body, to prevent reaching the limit for goroutines or file handles
			//
			res.Body.Close()

			// Reset the buffer and items counter
			//
			buf.Reset()
			numItems = 0
		}

	}
	dur := time.Since(start)

	if numErrors > 0 {
		log.Fatalf(
			"Indexed [%s] documents with [%s] errors in %s (%s docs/sec)",
			humanize.Comma(int64(numIndexed)),
			humanize.Comma(int64(numErrors)),
			dur.Truncate(time.Millisecond),
			humanize.Comma(int64(1000.0/float64(dur/time.Millisecond)*float64(numIndexed))),
		)
	} else {
		log.Printf(
			"Sucessfuly indexed [%s] documents in %s (%s docs/sec)",
			humanize.Comma(int64(numIndexed)),
			dur.Truncate(time.Millisecond),
			humanize.Comma(int64(1000.0/float64(dur/time.Millisecond)*float64(numIndexed))),
		)
	}
}

func (e *ElasticSearch) Delete(id string) error {
	res, err := e.client.Delete(e.indexName, id)
	if err != nil {
		return err
	}

	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("error: %s", res)
	}
	return nil
}

func (e *ElasticSearch) Update(id string, payload []byte) error {
	res, err := e.client.Update(e.indexName, id, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("error: %s", res)
	}
	return nil
}

func (e ElasticSearch) IsExist() (bool, error) {
	var indices []string
	indices = append(indices, e.indexName)
	res, err := e.client.Indices.Exists(indices)

	if err != nil {
		return false, err
	}
	if res.IsError() {
		return false, nil
	}
	defer res.Body.Close()

	return true, nil
}

func (e *ElasticSearch) CreateIndex(mapping string) error {

	res, err := e.client.Indices.Create(e.indexName,
		e.client.Indices.Create.WithBody(strings.NewReader(mapping)),
		e.client.Indices.Create.WithTimeout(30))

	if err != nil {
		return err
	}

	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("error: %s", res)
	}

	return nil
}

// func (e *ElasticSearch) Put(payload []byte) error {
// 	r := bytes.NewReader(payload)
// 	uuid := uuid.New().String()
// 	uuid = strings.Replace(uuid, "-", "", -1)

// 	res, err := e.client.Create(e.indexName, uuid, r)

// 	if err != nil {
// 		return err
// 	}

// 	defer res.Body.Close()

// 	if res.IsError() {
// 		var e map[string]interface{}
// 		if err := json.NewDecoder(res.Body).Decode(&e); err != nil {
// 			return err
// 		}
// 		return fmt.Errorf("[%s] %s: %s", res.Status(), e["error"].(map[string]interface{})["type"], e["error"].(map[string]interface{})["reason"])
// 	}

// 	return nil
// }

func (e *ElasticSearch) Search(data string, after ...string) (*EnvelopeResponse, error) {
	var b strings.Builder

	b.WriteString("\"query\": {\"match\":")
	b.WriteString(data)
	b.WriteString("}")

	var r EnvelopeResponse

	res, err := e.client.Search(
		e.client.Search.WithIndex(e.indexName),
		e.client.Search.WithBody(e.buildQuery(b.String(), after...)),
	)
	if err != nil {
		return &r, err
	}
	defer res.Body.Close()

	if res.IsError() {
		var e map[string]interface{}
		if err := json.NewDecoder(res.Body).Decode(&e); err != nil {
			return &r, err
		}
		return &r, fmt.Errorf("[%s] %s: %s", res.Status(), e["error"].(map[string]interface{})["type"], e["error"].(map[string]interface{})["reason"])
	}

	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return &r, err
	}
	return &r, nil
}

func (e *ElasticSearch) buildQuery(query string, after ...string) io.Reader {
	var b strings.Builder

	b.WriteString("{\n")

	if len(after) > 0 && after[0] != "" && after[0] != "null" {
		b.WriteString(",\n")
		b.WriteString(fmt.Sprintf(`	"search_after": %s`, after))
	}

	b.WriteString("\n}")

	// fmt.Printf("%s\n", b.String())
	return strings.NewReader(b.String())
}

type EnvelopeResponse struct {
	Took int
	Hits struct {
		Total struct {
			Value int
		}
		Hits []struct {
			ID         string          `json:"_id"`
			Source     json.RawMessage `json:"_source"`
			Highlights json.RawMessage `json:"highlight"`
			Sort       []interface{}   `json:"sort"`
		}
	}
}
