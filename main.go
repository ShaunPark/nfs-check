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
		fmt.Printf("\nJob[%d] : %s,  %s, %s\n", (i + 1), job.JobType, job.Type, targetDir)

		if _, err := os.Stat(targetDir); os.IsNotExist(err) {
			fmt.Printf("0. directory %s is not exist.\n", targetDir)
			continue
		} else {
			fmt.Printf("0. directory %s is exist. Go next step.\n", targetDir)
		}

		switch job.Type {
		case "global":
			c.processGlobal(targetDir, job)
		case "personal":
			c.processPersonal(targetDir, job)
		case "project":
			c.processProject(targetDir, job)
		}
	}
}

func makeDBfileName(outDir string, fPath string) string {
	now := time.Now()
	return fmt.Sprintf("%s/%s.%d.db", outDir, strings.ReplaceAll(fPath, "/", "."), now.Unix())
}

func (c CheckJob) processGlobal(tgtDir string, job types.Target) {
	fmt.Println("1. Start processGlobal")
	// 대상 dir 목록 생성
	dirs := []string{}
	// subDirs이면 하위 디렉토리를 조회해서 목록생성
	if job.JobType == "subDirs" {
		if files, err := ioutil.ReadDir(tgtDir); err != nil {
			fmt.Printf("1-1. Getting sub directories of '%s' failed. Skip this dir.\n", tgtDir)
			return
		} else {
			for _, f := range files {
				if f.IsDir() && !contains(job.SkipDirs, f.Name()) {
					dirs = append(dirs, tgtDir+"/"+f.Name())
				}
			}
		}
	} else if job.JobType == "singleDir" {
		// singleDir이면 단일 디렉토리를 목록에 추가
		// 디렉토리가 존재하지 않으면 작업을 수행하지 않음
		if _, err := os.Stat(tgtDir); os.IsNotExist(err) {
			fmt.Printf("1-1. target dir '%s' is not exist. Skip this dir.\n", tgtDir)
			return
		} else {
			dirs = append(dirs, tgtDir)
		}
	} else {
		fmt.Printf("1-1. Invalid JobType [%s]. Skip this job", job.JobType)
		return
	}
	fmt.Printf("2. %d directories will be processed.\n", len(dirs))

	ret := make([]interface{}, 0)

	// 조회된 dir들에 대해서 수행
	for i, dir := range dirs {
		fPath := strings.Replace(dir, c.config.MountDir+"/", "", 1)
		// 데이터베이스 파일명 생성
		database := makeDBfileName(c.config.OutputDir, fPath)
		// 대상 디렉토리가 있는지 확인 없으면 스킵
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			fmt.Printf("3-%d. ir %s is not exist. Skip this job.\n", i, dir)
			continue
		}
		// nice 명령어 수행. 실패 시 스킵
		if err := runCommand("nice", fmt.Sprintf(niceArgsFmt, dir, database)); err != nil {
			log.Print(err)
			continue
		}

		// database 파일 생성 확인.
		if _, err := os.Stat(database); os.IsNotExist(err) {
			fmt.Printf("3-%d. db file %s is not created. Skip this directory\n", i, database)
			continue
		} else {
			fmt.Printf("3-%d. database file of '%s' is generated.\n", i, database)
		}
		// duc 명령어 수행
		if err := runCommandWithFunc("duc", fmt.Sprintf(ducArgsFmtGlobal, dir, database), ret, func(input string, r []interface{}) {
			strs := strings.Fields(input)
			if strs[0] != "Date" {
				ss := strings.Split(strs[5], "/")

				doc := types.ESDocGlobal{
					ESDoc: types.ESDoc{
						Timestamp:  time.Now(),
						Cluster:    c.config.ClusterName,
						VolumeType: "global",
						FullPath:   "/" + fPath,
						DiskSize:   strs[4],
					},
					VolumeName: ss[len(ss)-1],
				}
				ret = append(ret, doc)
			}
		}); err != nil {
			log.Print(err)
			continue
		}
		fmt.Printf("4-%d. Process for '%s' is finished .\n", i, dir)
	}

	// elasticsearch에 결과 저장
	if len(ret) > 0 {
		saveToElasticSearch(*c.config, ret)
		fmt.Printf("5. %d record saved to elasticsearch .\n", len(ret))
	}
}

