package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"encoding/json"

	"github.com/ShaunPark/nfsCheck/elasticsearch"
	"github.com/ShaunPark/nfsCheck/types"
	"github.com/ShaunPark/nfsCheck/utils"
	"gopkg.in/alecthomas/kingpin.v2"
)

type CheckJob struct {
	config *types.Config
}

const niceArgsFmt = "-n 19 ionice -c 3 duc index %s -d %s"
const ducArgsFmtPersonal = "ls -R %s -d %s --level=0 --dirs-only --full-path --bytes"
const ducArgsFmtProject = "ls -R %s -d %s --level=3 --dirs-only --full-path --bytes"
const ducArgsFmtGlobal = "info %s -d %s --bytes"

func (c CheckJob) run(weekday time.Weekday) {
	fmt.Printf("%s\n", strings.Repeat("=", 40))
	fmt.Printf("NFS disk check job for %s started.\n", weekday)
	fmt.Printf("%s\n", strings.Repeat("=", 40))

	processed := false
	for _, d := range c.config.Days {
		if time.Weekday(weekday).String() == d.Day {
			c.processJob(d.Targets)
			processed = true
		}
	}

	if !processed {
		fmt.Printf("No Job configuration for %s. Skip todays job.\n", weekday)
	}
	fmt.Printf("%s\n", strings.Repeat("=", 40))
	fmt.Printf("NFS disk check job for %s finished.\n", weekday)
	fmt.Printf("%s\n", strings.Repeat("=", 40))
}

func (c CheckJob) processJob(jobs []types.Target) {
	fmt.Printf("Configured Job Count : %d\n", len(jobs))

	for i, job := range jobs {
		targetDir := c.config.MountDir + "/" + job.Location
		database := c.config.OutputDir + "/" + strings.ReplaceAll(job.Location, "/", ".") + ".db"

		fmt.Printf("Job[%d] : %s,  %s, %s\n", (i + 1), job.JobType, job.Type, targetDir)

		if _, err := os.Stat(targetDir); os.IsNotExist(err) {
			fmt.Printf("directory %s is not exist.\n", targetDir)
			listDir(targetDir)
			return
		} else {
			fmt.Printf("directory %s is exist. Go next step.\n", targetDir)
		}

		switch job.Type {
		case "global":
			c.processGlobal(targetDir, database, job)
		case "personal":
			c.processPersonal(targetDir, database, job)
		case "project":
			c.processProject(targetDir, database, job)
		}
	}
}

func listDir(d string) {
	files, err := ioutil.ReadDir(d)
	if err != nil {
		log.Fatal(err)
	}

	for _, f := range files {
		fmt.Println(f.Name())
	}
}

func (c CheckJob) processGlobal(t string, d string, job types.Target) {
	fmt.Println("Start processGlobal")

	dirs := []string{}

	if job.JobType == "subDirs" {
		if files, err := ioutil.ReadDir(t); err != nil {
			log.Fatal(err)
		} else {
			for _, f := range files {
				if f.IsDir() && !contains(job.SkipDirs, f.Name()) {
					dirs = append(dirs, t+"/"+f.Name())
				}
			}
		}
	} else if job.JobType == "singleDir" {
		dirs = append(dirs, t)
	} else {
		fmt.Printf("Invalid JobType [%s]. Skip this job", job.JobType)
		return
	}

	fmt.Printf("Process for %d dirs\n", len(dirs))
	ret := make([]interface{}, 0)

	for _, dir := range dirs {
		if _, err := os.Stat(d); os.IsNotExist(err) {
		} else {
			os.Remove(d)
			fmt.Printf("db file %s is deleted. Go next step.\n", d)
		}

		database := c.config.OutputDir + "/" + strings.ReplaceAll(dir, "/", ".") + ".db"

		runCommand("nice", fmt.Sprintf(niceArgsFmt, t, d))

		if _, err := os.Stat(d); os.IsNotExist(err) {
			fmt.Printf("db file %s is not created.\n", d)
			return
		} else {
			fmt.Printf("directory %s is created. Go next step.\n", d)
		}

		runCommandWithFunc("duc", fmt.Sprintf(ducArgsFmtGlobal, dir, database), ret, func(input string, r []interface{}) {
			strs := strings.Fields(input)
			if strs[0] != "Date" {
				ss := strings.Split(strs[5], "/")

				doc := types.ESDocGlobal{
					ESDoc: types.ESDoc{
						Timestamp:  time.Now(),
						Cluster:    c.config.ClusterName,
						VolumeType: "global",
						FullPath:   "/" + strings.Replace(dir, c.config.MountDir+"/", "", 1),
						DiskSize:   strs[4],
					},
					VolumeName: ss[len(ss)-1],
				}
				ret = append(ret, doc)
			}
		})
	}

	saveToElasticSearch(*c.config, ret)
}

