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
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/mitchellh/go-ps"
	"gopkg.in/yaml.v3"
)

const (
	CurrentVersion       string        = "1.2.0"
	LauncherSleepTime    time.Duration = 1 * time.Second
	UpdateMarkerLifeTime time.Duration = 30 * time.Second
	SettingsFileName     string        = "alarm-button-settings.yaml"
	VersionFileName      string        = "alarm-button-version.yaml"
	UpdateMarkerFileName string        = "alarm-button-update-marker.bin"
	ServerExecutable     string        = "alarm-server.exe"
	CheckerExecutable    string        = "alarm-checker.exe"
	UpdaterExecutable    string        = "alarm-updater.exe"
	DefaultFileMode      os.FileMode   = 0755
	//хеш-функция должна быть импортирована выше, иначе ничего не заработает
	//import _ "crypto/sha512"
	DefaultChecksumFunction crypto.Hash   = crypto.SHA512
	clientBufferSize        uint          = 1024
	clientSleepTime         time.Duration = 5 * time.Second
)

var (
	Settings         *CommonSettings
	AllowedUserRoles = map[string][]string{
		"client": {"alarm-button-on.exe", CheckerExecutable, UpdaterExecutable, SettingsFileName},
		"server": {"alarm-button-off.exe", ServerExecutable, UpdaterExecutable, SettingsFileName},
	}
	ExecutablesByUserRoles = map[string]string{
		"client": CheckerExecutable,
		"server": ServerExecutable,
	}
	AllExecutableFiles = []string{"alarm-button-off.exe", "alarm-button-on.exe", CheckerExecutable, ServerExecutable, UpdaterExecutable}
)

type CommonSettings struct {
	ServerUpdateFolder string `yaml:"updateFolder"`
	ServerSocket       string `yaml:"serverSocket"`
	UpdateType         string `yaml:"-"`
}

func ReadCommonSettingsFromFile() error {
	_, err := os.Stat(SettingsFileName)
	if err != nil {
		return err
	} else {
		data, err := os.ReadFile(SettingsFileName)
		if err != nil {
			return err
		}
		err = yaml.Unmarshal(data, &Settings)
		if err != nil {
			return err
		}
	}
	_, err = url.ParseRequestURI(Settings.ServerUpdateFolder)
	if err != nil {
		return fmt.Errorf("invalid URI of updates folder, %s", err.Error())
	}
	_, err = net.ResolveTCPAddr("tcp", Settings.ServerSocket)
	if err != nil {
		return fmt.Errorf("invalid server address, %s", err.Error())
	}
	return nil
}

func ReadCommonSettingsFromArgs() error {
	serverUpdateFolder := ""
	serverSocket := ""
	parsingError := errors.New(
		"not all required parameters are specified - " +
			"the first parameter must be the URI of updates folder (for example, https://localhost.ru/alarm-button), " +
			"the second parameter must be the server socket (for example, 127.0.0.1:8080)")
	flag.Parse()
	if len(flag.Args()) == 2 {
		serverUpdateFolder = flag.Arg(0)
		serverSocket = flag.Arg(1)
		_, err := url.ParseRequestURI(serverUpdateFolder)
		if err != nil {
			parsingError = fmt.Errorf("invalid URI of updates folder, %s", err.Error())
		} else {
			parsingError = nil
		}
		if parsingError == nil {
			_, err := net.ResolveTCPAddr("tcp", serverSocket)
			if err != nil {
				parsingError = fmt.Errorf("invalid server address, %s", err.Error())
			} else {
				parsingError = nil
			}
		}
	}
	if parsingError == nil {
		Settings = &CommonSettings{
			ServerUpdateFolder: serverUpdateFolder,
			ServerSocket:       serverSocket,
			UpdateType:         "",
		}
	}
	return parsingError
}

func SaveCommonSettingsToFile() error {
	if Settings == nil {
		return errors.New("settings are not set")
	}
	contents, err := yaml.Marshal(Settings)
	if err != nil {
		return err
	}
	err = os.WriteFile(SettingsFileName, contents, DefaultFileMode)
	if err != nil {
		return err
	}
	return nil
}

type UpdateDescription struct {
	VersionNumber string              `yaml:"version"`
	Files         map[string]string   `yaml:"files"`
	Roles         map[string][]string `yaml:"roles"`
	Executables   map[string]string   `yaml:"executables"`
}

func NewUpdateDescription() *UpdateDescription {
	return &UpdateDescription{
		VersionNumber: CurrentVersion,
		Files:         make(map[string]string, 16),
		Roles:         make(map[string][]string, 16),
		Executables:   make(map[string]string, 16),
	}
}

type Serializable interface {
	Serialize() ([]byte, error)
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
	return fmt.Sprintf("host: %v, user: %v", initiatorData.Host, initiatorData.User)
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
		buttonPressed = "yes"
	} else {
		buttonPressed = "no"
	}
	return fmt.Sprintf("initiator: %v, button is pressed: %v", alarmRequest.Initiator.String(), buttonPressed)
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
		buttonPressed = "yes"
	} else {
		buttonPressed = "no"
	}
	return fmt.Sprintf("%v, button is pressed: %v", alarmResponse.DateTime.Format(time.RFC3339), buttonPressed)
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
	return fmt.Sprintf("initiator: %v", stateRequest.Initiator.String())
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
		buttonPressed = "yes"
	} else {
		buttonPressed = "no"
	}
	return fmt.Sprintf("%v, initiator: %v, button is pressed: %v",
		stateResponse.DateTime.Format(time.RFC3339),
		stateResponse.Initiator.String(),
		buttonPressed)
}

func (stateResponse *StateResponse) Serialize() ([]byte, error) {
	return SerializeWithTypeName("StateResponse", stateResponse)
}

