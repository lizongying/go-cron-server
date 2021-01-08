package main

import (
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"time"
)

type Cmd struct {
	Id     int
	Script string
	Dir    string
	Spec   string
	Server string
	Enable bool
}

type RespAdd struct {
	Code int
	Msg  string
}

type RespPing struct {
	Code int
	Msg  string
}

type RespAddCmd struct {
	Code int
	Msg  string
}

type RespRemoveCmd struct {
	Code int
	Msg  string
}

type Job struct {
	Script string
}

type RespListCmd struct {
	Code int
	Msg  string
	Data []Job
}

var OK = 1
var ERR = 0
var ServerUri = "127.0.0.1:1234"
var PING = time.Second
var CodeSuccess = 0
var Success = "success"

type Client struct {
	Uri     string
	Client  *rpc.Client
	Status  int
	ListCmd []Job
}

type Server struct {
	Clients map[string]Client
}

var Clients = make(map[string]Client)

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

func (server *Server) Add(uri string, respAdd *RespAdd) error {
	conn, err := rpc.DialHTTP("tcp", uri)
	if err != nil {
		Error.Println("Add client failed")
		return errors.New("add client failed")
	}
	Clients[uri] = Client{Uri: uri, Client: conn, Status: OK}
	server.Clients = Clients
	respAdd.Msg = Success
	return nil
}

func (server *Server) ping() {
	for {
		time.Sleep(PING)
		for _, client := range server.Clients {
			go func(client Client) {
				respPing := new(RespPing)
				ping := client.Client.Go("Client.Ping", "", respPing, nil)
				replyCall := <-ping.Done
				if replyCall.Error != nil {
					client.Status = ERR
					Error.Println("Ping failed:", client.Uri, replyCall.Error)
					go func(client Client) {
						respAdd := new(RespAdd)
						add := client.Client.Go("Client.Add", "", respAdd, nil)
						replyCall := <-add.Done
						if replyCall.Error != nil {
							client.Status = ERR
							Error.Println("Add failed:", client.Uri, replyCall.Error)
							return
						}
						Info.Println(respPing.Msg)
						if respAdd.Code == CodeSuccess {
							client.Status = OK
						}
						Info.Println("add success")
					}(client)
					return
				}
				Info.Println(respPing.Msg)
			}(client)
		}
	}
}

var (
	Info    *log.Logger
	Warning *log.Logger
	Error   *log.Logger
)

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
	server := new(Server)
	server.run()
	server.ping()
	//cmd := &Cmd{Id: 1, Script: "sleep 100", Spec: "* * * * *"}
}
func AddCmd(client Client) {
	respAddCmd := new(RespAddCmd)
	addCmd := client.Client.Go("Client.AddCmd", nil, respAddCmd, nil)
	replyCall := <-addCmd.Done
	if replyCall.Error != nil {
		Error.Println("Add failed:", client.Uri, replyCall.Error)
		return
	}
	Info.Println(respAddCmd.Msg)
	if respAddCmd.Code == CodeSuccess {
		Info.Println("Add success:", client.Uri)
	}
	Info.Println("Add success:", client.Uri)
}

func RemoveCmd(client Client) {
	respRemoveCmd := new(RespRemoveCmd)
	removeCmd := client.Client.Go("Client.RemoveCmd", nil, respRemoveCmd, nil)
	replyCall := <-removeCmd.Done
	if replyCall.Error != nil {
		Error.Println("Remove failed:", client.Uri, replyCall.Error)
		return
	}
	Info.Println(respRemoveCmd.Msg)
	if respRemoveCmd.Code == CodeSuccess {
		Info.Println("Remove success:", client.Uri)
	}
	Info.Println("Remove success:", client.Uri)
}

func ListCmd(client Client) {
	respListCmd := new(RespListCmd)
	listCmd := client.Client.Go("Client.ListCmd", nil, respListCmd, nil)
	replyCall := <-listCmd.Done
	if replyCall.Error != nil {
		Error.Println("List failed:", client.Uri, replyCall.Error)
		return
	}
	Info.Println(respListCmd.Msg)
	if respListCmd.Code == CodeSuccess {
		client.ListCmd = respListCmd.Data
		Info.Println("List success:", client.ListCmd)
	}
	Info.Println("List success:", client.ListCmd)
}
