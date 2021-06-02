package main

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/oshokin/alarm-button/entities"
)

const (
	serverBufferSize          uint          = 1024
	serverFileLogMaxAge       time.Duration = 24 * time.Hour
	serverFileLogRotationTime time.Duration = time.Hour
)

type Server struct {
	Socket           string
	CurrentState     *entities.StateResponse
	InfoLog          *log.Logger
	ErrorLog         *log.Logger
	FileLog          *rotatelogs.RotateLogs
	interruptChannel chan os.Signal
}

func NewServer() (*Server, error) {
	server := Server{
		CurrentState: entities.NewStateResponse(&entities.InitiatorData{
			Host: "",
			User: "",
		}, false),
		InfoLog:          log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime),
		ErrorLog:         log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile),
		interruptChannel: make(chan os.Signal, 1),
	}
	signal.Notify(server.interruptChannel, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-server.interruptChannel
		server.Stop(1)
	}()

	fileLog, err := rotatelogs.New(
		"alarm-button-server-%Y-%m-%d-%H-%M-%S.log",
		rotatelogs.WithMaxAge(serverFileLogMaxAge),
		rotatelogs.WithRotationTime(serverFileLogRotationTime),
	)
	if err != nil {
		return &server, err
	}
	server.FileLog = fileLog
	server.InfoLog.SetOutput(server.FileLog)
	server.ErrorLog.SetOutput(server.FileLog)

	port, err := parseServerArgs()
	if err != nil {
		return &server, err
	}
	server.Socket = "0.0.0.0:" + port
	return &server, nil
}

func parseServerArgs() (string, error) {
	areArgsCorrect := true
	port := ""

	if len(os.Args) < 2 {
		areArgsCorrect = false
	} else {
		port = os.Args[1]
		if _, err := strconv.Atoi(port); err != nil {
			areArgsCorrect = false
		}
	}
	if areArgsCorrect {
		return port, nil
	} else {
		return "", errors.New("порт сервера не указан или указан неверно")
	}
}

func main() {
	server, err := NewServer()
	if err != nil {
		server.ErrorLog.Println("Ошибка при запуске сервера: ", err.Error())
		server.Stop(1)
	}
	server.Run()
}

func (server *Server) Run() {
	listener, err := net.Listen("tcp", server.Socket)
	if err != nil {
		server.ErrorLog.Fatal("Ошибка при запуске сервера: ", err.Error())
	}
	defer listener.Close()
	server.InfoLog.Println("Сервер запущен на ", server.Socket)
	for {
		connection, err := listener.Accept()
		if err != nil {
			server.ErrorLog.Println("Ошибка при ожидании соединения: ", err.Error())
			continue
		}
		go server.decodeClientRequest(connection)
	}
}

func (server *Server) Stop(exitCode int) {
	if server.InfoLog != nil {
		server.InfoLog.Println("Сервер остановлен")
		defer server.InfoLog.SetOutput(os.Stdout)
	}

	if server.ErrorLog != nil {
		defer server.ErrorLog.SetOutput(os.Stderr)
	}

	if server.FileLog != nil {
		defer server.FileLog.Close()
	}
	os.Exit(exitCode)
}

func (server *Server) decodeClientRequest(connection net.Conn) {
	byteBuf := make([]byte, serverBufferSize)
	bytesRead, err := connection.Read(byteBuf)
	if err != nil {
		server.ErrorLog.Println("Ошибка при чтении сообщения: ", err.Error())
	}
	message := &entities.Message{}
	if err := json.Unmarshal(byteBuf[:bytesRead], &message); err != nil {
		server.ErrorLog.Println("Ошибка при обработке сообщения: ", err.Error())
	}
	switch message.Type {
	case "AlarmRequest":
		alarmRequest := entities.AlarmRequest{}
		if err := json.Unmarshal(*message.Data, &alarmRequest); err != nil {
			server.ErrorLog.Println("Ошибка при обработке сообщения: ", err.Error())
		}
		server.processClientRequest(connection, alarmRequest)
	case "StateRequest":
		stateRequest := entities.StateRequest{}
		if err := json.Unmarshal(*message.Data, &stateRequest); err != nil {
			server.ErrorLog.Println("Ошибка при обработке сообщения: ", err.Error())
		}
		server.processClientRequest(connection, stateRequest)
	default:
		server.processClientRequest(connection, message)
	}
	connection.Close()
}

func (server *Server) processClientRequest(connection net.Conn, request interface{}) {
	switch request.(type) {
	case entities.AlarmRequest:
		alarmRequest := request.(entities.AlarmRequest)
		server.InfoLog.Println("Получено оповещение о тревоге: ", alarmRequest.String())
		server.CurrentState = alarmRequest.GetStateResponse()
		server.InfoLog.Println("Текущее состояние: ", server.CurrentState.String())
		response, err := alarmRequest.GetAlarmResponse().Serialize()
		if err != nil {
			server.ErrorLog.Println("Ошибка при формировании ответа: ", err.Error())
		} else {
			connection.Write(response)
		}
	case entities.StateRequest:
		stateRequest := request.(entities.StateRequest)
		server.InfoLog.Println("Получен запрос на проверку состояния: ", stateRequest.String())
		response, err := server.CurrentState.Serialize()
		if err != nil {
			server.ErrorLog.Println("Ошибка при формировании ответа: ", err.Error())
		} else {
			connection.Write(response)
			server.InfoLog.Println("Клиенту отправлено состояние: ", server.CurrentState.String())
		}
	default:
		server.InfoLog.Println("Получена другая информация: ", request)
	}
}
