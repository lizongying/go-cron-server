package main

import (
	"context"
	"errors"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
	"go-cron-server/app"
	"io"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type RespCommon struct {
	Code int
	Msg  string
}

type RespClientAdd struct {
	RespCommon
}

type RespClientPing struct {
	RespCommon
}

type Job struct {
	Id        int          `json:"id"`
	Name      string       `json:"name"`
	Project   string       `json:"project"`
	Creator   string       `json:"creator"`
	CreatTime string       `json:"creat_time"`
	Enable    bool         `json:"enable"`
	Server    string       `json:"server"`
	Script    string       `json:"script"`
	Dir       string       `json:"dir"`
	Spec      string       `json:"spec"`
	Group     string       `json:"group"`
	Prev      string       `json:"prev"`
	Next      string       `json:"next"`
	Pid       int          `json:"pid"`
	State     string       `json:"state"`
	EntryID   cron.EntryID `json:"-"`
	Md5       string       `json:"-"`
}

type RespJobList struct {
	RespCommon
	Data []Job
}

type RespJobAdd struct {
	RespCommon
}

type RespJobRemove struct {
	RespCommon
}

type ReqJobList struct {
	Server string `json:"server"`
}

type ReqJobAdd struct {
	Id        int    `json:"id" binding:"required"`
	Name      string `json:"name" binding:"required"`
	Project   string `json:"project" binding:"required"`
	Creator   string `json:"creator" binding:"required"`
	CreatTime string `json:"creat_time" binding:"required"`
	Server    string `json:"server"`
	Script    string `json:"script" binding:"required"`
	Dir       string `json:"dir"`
	Spec      string `json:"spec" binding:"required"`
	Group     string `json:"group"`
}

type ReqJobRemove struct {
	Id     int    `json:"id" binding:"required"`
	Server string `json:"server"`
}

type ReqJobStart struct {
	Id     int    `json:"id" binding:"required"`
	Server string `json:"server"`
}

type ReqJobStop struct {
	Id     int    `json:"id" binding:"required"`
	Server string `json:"server"`
}

type Client struct {
	Uri     string
	Name    string
	Client  *rpc.Client
	Status  int
	JobList map[int]*Job
}

type ClientInfo struct {
	Uri   string `yaml:"uri" json:"-"`
	Name  string `yaml:"name" json:"-"`
	Group string `yaml:"group" json:"-"`
}

type Server struct{}

var OK = 1
var ERR = 0
var ServerUri = "127.0.0.1:1234"
var Interval = time.Second
var CodeSuccess = 0
var CodeError = 1
var Success = "success"
var MsgError = "error"
var ApiUri = ""
var ApiMode = ""
var Collection = ""

var Clients = make(map[string]*Client)

var (
	Info    *log.Logger
	Warning *log.Logger
	Error   *log.Logger
)

func init() {
	app.InitConfig()
	mongo := app.Conf.Mongo
	Collection = mongo.Collection
	if mongo.Uri != "" {
		app.InitMongo(mongo)
	}
	ServerUri = app.Conf.Server.Uri
	ApiUri = app.Conf.Api.Uri
	ApiMode = app.Conf.Api.Mode
	Interval = time.Duration(app.Conf.Server.Interval) * time.Second
	logFile, err := os.OpenFile(app.Conf.Log.Filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalln("open log file failed.", err)
	}
	Info = log.New(os.Stdout, "Info:", log.Ldate|log.Ltime|log.Lshortfile)
	Warning = log.New(os.Stdout, "Warning:", log.Ldate|log.Ltime|log.Lshortfile)
	Error = log.New(io.MultiWriter(os.Stderr, logFile), "Error:", log.Ldate|log.Ltime|log.Lshortfile)
}

func main() {
	server := new(Server)
	server.run()
	server.clientPing()

	gin.SetMode(ApiMode)
	r := gin.New()
	r.Use(cors.Default())
	r.StaticFile("/", "./dist/index.html")
	r.StaticFile("/favicon.ico", "./dist/favicon.ico")
	r.Static("/static", "./dist/static")

	r.POST("/api/job/list", func(c *gin.Context) {
		var req ReqJobList
		if err := c.BindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code": CodeError,
				"msg":  MsgError,
			})
			return
		}
		res, err := JobList(&req)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code": CodeError,
				"msg":  MsgError,
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code": CodeSuccess,
			"data": res,
		})
	})

	r.POST("/api/job/add", func(c *gin.Context) {
		var req ReqJobAdd
		if err := c.BindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code": CodeError,
			})
			return
		}
		res, err := JobAdd(&req)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code": CodeError,
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code": CodeSuccess,
			"data": res,
		})
	})

	r.POST("/api/job/remove", func(c *gin.Context) {
		var req ReqJobRemove
		if err := c.BindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code": CodeError,
			})
			return
		}
		res, err := JobRemove(&req)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code": CodeError,
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code": CodeSuccess,
			"data": res,
		})
	})

	r.POST("/api/job/start", func(c *gin.Context) {
		var req ReqJobStart
		if err := c.BindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code": CodeError,
			})
			return
		}
		res, err := JobStart(&req)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code": CodeError,
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code": CodeSuccess,
			"data": res,
		})
	})

	r.POST("/api/job/stop", func(c *gin.Context) {
		var req ReqJobStop
		if err := c.BindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code": CodeError,
			})
			return
		}
		res, err := JobStop(&req)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code": CodeError,
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code": CodeSuccess,
			"data": res,
		})
	})

	if err := r.Run(ApiUri); err != nil {
		log.Fatalln(err)
	}

	srv := &http.Server{
		Addr:    ApiUri,
		Handler: r,
	}

	// Initializing the server in a goroutine so that
	// it won't block the graceful shutdown handling below
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server with
	// a timeout of 5 seconds.
	quit := make(chan os.Signal)
	// kill (no param) default send syscall.SIGTERM
	// kill -2 is syscall.SIGINT
	// kill -9 is syscall.SIGKILL but can't be catch, so don't need add it
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
}