type Client struct {
	Initiator            *InitiatorData
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
	isUpdaterRunningNow := IsUpdaterRunningNow(client.InfoLog, client.ErrorLog)
	if isUpdaterRunningNow {
		return &client, errors.New("the updater is running now")
	}
	err := ReadCommonSettingsFromFile()
	if err != nil {
		return &client, err
	}
	initiatorData, err := NewInitiatorData()
	if err != nil {
		return &client, err
	}
	client.Initiator = initiatorData
	debugMode, err := parseClientArgs()
	if err != nil {
		return &client, err
	}
	client.debugMode = debugMode
	return &client, nil
}

func parseClientArgs() (bool, error) {
	debugModePointer := flag.Bool("debug", false, "debug mode (PC does not turn off)")
	flag.Parse()
	var err error
	if len(flag.Args()) > 0 {
		err = errors.New("invalid command line arguments")
	} else {
		err = nil
	}
	return *debugModePointer, err
}

func (client *Client) RunChecker() {
	request, err := NewStateRequest(client).Serialize()
	if err != nil {
		client.ErrorLog.Println("Error while converting data:", err.Error())
		client.Stop(false, 1)
	}
	for {
		client.InfoLog.Println("Trying to send an alarm status request to the server")
		client.sendToServer(request)
	}
}

func (client *Client) RunAlarmer(IsAlarmButtonPressed bool) {
	client.IsAlarmButtonPressed = IsAlarmButtonPressed
	request, err := NewAlarmRequest(client).Serialize()
	if err != nil {
		client.ErrorLog.Println("Error while converting data:", err.Error())
		client.Stop(false, 1)
	}
	for {
		client.InfoLog.Println("Trying to send an alarm request to the server")
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
			client.ErrorLog.Println("Error during shutdown:", err.Error())
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
	client.InfoLog.Println("Turning off the PC")
	if client.debugMode {
		return nil
	} else {
		osLC := strings.ToLower(client.OperatingSystem)
		if strings.Contains(osLC, "linux") || strings.Contains(osLC, "darwin") {
			return exec.Command("shutdown", "-h", "now").Start()
		} else if strings.Contains(osLC, "windows") {
			return exec.Command("shutdown.exe", "-s", "-f", "-t", "0").Start()
		} else {
			return fmt.Errorf("%s OS is not supported", client.OperatingSystem)
		}
	}
}

func (client *Client) sendToServer(request []byte) {
	connection, err := net.Dial("tcp", Settings.ServerSocket)
	if err != nil {
		client.ErrorLog.Println("Failed to read server response:", err.Error())
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
		client.ErrorLog.Println("Failed to read server response:", err.Error())
	} else {
		message := &Message{}
		if err := json.Unmarshal(byteBuf[:bytesRead], &message); err != nil {
			client.ErrorLog.Println("Error while parsing the message:", err.Error())
		}
		switch message.Type {
		case "AlarmResponse":
			alarmResponse := AlarmResponse{}
			if err := json.Unmarshal(*message.Data, &alarmResponse); err != nil {
				client.ErrorLog.Println("Error while parsing the message:", err.Error())
			}
			client.processServerResponse(alarmResponse)
		case "StateResponse":
			stateResponse := StateResponse{}
			if err := json.Unmarshal(*message.Data, &stateResponse); err != nil {
				client.ErrorLog.Println("Error while parsing the message:", err.Error())
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
		client.InfoLog.Println("Alarm response received:", alarmResponse.String())
		client.Stop(false)
	case StateResponse:
		stateResponse := response.(StateResponse)
		client.InfoLog.Println("Status check response received:", stateResponse.String())
		client.IsAlarmButtonPressed = stateResponse.IsAlarmButtonPressed
		client.processAlarmButtonState()
	default:
		client.InfoLog.Println("Other information received:", response)
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
		return nil, errors.New("hash function is not available, checksum calculation is not possible")
	}
	hasher := DefaultChecksumFunction.New()
	hasher.Write(contents)
	newFileChecksum := hasher.Sum(nil)

	return newFileChecksum[:], nil
}

func IsUpdaterRunningNow(infoLog *log.Logger, errorLog *log.Logger) bool {
	if infoLog != nil {
		infoLog.Println("Checking for the presence of an update marker")
	}
	funcResult := true
	fileInfo, err := os.Stat(UpdateMarkerFileName)
	if err != nil {
		if os.IsNotExist(err) {
			if infoLog != nil {
				infoLog.Println("Update marker not found")
			}
			funcResult = false
		} else {
			if time.Since(fileInfo.ModTime()) > UpdateMarkerLifeTime {
				if infoLog != nil {
					infoLog.Println("The update marker is too old, perhaps the update is stuck. Trying to delete the file")
				}
				err = TerminateProcessByName(UpdaterExecutable)
				funcResult = (err != nil)
				if err == nil {
					err = os.Remove(UpdateMarkerFileName)
					funcResult = (err != nil)
				}
			}
		}
	}
	return funcResult
}

func TerminateProcessByName(processNameToTerminate string) error {
	processList, err := ps.Processes()
	if err != nil {
		return err
	}
	thisProcessID := os.Getpid()
	for processIndex := range processList {
		process := processList[processIndex]
		processID := process.Pid()
		if processID == thisProcessID {
			continue
		}
		processName := process.Executable()
		if processName == processNameToTerminate {
			runningProcess, err := os.FindProcess(processID)
			if err != nil {
				return err
			}
			err = runningProcess.Kill()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func SliceToStringMap(elements []string) map[string]bool {
	funcResult := make(map[string]bool, len(elements))
	for _, value := range elements {
		funcResult[value] = true
	}
	return funcResult
}
