package main

import (
	"encoding/xml"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"strings"
	"time"

	"github.com/ShaunPark/nfsCheck/types"
	"github.com/ShaunPark/nfsCheck/utils"

	"gopkg.in/alecthomas/kingpin.v2"
)

type CheckJob struct {
	config *types.Config
}

const SINGLE_DIR = "singleDir"
const SUB_DIRS = "subDirs"
const GLOBAL = "global"
const PROJECT = "project"
const PERSONAL = "personal"

const INDEX_FMT = "nice -n 19 ionice -c 3 duc index %s -d %s -m %d"
const XML_FMt = "duc xml %s -d %s"

type docFunc func(path string, size string) (v interface{})

func (c CheckJob) run(weekday time.Weekday) {
	fmt.Printf("%s\n", strings.Repeat("=", 40))
	fmt.Printf("NFS disk check job for %s started.\n", weekday)
	fmt.Printf("%s\n", strings.Repeat("=", 40))

	processed := false

	for _, d := range c.config.Days {
		if time.Weekday(weekday).String() == d.Day {
			if processed {
				fmt.Printf("%s\n", strings.Repeat("-", 40))
			}

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
		if i != 0 {
			fmt.Printf("%s\n", strings.Repeat("-", 40))
		}
		fmt.Printf("Job[%d] : %s,  %s, %s\n", (i + 1), job.JobType, job.Type, targetDir)

		c.process(job)
	}
}

func (c CheckJob) process(job types.Target) {
	var fn docFunc
	var depth int

	switch job.Type {
	case GLOBAL:
		depth = 2
		fn = func(path string, size string) (v interface{}) {
			strs := strings.Split(path, "/")
			doc := types.ESDocGlobal{
				ESDoc: types.ESDoc{
					Timestamp:  time.Now(),
					Cluster:    c.config.ClusterName,
					VolumeType: "global",
					FullPath:   path,
					DiskSize:   size,
				},
				VolumeName: strs[len(strs)-1],
			}
			return doc
		}
	case PROJECT:
		depth = 4
		fn = func(path string, size string) (v interface{}) {
			strs := strings.Split(path, "/")
			doc := types.ESDocProject{
				ESDocGlobal: types.ESDocGlobal{
					ESDoc: types.ESDoc{
						Timestamp:  time.Now(),
						Cluster:    c.config.ClusterName,
						VolumeType: job.Type,
						FullPath:   path,
						DiskSize:   size,
					},
					VolumeName: strs[len(strs)-1],
				},
				ProjectName: strs[len(strs)-2],
			}
			return doc
		}
	case PERSONAL:
		depth = 2
		fn = func(path string, size string) (v interface{}) {
			strs := strings.Split(path, "/")
			doc := types.ESDocPersonal{
				ESDoc: types.ESDoc{
					Timestamp:  time.Now(),
					Cluster:    c.config.ClusterName,
					VolumeType: job.Type,
					FullPath:   path,
					DiskSize:   size,
				},
				UserName: strs[len(strs)-1],
			}
			return doc
		}
	default:
		return
	}

	var jsons = make([]interface{}, 0)

	if job.JobType == SINGLE_DIR {
		ret := c.execute(path.Join(c.config.MountDir, job.Location), job, fn)
		if ret != nil {
			jsons = append(jsons, ret)
		}
	} else {
		utils.WalkDirWithDepth(c.config.MountDir, job.Location, func(dir string, d fs.DirEntry, err error) error {
			if d.IsDir() && !contains(job.SkipDirs, strings.Replace(dir, path.Join(c.config.MountDir, job.Location)+"/", "", 1)) {
				ret := c.execute(dir, job, fn)
				if ret != nil {
					jsons = append(jsons, ret)
				}
			}
			return nil
		}, depth)
	}

	// elasticsearch에 결과 저장
	if len(jsons) > 0 {
		fmt.Printf("%s\n", strings.Repeat("-", 40))
		fmt.Printf("4. %d records saved to elasticsearch .\n", len(jsons))
		saveToElasticSearch(*c.config, jsons)
	}
}

func (c CheckJob) execute(path string, job types.Target, fn docFunc) interface{} {
	fmt.Printf("%s\n", strings.Repeat("-", 40))
	fmt.Printf("1. Start execute for '%s'\n", path)
	// 디렉토리가 존재하지 않으면 작업을 수행하지 않음
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Printf("1-1. target dir '%s' is not exist. Skip this dir.\n", path)
		return nil
	}

	// 데이터베이스 파일명 생성
	database := makeDBfileName(c.config.OutputDir, path)
	// nice 명령어 수행. 실패 시 스킵
	if err := runCommand(fmt.Sprintf(INDEX_FMT, path, database, 2)); err != nil {
		log.Print(err)
		return nil
	}

	// database 파일 생성 확인.
	if _, err := os.Stat(database); os.IsNotExist(err) {
		fmt.Printf("2. db file %s is not created. Skip this directory\n", database)
		return nil
	} else {
		fmt.Printf("2. database file of '%s' is generated.\n", database)
	}

	if bytes, err := runCommandGetStdOutBytes(fmt.Sprintf(XML_FMt, path, database)); err == nil {
		var ducRet types.DUC_RET
		xmlErr := xml.Unmarshal(bytes, &ducRet)
		// println(string(bytes))
		if xmlErr == nil {
			d := fn(strings.Replace(ducRet.Root, c.config.MountDir, "", 1), ducRet.Size)
			fmt.Printf("3. Process for %s finished successfully.\n", path)

			return d
		} else {
			log.Print(err)
		}
	} else {
		log.Print(err)
	}
	fmt.Printf("3. Process for %s failed. \n", path)
	return nil
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
