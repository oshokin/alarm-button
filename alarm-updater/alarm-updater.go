package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/doitdistributed/go-update"
	"github.com/mitchellh/go-ps"
	"github.com/oshokin/alarm-button/entities"
	"gopkg.in/yaml.v3"
)

type Updater struct {
	UpdateDescription  *entities.UpdateDescription
	IsUpdateNeeded     bool
	InfoLog            *log.Logger
	ErrorLog           *log.Logger
	temporaryDirectory string
	downloadedFiles    map[string]string
	interruptChannel   chan os.Signal
}

func NewUpdater() (*Updater, error) {
	updater := Updater{
		InfoLog:          log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime),
		ErrorLog:         log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile),
		downloadedFiles:  make(map[string]string, 16),
		interruptChannel: make(chan os.Signal, 1),
	}
	signal.Notify(updater.interruptChannel, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-updater.interruptChannel
		updater.Stop(1)
	}()
	isUpdaterRunningNow := entities.IsUpdaterRunningNow(updater.InfoLog, updater.ErrorLog)
	if isUpdaterRunningNow {
		return &updater, errors.New("the updater is already running")
	}
	updateMarker, err := os.Create(entities.UpdateMarkerFileName)
	if err != nil {
		return &updater, err
	}
	err = updateMarker.Close()
	if err != nil {
		return &updater, err
	}
	err = entities.ReadCommonSettingsFromFile()
	if err != nil {
		return &updater, err
	}
	entities.Settings.UpdateType, err = parseUpdaterArgs()
	if err != nil {
		return &updater, err
	}
	return &updater, nil
}

func parseUpdaterArgs() (string, error) {
	updateTypePointer := flag.String("type", "client", "user role")
	flag.Parse()
	var err error
	if len(flag.Args()) > 0 {
		err = errors.New("invalid command line arguments")
	} else {
		err = nil
	}
	return *updateTypePointer, err
}

func (updater *Updater) Stop(exitCode int) {
	_, err := os.Stat(entities.UpdateMarkerFileName)
	if err == nil {
		err := os.Remove(entities.UpdateMarkerFileName)
		if err != nil && updater.ErrorLog != nil {
			updater.ErrorLog.Println("Error while deleting the update marker:", err.Error())
		}
	}
	_, err = os.Stat(updater.temporaryDirectory)
	if err == nil {
		err := os.RemoveAll(updater.temporaryDirectory)
		if err != nil && updater.ErrorLog != nil {
			updater.ErrorLog.Println("Error while deleting the temporary directory:", err.Error())
		}
	}
	if updater.InfoLog != nil {
		updater.InfoLog.Println("The updater has been stopped")
	}
	os.Exit(exitCode)
}

func main() {
	updater, err := NewUpdater()
	if err != nil {
		updater.ErrorLog.Println("Error while launching the updater:", err.Error())
		updater.Stop(1)
	}
	updater.Run()
}

func (updater *Updater) Run() {
	updater.InfoLog.Println("Terminating alarm button processes forcibly")
	err := updater.terminateAlarmButtonProcesses()
	if err != nil {
		updater.ErrorLog.Println("Error while terminating alarm button processes:", err.Error())
		updater.Stop(1)
	}
	updater.InfoLog.Println("Downloading the update description from the server")
	err = updater.fillUpdateDescription()
	if err != nil {
		updater.ErrorLog.Println("Error while downloading version description:", err.Error())
		updater.Stop(1)
	}
	updater.InfoLog.Println("Verifying the checksum of files on the client and server")
	err = updater.validateChecksum()
	if err != nil {
		updater.ErrorLog.Println("Error while verifying the checksum:", err.Error())
		updater.Stop(1)
	}
	if updater.IsUpdateNeeded {
		updater.InfoLog.Println("Downloading update files to a temporary folder")
		err = updater.downloadFiles()
		if err != nil {
			updater.ErrorLog.Println("Error while downloading files from the server:", err.Error())
			updater.Stop(1)
		}
		updater.InfoLog.Println("Updating files on the client")
		err = updater.updateFiles()
		if err != nil {
			updater.ErrorLog.Println("Error while updating files on the client:", err.Error())
			updater.Stop(1)
		}
	} else {
		updater.InfoLog.Println("No update required")
	}
	updater.InfoLog.Println("Starting required executables")
	err = updater.startRequiredExecutables()
	if err != nil {
		updater.ErrorLog.Println("Error while starting required executables:", err.Error())
		updater.Stop(1)
	}
	updater.InfoLog.Println("Exiting the updater now")
	updater.Stop(0)
}

