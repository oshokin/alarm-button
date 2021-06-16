package entities

import (
	"crypto"
	_ "crypto/sha512"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const (
	CurrentVersion       string        = "1.1.0"
	LauncherSleepTime    time.Duration = 1 * time.Second
	UpdateMarkerLifeTime time.Duration = 30 * time.Second
	VersionFileName      string        = "button-version.yaml"
	UpdateMarkerFileName string        = "button-update-marker.bin"
	DefaultFileMode      os.FileMode   = 0755
	//хеш-функция должна быть импортирована выше, иначе ничего не заработает
	//import _ "crypto/sha512"
	DefaultChecksumFunction crypto.Hash   = crypto.SHA512
	clientBufferSize        uint          = 1024
	clientSleepTime         time.Duration = 5 * time.Second
)

type Serializable interface {
	Serialize() ([]byte, error)
}

type VersionDescription struct {
	VersionNumber string `yaml:"version"`
}

type UpdateDescription struct {
	VersionNumber string              `yaml:"version"`
	Files         map[string]string   `yaml:"files"`
	Roles         map[string][]string `yaml:"roles"`
}

func NewUpdateDescription() *UpdateDescription {
	return &UpdateDescription{
		VersionNumber: CurrentVersion,
		Files:         make(map[string]string, 16),
		Roles:         make(map[string][]string, 16),
	}
}

type Message struct {
	Type string           `json:"type" required:"true"`
	Data *json.RawMessage `json:"data" required:"true"`
}

type InitiatorData struct {
	Host string `json:"host" required:"true"`
	User string `json:"user" required:"true"`
}

func NewInitiatorData() (*InitiatorData, error) {
	hostName, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	user, err := user.Current()
	if err != nil {
		return nil, err
	}
	return &InitiatorData{
		Host: hostName,
		User: user.Username,
	}, nil
}

func (initiatorData *InitiatorData) String() string {
	return fmt.Sprintf("хост: %v, пользователь: %v", initiatorData.Host, initiatorData.User)
}

type AlarmRequest struct {
	Initiator            *InitiatorData `json:"initiator" required:"true"`
	IsAlarmButtonPressed bool           `json:"isAlarmButtonPressed" required:"true"`
}

func NewAlarmRequest(client *Client) *AlarmRequest {
	return &AlarmRequest{Initiator: client.Initiator, IsAlarmButtonPressed: client.IsAlarmButtonPressed}
}

func (alarmRequest *AlarmRequest) GetAlarmResponse() *AlarmResponse {
	return &AlarmResponse{DateTime: time.Now(), IsAlarmButtonPressed: alarmRequest.IsAlarmButtonPressed}
}

func (alarmRequest *AlarmRequest) GetStateResponse() *StateResponse {
	return NewStateResponse(alarmRequest.Initiator, alarmRequest.IsAlarmButtonPressed)
}

func (alarmRequest *AlarmRequest) String() string {
	var buttonPressed string
	if alarmRequest.IsAlarmButtonPressed {
		buttonPressed = "да"
	} else {
		buttonPressed = "нет"
	}
	return fmt.Sprintf("инициатор: %v, кнопка нажата: %v", alarmRequest.Initiator.String(), buttonPressed)
}

func (alarmRequest *AlarmRequest) Serialize() ([]byte, error) {
	return SerializeWithTypeName("AlarmRequest", alarmRequest)
}

type AlarmResponse struct {
	DateTime             time.Time `json:"dateTime" required:"true"`
	IsAlarmButtonPressed bool      `json:"isAlarmButtonPressed" required:"true"`
}

func (alarmResponse *AlarmResponse) String() string {
	var buttonPressed string
	if alarmResponse.IsAlarmButtonPressed {
		buttonPressed = "да"
	} else {
		buttonPressed = "нет"
	}
	return fmt.Sprintf("%v, кнопка нажата: %v", alarmResponse.DateTime.Format(time.RFC3339), buttonPressed)
}

func (alarmResponse *AlarmResponse) Serialize() ([]byte, error) {
	return SerializeWithTypeName("AlarmResponse", alarmResponse)
}

type StateRequest struct {
	Initiator *InitiatorData `json:"initiator" required:"true"`
}

func NewStateRequest(client *Client) *StateRequest {
	return &StateRequest{Initiator: client.Initiator}
}

func (stateRequest *StateRequest) String() string {
	return fmt.Sprintf("инициатор: %v", stateRequest.Initiator.String())
}

func (stateRequest *StateRequest) Serialize() ([]byte, error) {
	return SerializeWithTypeName("StateRequest", stateRequest)
}

type StateResponse struct {
	DateTime             time.Time      `json:"dateTime" required:"true"`
	Initiator            *InitiatorData `json:"initiator" required:"true"`
	IsAlarmButtonPressed bool           `json:"isAlarmButtonPressed" required:"true"`
}

func NewStateResponse(data *InitiatorData, buttonPressed bool) *StateResponse {
	return &StateResponse{
		DateTime:             time.Now(),
		Initiator:            data,
		IsAlarmButtonPressed: buttonPressed,
	}
}

func (stateResponse *StateResponse) String() string {
	var buttonPressed string
	if stateResponse.IsAlarmButtonPressed {
		buttonPressed = "да"
	} else {
		buttonPressed = "нет"
	}
	return fmt.Sprintf("%v, инициатор: %v, кнопка нажата: %v",
		stateResponse.DateTime.Format(time.RFC3339),
		stateResponse.Initiator.String(),
		buttonPressed)
}

func (stateResponse *StateResponse) Serialize() ([]byte, error) {
	return SerializeWithTypeName("StateResponse", stateResponse)
}

type Client struct {
	Initiator            *InitiatorData
	ServerSocket         string
	OperatingSystem      string
	IsAlarmButtonPressed bool
	InfoLog              *log.Logger
	ErrorLog             *log.Logger
	interruptChannel     chan os.Signal
	debugMode            bool
}

func NewClient() (*Client, error) {
	client := Client{
		Initiator:        nil,
		ServerSocket:     "",
		OperatingSystem:  runtime.GOOS,
		InfoLog:          log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime),
		ErrorLog:         log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile),
		interruptChannel: make(chan os.Signal, 1),
	}
	signal.Notify(client.interruptChannel, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-client.interruptChannel
		client.Stop(false, 1)
	}()
	initiatorData, err := NewInitiatorData()
	if err != nil {
		return &client, err
	}
	client.Initiator = initiatorData
	serverSocket, debugMode, err := parseClientArgs()
	if err != nil {
		return &client, err
	}
	client.ServerSocket = serverSocket
	client.debugMode = debugMode
	return &client, nil
}