func (c CheckJob) processProject(t string, d string, job types.Target) {
	ret := make([]interface{}, 0)
	runCommand("nice", fmt.Sprintf(niceArgsFmt, t, d))
	runCommandWithFunc("duc", fmt.Sprintf(ducArgsFmtProject, t, d), ret, func(input string, r []interface{}) {
		str := strings.Split(strings.Trim(input, " "), " ")
		paths := strings.Split(str[1], "/")
		if len(paths) == 4 && !startsWith(job.SkipDirs, str[1]) {
			doc := types.ESDocProject{
				ESDocGlobal: types.ESDocGlobal{
					ESDoc: types.ESDoc{
						Timestamp:  time.Now(),
						Cluster:    c.config.ClusterName,
						VolumeType: "project",
						FullPath:   "/" + job.Location + "/" + str[1],
						DiskSize:   str[0],
					},
					VolumeName: paths[3],
				},
				ProjectName: paths[2],
			}
			ret = append(ret, doc)
		}
	})

	saveToElasticSearch(*c.config, ret)
}

func (c CheckJob) processPersonal(t string, d string, job types.Target) {
	ret := make([]interface{}, 0)
	runCommand("nice", fmt.Sprintf(niceArgsFmt, t, d))
	runCommandWithFunc("duc", fmt.Sprintf(ducArgsFmtPersonal, t, d), ret, func(input string, r []interface{}) {
		println(input)
		str := strings.Split(strings.Trim(input, " "), " ")

		if !contains(job.SkipDirs, str[1]) {
			doc := types.ESDocPersonal{
				ESDoc: types.ESDoc{
					Timestamp:  time.Now(),
					Cluster:    c.config.ClusterName,
					VolumeType: "personal",
					FullPath:   "/" + job.Location + "/" + str[1],
					DiskSize:   str[0],
				},
				UserName: str[1],
			}
			ret = append(ret, doc)
		}
	})

	saveToElasticSearch(*c.config, ret)
}

func runCommand(c string, args string) {
	cmd := exec.Command(c, strings.Fields(args)...)
	if err := cmd.Start(); err != nil {
		panic(err)
	}
	defer cmd.Wait()
}

func runCommandWithFunc(c string, args string, r []interface{}, f func(t string, r []interface{})) {
	cmd := exec.Command(c, strings.Fields(args)...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		panic(err)
	}
	buf := bufio.NewScanner(out)
	if err := cmd.Start(); err != nil {
		panic(err)
	}
	defer cmd.Wait()

	for buf.Scan() {
		f(buf.Text(), r)
	}
	defer cmd.Wait()
}

func saveToElasticSearch(config types.Config, v []interface{}) {
	for _, d := range v {
		printJson(d)
	}

	es := elasticsearch.NewESClient(&config)
	es.Bulk(v)
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func startsWith(s []string, e string) bool {
	for _, a := range s {
		if strings.HasPrefix(e, a) {
			return true
		}
	}
	return false
}

func printJson(v interface{}) {
	bytes, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}

	println(string(bytes))
}

var (
	configFile = kingpin.Flag("configFile", "Config yaml file.").Short('f').String()
)

func main() {
	kingpin.Parse()

	cm := utils.NewConfigManager(*configFile)
	cj := &CheckJob{config: cm.GetConfig()}

	cj.run(time.Now().Weekday())
}