func (updater *Updater) terminateAlarmButtonProcesses() error {
	executableFiles := entities.SliceToStringMap(entities.AllExecutableFiles)
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
		if _, found := executableFiles[processName]; found {
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

func (updater *Updater) fillUpdateDescription() error {
	response, err := updater.getFileBodyFromServer(entities.VersionFileName)
	if response != nil {
		defer response.Body.Close()
	}
	if err != nil {
		return err
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(data, &updater.UpdateDescription)
	if err != nil {
		return err
	}
	return nil
}

func (updater *Updater) getFileBodyFromServer(fileName string) (*http.Response, error) {
	serverUpdateURL, err := url.Parse(entities.Settings.ServerUpdateFolder)
	if err != nil {
		return nil, err
	}
	serverUpdateURL.Path = path.Join(serverUpdateURL.Path, fileName)
	finalURL := serverUpdateURL.String()
	response, err := http.Get(finalURL)
	if err != nil {
		return response, err
	}
	if response.StatusCode != 200 {
		return response, fmt.Errorf("%s, %s", finalURL, response.Status)
	}
	return response, err
}

func (updater *Updater) validateChecksum() error {
	files, areRolesFound := updater.UpdateDescription.Roles[entities.Settings.UpdateType]
	if !areRolesFound {
		return fmt.Errorf("unable to find a list of files for the user role %s", entities.Settings.UpdateType)
	}
	for _, fileName := range files {
		serverFileBase64, isServerChecksumFound := updater.UpdateDescription.Files[fileName]
		if !isServerChecksumFound {
			return fmt.Errorf("the checksum of the file %s is not set on the server", fileName)
		}
		serverFileChecksum, err := base64.StdEncoding.DecodeString(serverFileBase64)
		if err != nil {
			return err
		}
		isClientChecksumCorrect := true
		if _, err := os.Stat(fileName); err != nil {
			if os.IsNotExist(err) {
				isClientChecksumCorrect = false
			} else {
				return err
			}
		}
		if isClientChecksumCorrect {
			clientChecksum, err := entities.GetFileChecksum(fileName)
			if err != nil {
				return err
			}
			comparationResult := bytes.Compare(serverFileChecksum, clientChecksum)
			if comparationResult != 0 {
				isClientChecksumCorrect = false
			}
		}
		if !isClientChecksumCorrect {
			updater.IsUpdateNeeded = true
			return nil
		}
	}
	return nil
}

func (updater *Updater) downloadFiles() error {
	temporaryDirectory, err := ioutil.TempDir("", "alarm-button-updater-")
	if err != nil {
		return err
	}
	updater.temporaryDirectory = temporaryDirectory
	files := updater.UpdateDescription.Roles[entities.Settings.UpdateType]
	for _, fileName := range files {
		response, err := updater.getFileBodyFromServer(fileName)
		if err != nil {
			if response != nil {
				response.Body.Close()
			}
			return err
		}
		outputFileName := filepath.Join(temporaryDirectory, fileName)
		outputFile, err := os.Create(outputFileName)
		if err != nil {
			response.Body.Close()
			return err
		}
		_, err = io.Copy(outputFile, response.Body)
		response.Body.Close()
		outputFile.Close()

		if err != nil {
			return err
		}
		updater.downloadedFiles[fileName] = outputFileName
		updater.InfoLog.Printf("The file %s was downloaded successfully\n", outputFileName)
	}
	return nil
}

func (updater *Updater) updateFiles() error {
	for fileName, downloadedFileName := range updater.downloadedFiles {
		updater.InfoLog.Printf("Updating the file %s\n", fileName)
		data, err := os.ReadFile(downloadedFileName)
		if err != nil {
			return err
		}
		updater.InfoLog.Printf("Looking for a checksum")
		downloadedFileBase64, isChecksumFound := updater.UpdateDescription.Files[fileName]
		if !isChecksumFound {
			return fmt.Errorf("the checksum of the %s file is not set", downloadedFileName)
		}
		downloadedFileChecksum, err := base64.StdEncoding.DecodeString(downloadedFileBase64)
		if err != nil {
			return err
		}
		if _, err := os.Stat(fileName); err != nil && os.IsNotExist(err) {
			_, err := os.Create(fileName)
			if err != nil {
				return err
			}
		}
		updater.InfoLog.Printf("Applying update")
		options := &update.Options{
			TargetPath: fileName,
			TargetMode: entities.DefaultFileMode,
			Checksum:   downloadedFileChecksum,
			Hash:       entities.DefaultChecksumFunction,
		}
		dataReader := bytes.NewReader(data)
		err = update.Apply(dataReader, *options)
		if err != nil {
			return err
		}
		oldFileName := fmt.Sprintf("%s.old", fileName)
		if _, err := os.Stat(oldFileName); err == nil {
			os.Remove(oldFileName)
		}
	}
	return nil
}

func (updater *Updater) startRequiredExecutables() error {
	executable, isExecutableFound := updater.UpdateDescription.Executables[entities.Settings.UpdateType]
	if !isExecutableFound {
		return fmt.Errorf("unable to find a executable for the user role %s", entities.Settings.UpdateType)
	}
	osLC := strings.ToLower(runtime.GOOS)
	if strings.Contains(osLC, "linux") || strings.Contains(osLC, "darwin") {
		return exec.Command(executable).Start()
	} else if strings.Contains(osLC, "windows") {
		return exec.Command("cmd.exe", "/C", "start", executable).Start()
	} else {
		return fmt.Errorf("%s OS is not supported", runtime.GOOS)
	}
}
