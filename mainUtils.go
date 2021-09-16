package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ShaunPark/nfsCheck/elasticsearch"
	"github.com/ShaunPark/nfsCheck/types"
)

type CommandFunc func(input string, r []interface{})

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func printJson(v interface{}) {
	bytes, _ := json.Marshal(v)
	println(string(bytes))
}

func saveToElasticSearch(config types.Config, v []interface{}) {
	for _, d := range v {
		printJson(d)
	}

	es := elasticsearch.NewESClient(&config)
	es.Bulk(v)
}

func makeDBfileName(outDir string, fPath string) string {
	now := time.Now()
	return fmt.Sprintf("%s/%s.%d.db", outDir, strings.ReplaceAll(fPath, "/", "."), now.Unix())
}