func (server *Server) run() {
	if err := rpc.Register(server); err != nil {
		Error.Println("Server register failed.", err)
		return
	}
	rpc.HandleHTTP()
	listen, err := net.Listen("tcp", ServerUri)
	if err != nil {
		Error.Println("Server listen failed.", err)
		return
	}
	go func() {
		if err = http.Serve(listen, nil); err != nil {
			Error.Println("Server failed.", err)
		}
	}()
}

func (server *Server) ClientAdd(clientInfo ClientInfo, respClientAdd *RespClientAdd) error {
	if Clients[clientInfo.Name] != nil && Clients[clientInfo.Name].Status == OK {
		respClientAdd.Code = CodeSuccess
		respClientAdd.Msg = Success
		//Info.Println("Client add success.")
		return nil
	}
	conn, err := rpc.DialHTTP("tcp", clientInfo.Uri)
	if err != nil {
		Error.Println("Client add failed.", err)
		return errors.New("client add failed")
	}
	if Clients[clientInfo.Name] == nil {
		Clients[clientInfo.Name] = &Client{Uri: clientInfo.Uri, Name: clientInfo.Name, Client: conn, Status: OK, JobList: make(map[int]*Job)}
	} else {
		Clients[clientInfo.Name] = &Client{Uri: clientInfo.Uri, Name: clientInfo.Name, Client: conn, Status: OK, JobList: Clients[clientInfo.Name].JobList}
		for _, job := range Clients[clientInfo.Name].JobList {
			if !job.Enable {
				continue
			}
			req := ReqJobAdd{
				Id:        job.Id,
				Name:      job.Name,
				Project:   job.Project,
				Creator:   job.Creator,
				CreatTime: job.CreatTime,
				Server:    job.Server,
				Script:    job.Script,
				Dir:       job.Dir,
				Spec:      job.Spec,
				Group:     job.Group,
			}
			_, err := JobAdd(&req)
			if err != nil {
				Error.Println("Job add failed.", err)
			}
		}
	}
	respClientAdd.Code = CodeSuccess
	respClientAdd.Msg = Success
	Info.Println("Client add success.")
	return nil
}

func (server *Server) clientPing() {
	go func() {
		for {
			time.Sleep(Interval)
			for _, client := range Clients {
				go func(client *Client) {
					respClientPing := new(RespClientPing)
					clientPing := client.Client.Go("Client.ClientPing", "Server", respClientPing, nil)
					replyCall := <-clientPing.Done
					if replyCall.Error != nil || respClientPing.Code == CodeError {
						client.Status = ERR
						Error.Println("Ping failed.", replyCall.Error)
						go func(client *Client) {
							respClientAdd := new(RespClientAdd)
							clientAdd := client.Client.Go("Client.ClientAdd", "Server", respClientAdd, nil)
							replyCall := <-clientAdd.Done
							if replyCall.Error != nil || respClientAdd.Code == CodeError {
								client.Status = ERR
								Error.Println("Client add failed.", replyCall.Error)
								return
							}
							client.Status = OK
							Info.Println("Client add success.")
						}(client)
						return
					}
					//Info.Println("Ping ok.")
				}(client)
			}
		}
	}()
}

func (server *Server) ClientPing(args string, respClientPing *RespClientPing) error {
	//Info.Println("Ping ok.")
	respClientPing.Code = CodeSuccess
	respClientPing.Msg = Success
	return nil
}

