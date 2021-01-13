package main

import (
	"context"
	"errors"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
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

type Cmd struct {
	Id     int
	Script string
	Dir    string
	Spec   string
	Group  string
}

type RespCommon struct {
	Code int
	Msg  string
}

type RespAdd struct {
	RespCommon
}

type RespPing struct {
	RespCommon
}

type RespAddCmd struct {
	RespCommon
}

type RespRemoveCmd struct {
	RespCommon
}

type Job struct {
	Id     int    `json:"id"`
	Script string `json:"script"`
	Dir    string `json:"dir"`
	Spec   string `json:"spec"`
	Group  string `json:"group"`
	Prev   string `json:"prev"`
	Next   string `json:"next"`
	Pid    int    `json:"pid"`
	State  string `json:"state"`
}

type RespListCmd struct {
	RespCommon
	Data []Job
}

type ReqListAll struct {
	ClientName string `json:"client_name"`
}

type ReqAddCmd struct {
	Id     int    `json:"id" binding:"required"`
	Script string `json:"script" binding:"required"`
	Dir    string `json:"dir"`
	Spec   string `json:"spec" binding:"required"`
	Group  string `json:"group"`
}

type ReqRemoveCmd struct {
	Id int `json:"id" binding:"required"`
}

type Client struct {
	Uri     string
	Name    string
	Client  *rpc.Client
	Status  int
	ListCmd []Job
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

var Clients = make(map[string]Client)

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
		log.Fatalln("open log file failed")
	}
	Info = log.New(os.Stdout, "Info:", log.Ldate|log.Ltime|log.Lshortfile)
	Warning = log.New(os.Stdout, "Warning:", log.Ldate|log.Ltime|log.Lshortfile)
	Error = log.New(io.MultiWriter(os.Stderr, logFile), "Error:", log.Ldate|log.Ltime|log.Lshortfile)
}

func main() {
	server := new(Server)
	server.run()
	server.pingClient()

	gin.SetMode(ApiMode)
	r := gin.New()
	r.Use(cors.Default())

	r.POST("/api/cron/list_all", func(c *gin.Context) {
		var req ReqListAll
		if err := c.BindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code": CodeError,
				"msg":  MsgError,
			})
			return
		}
		res, err := ListAll(&req)
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

	r.POST("/api/cron/add", func(c *gin.Context) {
		var req ReqAddCmd
		if err := c.BindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code": CodeError,
			})
			return
		}
		res, err := AddCmd(&req)
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

	r.POST("/api/cron/remove", func(c *gin.Context) {
		var req ReqRemoveCmd
		if err := c.BindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"code": CodeError,
			})
			return
		}
		res, err := RemoveCmd(&req)
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
		Error.Println("Server register failed")
		return
	}
	rpc.HandleHTTP()
	listen, err := net.Listen("tcp", ServerUri)
	if err != nil {
		Error.Println("Server listen failed")
		return
	}
	go func() {
		if err = http.Serve(listen, nil); err != nil {
			Error.Println("Server failed")
		}
	}()
}

func (server *Server) Add(clientInfo ClientInfo, respAdd *RespAdd) error {
	conn, err := rpc.DialHTTP("tcp", clientInfo.Uri)
	if err != nil {
		Error.Println("Add client failed. client:", clientInfo.Name)
		return errors.New("add client failed")
	}
	Clients[clientInfo.Name] = Client{Uri: clientInfo.Uri, Name: clientInfo.Name, Client: conn, Status: OK}
	respAdd.Code = CodeSuccess
	respAdd.Msg = Success
	//Info.Println("Add client success. client:", clientInfo.Name)
	return nil
}

func (server *Server) pingClient() {
	go func() {
		for {
			time.Sleep(Interval)
			for _, client := range Clients {
				go func(client Client) {
					respPing := new(RespPing)
					ping := client.Client.Go("Client.Ping", "", respPing, nil)
					replyCall := <-ping.Done
					if replyCall.Error != nil || respPing.Code == CodeError {
						client.Status = ERR
						Error.Println("Ping failed. client:", client.Name, replyCall.Error)
						go func(client Client) {
							respAdd := new(RespAdd)
							add := client.Client.Go("Client.Add", "", respAdd, nil)
							replyCall := <-add.Done
							if replyCall.Error != nil || respAdd.Code == CodeError {
								client.Status = ERR
								Error.Println("Add client failed. client:", client.Name, replyCall.Error)
								return
							}
							client.Status = OK
							Info.Println("Add client success. client:", client.Name)
						}(client)
						return
					}
					//Info.Println("Ping ok. client:", client.Name)
				}(client)
			}
		}
	}()
}

func ListAll(req *ReqListAll) (resp map[string][]Job, err error) {
	var wg sync.WaitGroup
	resp = make(map[string][]Job)
	for clientName, client := range Clients {
		if req.ClientName != "" && req.ClientName != clientName {
			continue
		}
		respListCmd := new(RespListCmd)
		listCmd := client.Client.Go("Client.ListCmd", "", respListCmd, nil)
		replyCall := <-listCmd.Done
		if replyCall.Error != nil || respListCmd.Code == CodeError {
			Error.Println("Client list failed. client:", client.Name, replyCall.Error)
			continue
		}
		client.ListCmd = respListCmd.Data
		resp[clientName] = respListCmd.Data
		Info.Println("Client list success. client:", client.Name)
	}
	wg.Wait()
	return resp, nil
}

func AddCmd(req *ReqAddCmd) (resp map[string]bool, err error) {
	var wg sync.WaitGroup
	resp = make(map[string]bool)
	cmd := Cmd{
		Id:     req.Id,
		Script: req.Script,
		Dir:    req.Dir,
		Spec:   req.Spec,
		Group:  req.Group,
	}
	for clientName, client := range Clients {
		respAddCmd := new(RespAddCmd)
		addCmd := client.Client.Go("Client.AddCmd", cmd, respAddCmd, nil)
		replyCall := <-addCmd.Done
		if replyCall.Error != nil || respAddCmd.Code == CodeError {
			resp[clientName] = false
			Error.Println("Client add failed. client:", client.Name, replyCall.Error)
			continue
		}
		resp[clientName] = true
		Info.Println("Client add success. client:", client.Name)
	}
	wg.Wait()
	return resp, nil
}

func RemoveCmd(req *ReqRemoveCmd) (resp map[string]bool, err error) {
	var wg sync.WaitGroup
	resp = make(map[string]bool)
	cmd := Cmd{
		Id: req.Id,
	}
	for clientName, client := range Clients {
		respRemoveCmd := new(RespRemoveCmd)
		removeCmd := client.Client.Go("Client.RemoveCmd", cmd, respRemoveCmd, nil)
		replyCall := <-removeCmd.Done
		if replyCall.Error != nil || respRemoveCmd.Code == CodeError {
			resp[clientName] = false
			Error.Println("Client remove failed. client:", client.Name, replyCall.Error)
			continue
		}
		resp[clientName] = true
		Info.Println("Client remove success. client:", client.Name)
	}
	wg.Wait()
	return resp, nil
}