func (c CheckJob) processProject(tgtDir string, job types.Target) {
	fmt.Println("1. Start processProject")
	// database 파일명 생성
	database := makeDBfileName(c.config.OutputDir, job.Location)
	// target dir 유무 확인
	if _, err := os.Stat(tgtDir); os.IsNotExist(err) {
		fmt.Printf("1-1. dir %s is not exist. Skip this job.\n", tgtDir)
		return
	}
	// nice 명령어 실행
	if err := runCommand("nice", fmt.Sprintf(niceArgsFmt, tgtDir, database)); err != nil {
		log.Print(err)
		return
	}
	// dababase 파일이 생성되지 않으면 종료
	if _, err := os.Stat(database); os.IsNotExist(err) {
		fmt.Printf("2. database file of '%s' is not created. Skip this job.\n", database)
		return
	} else {
		fmt.Printf("2. database file of '%s' is generated.\n", database)
	}
	// duc 명령어 실행
	ret := make([]interface{}, 0)
	if err := runCommandWithFunc("duc", fmt.Sprintf(ducArgsFmtProject, tgtDir, database), ret, func(input string, r []interface{}) {
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
	}); err != nil {
		log.Print(err)
		return
	}
	fmt.Printf("3. Process for '%s' is finished .\n", tgtDir)

	// elasticsearch에 결과 저장
	if len(ret) > 0 {
		saveToElasticSearch(*c.config, ret)
		fmt.Printf("4. %d record saved to elasticsearch .\n", len(ret))
	}
}

func (c CheckJob) processPersonal(tgtDir string, job types.Target) {
	fmt.Println("1. Start processPersonal")
	// database 파일명 생성
	database := makeDBfileName(c.config.OutputDir, job.Location)
	if _, err := os.Stat(tgtDir); os.IsNotExist(err) {
		fmt.Printf("1-1. dir %s is not exist. Skip this job.\n", tgtDir)
		return
	}
	// nice 명령어 실행
	if err := runCommand("nice", fmt.Sprintf(niceArgsFmt, tgtDir, database)); err != nil {
		log.Print(err)
		return
	}
	// dababase 파일이 생성되지 않으면 종료
	if _, err := os.Stat(database); os.IsNotExist(err) {
		fmt.Printf("2. database file of '%s' is not created. Skip this job.\n", database)
		return
	} else {
		fmt.Printf("2. database file of '%s' is generated.\n", database)
	}

	// duc 명령어 실행
	ret := make([]interface{}, 0)
	if err := runCommandWithFunc("duc", fmt.Sprintf(ducArgsFmtPersonal, tgtDir, database), ret, func(input string, r []interface{}) {
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
	}); err != nil {
		log.Print(err)
		return
	}
	fmt.Printf("3. Process for '%s' is finished .\n", tgtDir)

	// elasticsearch에 결과 저장
	if len(ret) > 0 {
		saveToElasticSearch(*c.config, ret)
		fmt.Printf("4. %d record saved to elasticsearch .\n", len(ret))
	}
}

func runCommand(c string, args string) error {
	cmd := exec.Command(c, strings.Fields(args)...)
	if err := cmd.Start(); err != nil {
		return err
	}
	defer cmd.Wait()
	return nil
}

func runCommandWithFunc(c string, args string, r []interface{}, f func(t string, r []interface{})) error {
	cmd := exec.Command(c, strings.Fields(args)...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	buf := bufio.NewScanner(out)
	buf2 := bufio.NewScanner(stderr)
	if err := cmd.Start(); err != nil {
		return err
	}

	for buf.Scan() {
		f(buf.Text(), r)
	}

	for buf2.Scan() {
		fmt.Println(buf2.Text())
	}
	defer cmd.Wait()

	return nil
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
	bytes, _ := json.Marshal(v)
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
