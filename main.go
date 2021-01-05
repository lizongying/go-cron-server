package main

import (
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
}
type Cmd struct {
	Script string
	Dir    string
	Spec   string
	Server string
	Enable bool
}

var TaskMap = make(map[string]*Task, 0)

var (
	Info    *log.Logger
	Warning *log.Logger
	Error   *log.Logger
)

func init() {
	errFile, err := os.OpenFile("go_cron.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalln("open log file failed")
	}
	Info = log.New(os.Stdout, "Info:", log.Ldate|log.Ltime|log.Lshortfile)
	Warning = log.New(os.Stdout, "Warning:", log.Ldate|log.Ltime|log.Lshortfile)
	Error = log.New(io.MultiWriter(os.Stderr, errFile), "Error:", log.Ldate|log.Ltime|log.Lshortfile)
}

func main() {
	run()
	select {}
}
func run() {
	c := cron.New()
	c.Start()
	for {
		conf, err := ioutil.ReadFile("go_cron.json")
		if err != nil {
			Error.Println("read config file failed")
			time.Sleep(10 * time.Second)
			continue
		}
		var confList []struct {
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
			cmdList = append(cmdList, Cmd{Script: cmd.Script, Dir: cmd.Dir, Spec: cmd.Spec, Server: cmd.Server, Enable: cmd.Enable})
		}
		for _, cmd := range cmdList {
			go func(cmd Cmd) {
				script := cmd.Script
				server := cmd.Server
				spec := cmd.Spec
				if server != "" {
					script = fmt.Sprintf("ssh %s %s", server, script)
				}
				script = strings.Trim(script, " ")
				if script == "" {
					//Warning.Println("script is empty:", script)
					return
				}
				dir := cmd.Dir
				enable := cmd.Enable
				if enable {
					if TaskMap[script] != nil && TaskMap[script].EntryID > 0 {
						//Info.Println("script is in cron:", script)
						return
					}
					entryID, _ := c.AddFunc(spec, func() {
						execScript(script, dir)
					})
					if TaskMap[script] == nil {
						TaskMap[script] = &Task{}
					}
					TaskMap[script].EntryID = entryID
					Info.Println("add script to cron:", script)
				} else {
					if TaskMap[script] == nil {
						//Info.Println("script is not in cron:", script)
						return
					}
					entryID := TaskMap[script].EntryID
					if entryID == 0 {
						//Info.Println("script is not in cron")
						return
					}
					c.Remove(entryID)
					delete(TaskMap, "script")
					Info.Println("remove script from cron:", script)
				}
			}(cmd)
		}
		time.Sleep(time.Minute)
	}
}

func execScript(script string, dir string) {
	pid := TaskMap[script].Pid
	if pid > 0 {
		pids := []string{"h", "-o", "stat", "-p", strconv.Itoa(pid)}
		s, err := infoScript(pids...)
		if err == nil && len(s) > 0 {
			switch s[0:1] {
			case "R",
				"S":
				//Info.Println("script is in process:", script)
				return
			}
		}
	}
	s := strings.Split(script, " ")
	cmd := exec.Command(s[0], s[1:]...)
	if dir != "" {
		cmd.Dir = dir
	}
	err := cmd.Start()
	if err != nil {
		Error.Println(script, err.Error())
		return
	}
	pid = cmd.Process.Pid
	TaskMap[script].Pid = pid
	Info.Println(pid, cmd)
}

func infoScript(pids ...string) (string, error) {
	ps := exec.Command("ps", pids...)
	out, err := ps.Output()
	s := string(out)
	s = strings.Replace(s, "STAT", "", -1)
	s = strings.Replace(s, "\n", "", -1)
	s = strings.Replace(s, " ", "", -1)
	return s, err
}
