package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

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

	// es := elasticsearch.NewESClient(&config)
	// es.Bulk(v)
}

func makeDBfileName(outDir string, fPath string) string {
	now := time.Now()
	return fmt.Sprintf("%s/%s.%d.db", outDir, strings.ReplaceAll(fPath, "/", "."), now.Unix())
}

func runCommand(cmdStr string) error {
	fields := strings.Fields(cmdStr)
	cmd := exec.Command(fields[0], fields[1:]...)
	if err := cmd.Start(); err != nil {
		return err
	}
	defer cmd.Wait()
	return nil
}

func runCommandGetStdOutBytes(cmdStr string) ([]byte, error) {
	fields := strings.Fields(cmdStr)
	cmd := exec.Command(fields[0], fields[1:]...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	buf := bufio.NewScanner(out)
	buf2 := bufio.NewScanner(stderr)
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	ret := []byte{}
	for buf.Scan() {
		ret = append(ret, buf.Bytes()...)
	}

	for buf2.Scan() {
		fmt.Println(buf2.Text())
	}
	defer cmd.Wait()

	return ret, nil
}