func parseClientArgs() (string, bool, error) {
	serverSocket := ""
	parsingError := errors.New("укажите адрес сервера (хост:порт)")
	debugModePointer := flag.Bool("debug", false, "режим отладки (ПК не выключается)")
	flag.Parse()
	if len(flag.Args()) > 0 {
		serverSocket = flag.Arg(0)
		_, err := net.ResolveTCPAddr("tcp", serverSocket)
		if err != nil {
			parsingError = fmt.Errorf("некорректный адрес сервера, %s", err.Error())
		} else {
			parsingError = nil
		}
	}
	return serverSocket, *debugModePointer, parsingError
}

func (client *Client) RunChecker() {
	request, err := NewStateRequest(client).Serialize()
	if err != nil {
		client.ErrorLog.Println("Ошибка при преобразовании данных:", err.Error())
		client.Stop(false, 1)
	}
	for {
		client.InfoLog.Println("Пробую отправить запрос о состоянии на сервер")
		client.sendToServer(request)
	}
}

func (client *Client) RunAlarmer(IsAlarmButtonPressed bool) {
	client.IsAlarmButtonPressed = IsAlarmButtonPressed
	request, err := NewAlarmRequest(client).Serialize()
	if err != nil {
		client.ErrorLog.Println("Ошибка при преобразовании данных:", err.Error())
		client.Stop(false, 1)
	}
	for {
		client.InfoLog.Println("Пробую отправить запрос о тревоге на сервер")
		client.sendToServer(request)
	}
}