func JobList(req *ReqJobList) (resp map[string][]Job, err error) {
	var wg sync.WaitGroup
	resp = make(map[string][]Job)
	for server, client := range Clients {
		if req.Server != "" && req.Server != server {
			continue
		}
		respJobList := new(RespJobList)
		jobList := client.Client.Go("Client.JobList", "", respJobList, nil)
		replyCall := <-jobList.Done
		if replyCall.Error != nil || respJobList.Code == CodeError {
			Error.Println("Job list failed.", replyCall.Error)
			continue
		}
		for i, data := range respJobList.Data {
			client.JobList[data.Id] = &respJobList.Data[i]
		}
		for _, data := range client.JobList {
			resp[server] = append(resp[server], *data)
		}
		Info.Println("Job list success.")
	}
	wg.Wait()
	return resp, nil
}

func JobAdd(req *ReqJobAdd) (resp map[string]bool, err error) {
	var wg sync.WaitGroup
	resp = make(map[string]bool)
	job := Job{
		Id:     req.Id,
		Script: req.Script,
		Dir:    req.Dir,
		Spec:   req.Spec,
		Group:  req.Group,
	}
	for server, client := range Clients {
		if req.Server != "" && req.Server != server {
			continue
		}
		respJobAdd := new(RespJobAdd)
		jobAdd := client.Client.Go("Client.JobAdd", job, respJobAdd, nil)
		replyCall := <-jobAdd.Done
		if replyCall.Error != nil || respJobAdd.Code == CodeError {
			resp[server] = false
			Error.Println("Job add failed.", replyCall.Error)
			continue
		}
		resp[server] = true
		if Clients[server].JobList[req.Id] == nil {
			Clients[server].JobList[req.Id] = &Job{
				Id:        req.Id,
				Name:      req.Name,
				Project:   req.Project,
				Creator:   req.Creator,
				CreatTime: req.CreatTime,
				Enable:    true,
				Server:    req.Server,
				Script:    req.Script,
				Dir:       req.Dir,
				Spec:      req.Spec,
				Group:     req.Group,
			}
		}
		Info.Println("Job add success.")
	}
	wg.Wait()
	return resp, nil
}

func JobRemove(req *ReqJobRemove) (resp map[string]bool, err error) {
	var wg sync.WaitGroup
	resp = make(map[string]bool)
	job := Job{
		Id: req.Id,
	}
	for server, client := range Clients {
		if req.Server != "" && req.Server != server {
			continue
		}
		respJobRemove := new(RespJobRemove)
		jobRemove := client.Client.Go("Client.JobRemove", job, respJobRemove, nil)
		replyCall := <-jobRemove.Done
		if replyCall.Error != nil || respJobRemove.Code == CodeError {
			resp[server] = false
			delete(Clients[server].JobList, req.Id)
			Error.Println("Job remove failed.", replyCall.Error)
			continue
		}
		resp[server] = true
		delete(Clients[server].JobList, req.Id)
		Info.Println("Job remove success.")
	}
	wg.Wait()
	return resp, nil
}

func JobStart(req *ReqJobStart) (resp map[string]bool, err error) {
	var wg sync.WaitGroup
	resp = make(map[string]bool)
	for server, client := range Clients {
		var job Job
		if req.Server != "" && req.Server != server {
			continue
		}
		for id, ii := range Clients[server].JobList {
			if id != req.Id {
				continue
			}
			job = Job{
				Id:     req.Id,
				Script: ii.Script,
				Dir:    ii.Dir,
				Spec:   ii.Spec,
				Group:  ii.Group,
			}
			break
		}
		if job.Id == 0 {
			continue
		}
		respJobAdd := new(RespJobAdd)
		jobAdd := client.Client.Go("Client.JobAdd", job, respJobAdd, nil)
		replyCall := <-jobAdd.Done
		if replyCall.Error != nil || respJobAdd.Code == CodeError {
			resp[server] = false
			Error.Println("Job add failed.", replyCall.Error)
			continue
		}
		resp[server] = true
		Clients[server].JobList[req.Id].Enable = true
		Info.Println("Job add success.")
	}
	wg.Wait()
	return resp, nil
}

func JobStop(req *ReqJobStop) (resp map[string]bool, err error) {
	var wg sync.WaitGroup
	resp = make(map[string]bool)
	job := Job{
		Id: req.Id,
	}
	for server, client := range Clients {
		if req.Server != "" && req.Server != server {
			continue
		}
		respJobRemove := new(RespJobRemove)
		jobRemove := client.Client.Go("Client.JobRemove", job, respJobRemove, nil)
		replyCall := <-jobRemove.Done
		if replyCall.Error != nil || respJobRemove.Code == CodeError {
			resp[server] = false
			Error.Println("Job remove failed.", replyCall.Error)
			continue
		}
		resp[server] = true
		Clients[server].JobList[req.Id].Enable = false
		Info.Println("Job remove success.")
	}
	wg.Wait()
	return resp, nil
}
