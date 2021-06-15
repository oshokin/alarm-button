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
	"os/signal"
	"path"
	"path/filepath"
	"syscall"

	"github.com/doitdistributed/go-update"
	"github.com/hashicorp/go-version"
	"github.com/oshokin/alarm-button/entities"
	"gopkg.in/yaml.v3"
)

type Updater struct {
	ServerUpdateFolder string
	UpdateType         string
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
	updateMarker, err := os.Create(entities.UpdateMarkerFileName)
	if err != nil {
		return &updater, err
	}
	err = updateMarker.Close()
	if err != nil {
		return &updater, err
	}
	serverUpdateFolder, updateType, err := parseUpdaterArgs()
	if err != nil {
		return &updater, err
	}
	updater.ServerUpdateFolder = serverUpdateFolder
	updater.UpdateType = updateType

	return &updater, nil
}

func parseUpdaterArgs() (string, string, error) {
	areArgsCorrect := true
	serverUpdateFolder := ""
	updateTypePointer := flag.String("type", "user", "роль пользователя")
	flag.Parse()
	if len(flag.Args()) < 1 {
		areArgsCorrect = false
	} else {
		serverUpdateFolder = flag.Arg(0)
	}
	var err error
	if !areArgsCorrect {
		err = errors.New("URL файла с описанием обновления не указан или указан неверно")
	} else {
		err = nil
	}

	return serverUpdateFolder, *updateTypePointer, err
}

func (updater *Updater) Stop(exitCode int) {
	_, err := os.Stat(entities.UpdateMarkerFileName)
	if err == nil {
		err := os.Remove(entities.UpdateMarkerFileName)
		if err != nil && updater.ErrorLog != nil {
			updater.ErrorLog.Println("Ошибка при удалении маркера обновления:", err.Error())
		}
	}
	_, err = os.Stat(updater.temporaryDirectory)
	if err == nil {
		err := os.RemoveAll(updater.temporaryDirectory)
		if err != nil && updater.ErrorLog != nil {
			updater.ErrorLog.Println("Ошибка при удалении временного каталога:", err.Error())
		}
	}
	if updater.InfoLog != nil {
		updater.InfoLog.Println("Обновитель остановлен")
	}
	os.Exit(exitCode)
}

func main() {
	updater, err := NewUpdater()
	if err != nil {
		updater.ErrorLog.Println("Ошибка при запуске обновителя:", err.Error())
		updater.Stop(1)
	}
	updater.Run()
}

func (updater *Updater) Run() {
	updater.InfoLog.Println("Загружаю описание обновления с сервера")
	err := updater.fillUpdateDescription()
	if err != nil {
		updater.ErrorLog.Println("Ошибка при загрузке описания версии:", err.Error())
		updater.Stop(1)
	}
	updater.InfoLog.Println("Сравниваю версии на клиенте и сервере")
	err = updater.checkVersion()
	if err != nil {
		updater.ErrorLog.Println("Ошибка при сравнении версий:", err.Error())
		updater.Stop(1)
	}
	updater.InfoLog.Println("Сверяю контрольную сумму файлов на клиенте и сервере")
	err = updater.validateChecksum()
	if err != nil {
		updater.ErrorLog.Println("Ошибка при сверке контрольной суммы:", err.Error())
		updater.Stop(1)
	}
	updater.InfoLog.Println("Загружаю файлы обновления во временную папку")
	if updater.IsUpdateNeeded {
		err = updater.downloadFiles()
		if err != nil {
			updater.ErrorLog.Println("Ошибка при загрузке файлов с сервера:", err.Error())
			updater.Stop(1)
		}
		updater.InfoLog.Println("Обновляю файлы на клиенте")
		err = updater.updateFiles()
		if err != nil {
			updater.ErrorLog.Println("Ошибка при обновлении файлов на клиенте:", err.Error())
			updater.Stop(1)
		}
	} else {
		updater.InfoLog.Println("Обновление не требуется")
	}
	updater.Stop(0)
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
	serverUpdateURL, err := url.Parse(updater.ServerUpdateFolder)
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

func (updater *Updater) checkVersion() error {
	clientVersion, err := version.NewVersion(entities.CurrentVersion)
	if err != nil {
		return err
	}
	serverVersion, err := version.NewVersion(updater.UpdateDescription.VersionNumber)
	if err != nil {
		return err
	}
	updater.IsUpdateNeeded = (clientVersion.LessThan(serverVersion))

	return nil
}

func (updater *Updater) validateChecksum() error {
	files, areRolesFound := updater.UpdateDescription.Roles[updater.UpdateType]
	if !areRolesFound {
		return fmt.Errorf("не найден список файлов для роли %s", updater.UpdateType)
	}
	for _, fileName := range files {
		serverFileBase64, isServerChecksumFound := updater.UpdateDescription.Files[fileName]
		if !isServerChecksumFound {
			return fmt.Errorf("на сервере не задана контрольная сумма файла %s", fileName)
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
	files := updater.UpdateDescription.Roles[updater.UpdateType]

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
		updater.InfoLog.Printf("Успешно загружен файл %s\n", outputFileName)
	}

	return nil
}

func (updater *Updater) updateFiles() error {
	for fileName, downloadedFileName := range updater.downloadedFiles {
		updater.InfoLog.Printf("Обновляю файл %s\n", fileName)
		data, err := os.ReadFile(downloadedFileName)
		if err != nil {
			return err
		}
		updater.InfoLog.Printf("Ищу контрольную сумму")
		downloadedFileBase64, isChecksumFound := updater.UpdateDescription.Files[fileName]
		if !isChecksumFound {
			return fmt.Errorf("не задана контрольная сумма файла %s", downloadedFileName)
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
		updater.InfoLog.Printf("Применяю обновление")
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