func (client *Client) Stop(IsPowerOffRequired bool, params ...int) {
	exitCode := 0
	if len(params) > 0 {
		exitCode = params[0]
	}

	if IsPowerOffRequired {
		if err := client.shutdownPC(); err != nil {
			client.ErrorLog.Println("Ошибка при выключении компьютера:", err.Error())
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

func (client *Client) processAlarmButtonState() {
	if client.IsAlarmButtonPressed {
		client.Stop(client.IsAlarmButtonPressed)
	}
}

func (client *Client) shutdownPC() error {
	client.InfoLog.Println("Выключаем ПК")
	if client.debugMode {
		return nil
	} else {
		osLC := strings.ToLower(client.OperatingSystem)
		if strings.Contains(osLC, "linux") || strings.Contains(osLC, "darwin") {
			return exec.Command("shutdown", "-h", "now").Run()
		} else if strings.Contains(osLC, "windows") {
			return exec.Command("shutdown.exe", "-s", "-f", "-t", "0").Run()
		} else {
			return errors.New("ОС " + client.OperatingSystem + " не поддерживается.")
		}
	}
}

func (client *Client) sendToServer(request []byte) {
	connection, err := net.Dial("tcp", client.ServerSocket)
	if err != nil {
		client.ErrorLog.Println("Не удалось подключиться к серверу:", err.Error())
	} else {
		connection.Write(request)
		client.decodeServerResponse(connection)
		connection.Close()
	}
	time.Sleep(clientSleepTime)
}

func (client *Client) decodeServerResponse(connection net.Conn) {
	byteBuf := make([]byte, clientBufferSize)
	bytesRead, err := connection.Read(byteBuf)
	if err != nil {
		client.ErrorLog.Println("Не удалось прочитать ответ сервера:", err.Error())
	} else {
		message := &Message{}
		if err := json.Unmarshal(byteBuf[:bytesRead], &message); err != nil {
			client.ErrorLog.Println("Ошибка при парсинге сообщения:", err.Error())
		}
		switch message.Type {
		case "AlarmResponse":
			alarmResponse := AlarmResponse{}
			if err := json.Unmarshal(*message.Data, &alarmResponse); err != nil {
				client.ErrorLog.Println("Ошибка при парсинге сообщения:", err.Error())
			}
			client.processServerResponse(alarmResponse)
		case "StateResponse":
			stateResponse := StateResponse{}
			if err := json.Unmarshal(*message.Data, &stateResponse); err != nil {
				client.ErrorLog.Println("Ошибка при парсинге сообщения:", err.Error())
			}
			client.processServerResponse(stateResponse)
		default:
			client.processServerResponse(message)
		}
	}
}

func (client *Client) processServerResponse(response interface{}) {
	switch response.(type) {
	case AlarmResponse:
		alarmResponse := response.(AlarmResponse)
		client.InfoLog.Println("Получен ответ на разовую тревогу:", alarmResponse.String())
		client.Stop(false)
	case StateResponse:
		stateResponse := response.(StateResponse)
		client.InfoLog.Println("Получен ответ на проверку состояния:", stateResponse.String())
		client.IsAlarmButtonPressed = stateResponse.IsAlarmButtonPressed
		client.processAlarmButtonState()
	default:
		client.InfoLog.Println("Получена другая информация:", response)
	}
}

func SerializeWithTypeName(typeName string, entity interface{}) ([]byte, error) {
	byteMessage, err := json.Marshal(entity)
	if err != nil {
		return nil, err
	}
	data := json.RawMessage(byteMessage)
	encodedMessage, err := json.Marshal(Message{typeName, &data})
	if err != nil {
		return nil, err
	}
	return encodedMessage, nil
}

func GetFileChecksum(fileName string) ([]byte, error) {
	contents, err := os.ReadFile(fileName)
	if err != nil {
		return nil, err
	}
	if !DefaultChecksumFunction.Available() {
		return nil, errors.New("хеш-функция не доступна, подсчет контрольной суммы невозможен")
	}
	hasher := DefaultChecksumFunction.New()
	hasher.Write(contents)
	newFileChecksum := hasher.Sum(nil)

	return newFileChecksum[:], nil
}
