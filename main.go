package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"github.com/robfig/cron/v3"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Task struct {
	EntryID cron.EntryID
	Pid     int
	Md5     string
}
type Cmd struct {
	Id     int
	Script string
	Dir    string
	Spec   string
	Server string
	Enable bool
}

var TaskMap = make(map[int]*Task, 0)

var (
	Info    *log.Logger
	Warning *log.Logger
	Error   *log.Logger
)

var Interval = time.Minute

func init() {
	logFile, err := os.OpenFile("cron.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalln("open log file failed")
	}
	Info = log.New(os.Stdout, "Info:", log.Ldate|log.Ltime|log.Lshortfile)
	Warning = log.New(os.Stdout, "Warning:", log.Ldate|log.Ltime|log.Lshortfile)
	Error = log.New(io.MultiWriter(os.Stderr, logFile), "Error:", log.Ldate|log.Ltime|log.Lshortfile)
}

func main() {
	run()
	select {}
}
func run() {
	c := cron.New()
	c.Start()
	for {
		conf, err := ioutil.ReadFile("cron.json")
		if err != nil {
			Error.Println("read config file failed")
			time.Sleep(Interval)
			continue
		}
		var confList []struct {
			Id     int    `json:"id"`
			Script string `json:"script"`
			Dir    string `json:"dir"`
			Spec   string `json:"spec"`
			Server string `json:"server"`
			Enable bool   `json:"enable"`
		}
		if err := json.Unmarshal(conf, &confList); err != nil {
			Error.Println(err)
		}
		var cmdList []Cmd
		for _, cmd := range confList {
			cmdList = append(cmdList, Cmd{
				Id:     cmd.Id,
				Script: cmd.Script,
				Dir:    cmd.Dir,
				Spec:   cmd.Spec,
				Server: cmd.Server,
				Enable: cmd.Enable,
			})
		}
		for _, cmd := range cmdList {
			go func(cmd Cmd) {
				taskId := cmd.Id
				script := cmd.Script
				dir := cmd.Dir
				spec := cmd.Spec
				server := cmd.Server
				enable := cmd.Enable
				if enable {
					if TaskMap[taskId] == nil {
						TaskMap[taskId] = &Task{}
					}
					taskMd5 := fmt.Sprintf("%x", md5.Sum([]byte(script+dir+spec+server)))
					entryID := TaskMap[taskId].EntryID
					if entryID > 0 {
						//Info.Println("cmd is in cron:", cmd)
						//修改任务
						if TaskMap[taskId].Md5 != taskMd5 {
							entryIDOld := entryID
							entryID, _ = c.AddFunc(spec, func() {
								execScript(cmd)
							})
							if entryID == 0 {
								Info.Println("add cmd failed:", cmd)
								return
							}
							TaskMap[taskId].Md5 = taskMd5
							TaskMap[taskId].EntryID = entryID
							Info.Println("add cmd from cron:", cmd)
							c.Remove(entryIDOld)
							Info.Println("remove cmd from cron:", cmd)
						}
						return
					}
					//增加任务
					entryID, _ = c.AddFunc(spec, func() {
						execScript(cmd)
					})
					if entryID == 0 {
						delete(TaskMap, taskId)
						Info.Println("add cmd failed:", cmd)
						return
					}
					TaskMap[taskId].Md5 = taskMd5
					TaskMap[taskId].EntryID = entryID
					Info.Println("add cmd to cron:", cmd)
				} else {
					//删除任务
					if TaskMap[taskId] == nil {
						//Info.Println("cmd is not in cron:", cmd)
						return
					}
					entryID := TaskMap[taskId].EntryID
					if entryID == 0 {
						//Info.Println("cmd is not in cron:", cmd)
						return
					}
					c.Remove(entryID)
					delete(TaskMap, taskId)
					Info.Println("remove cmd from cron:", cmd)
				}
			}(cmd)
		}
		time.Sleep(Interval)
	}
}

func execScript(cmd Cmd) {
	taskId := cmd.Id
	script := cmd.Script
	dir := cmd.Dir
	server := cmd.Server
	if server != "" {
		script = fmt.Sprintf("ssh %s %s", server, script)
	}
	pid := TaskMap[taskId].Pid
	if pid > 0 {
		s, err := infoScript(pid)
		if err == nil && len(s) > 0 {
			switch s[0:1] {
			case "R",
				"S":
				//Info.Println("cmd is in process:", cmd)
				return
			}
		}
	}
	s := strings.Split(script, " ")
	shell := exec.Command(s[0], s[1:]...)
	if dir != "" {
		shell.Dir = dir
	}
	err := shell.Start()
	if err != nil {
		Error.Println("cmd run failed:", cmd)
		return
	}
	pid = shell.Process.Pid
	TaskMap[taskId].Pid = pid
	Info.Println(pid, shell)
}

func infoScript(pid int) (string, error) {
	shell := exec.Command("ps", "h", "-o", "stat", "-p", strconv.Itoa(pid))
	out, err := shell.Output()
	s := string(out)
	s = strings.Replace(s, "STAT", "", -1)
	s = strings.Replace(s, "\n", "", -1)
	s = strings.Replace(s, " ", "", -1)
	return s, err
}
