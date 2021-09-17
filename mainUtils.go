package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

func isValidPath(s []string, e string) bool {
	for _, a := range s {
		ret, err := filepath.Rel(a, e)
		if err == nil {
			if !strings.HasPrefix(ret, "..") {
				return true
			}
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
	dayDir := now.Format("20060102")
	ducDir := fmt.Sprintf("%s/ducdb", outDir)
	dbDir := fmt.Sprintf("%s/%s", ducDir, dayDir)
	// database 파일 생성 확인.
	if _, err := os.Stat(ducDir); os.IsNotExist(err) {
		os.Mkdir(ducDir, 0755)
	}
	if _, err := os.Stat(dbDir); os.IsNotExist(err) {
		os.Mkdir(dbDir, 0755)
	}
	return fmt.Sprintf("%s/%s.%d.db", dbDir, strings.ReplaceAll(fPath, "/", "."), now.Unix())
}
