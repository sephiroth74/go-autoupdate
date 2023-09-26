package autoupdate

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/charmbracelet/log"
	"github.com/goware/urlx"
	"github.com/hashicorp/go-version"
	"github.com/schollz/progressbar/v3"
	"github.com/sephiroth74/go-autoupdate/io_util"
	"github.com/sephiroth74/go-autoupdate/tar_util"
)

type Options struct {
	BaseUrl  string
	Version  string
	SelfName string
	Logger   *log.Logger
}

type AutoUpdate struct {
	Options Options
}

func (a AutoUpdate) BackgroundCheck() chan VersionResult {
	channel := make(chan VersionResult)
	go func() {
		result, err := a.getNewVersion()
		defer close(channel)
		if err != nil {
			channel <- VersionResult{Error: err}
		} else if result != nil {
			channel <- VersionResult{Version: result}
		} else {
			//
		}
	}()
	return channel
}

func (a AutoUpdate) InstallUpdate(v VersionJson, bar *progressbar.ProgressBar) chan error {
	ch := make(chan error)
	go func() {
		if !v.IsValidUpdate() {
			ch <- errors.New("invalid version update")
		} else {
			ch <- a.installUpdate(v, bar)
		}
		_ = bar.Close()
		close(ch)
	}()
	return ch

}

func (a AutoUpdate) getNewVersion() (*VersionJson, error) {
	var osName = runtime.GOOS
	var osArch = runtime.GOARCH
	currentVersion, err := version.NewVersion(a.Options.Version)
	if err != nil {
		return nil, err
	}

	var jsonUrl = fmt.Sprintf("%s/version_%s_%s.json", a.Options.BaseUrl, osName, osArch)
	versionJson, err := a.parseJson(jsonUrl)
	if err != nil {
		return nil, err
	}

	if versionJson.Semver.GreaterThan(currentVersion) {
		return versionJson, nil
	} else {
		return nil, nil
	}
}

func (a AutoUpdate) installUpdate(v VersionJson, bar *progressbar.ProgressBar) error {
	if a.Options.Logger != nil {
		a.Options.Logger.Infof("installing %s update..", v.Version)
	}

	// 1/4 download update
	dstFilename, err := a.downloadUpdate(v, bar)
	if err != nil {
		return err
	}

	// 2/4 verify checksum
	if err := a.verifyChecksum(v, dstFilename); err != nil {
		return err
	}

	// 3/4 extract update
	extractedFile, err := a.extractUpdate(v, dstFilename)
	if err != nil {
		return err
	}

	// 4/4 copy the update to the executable
	return a.writeUpdate(extractedFile, a.Options.SelfName)
}

func (a AutoUpdate) downloadUpdate(v VersionJson, bar *progressbar.ProgressBar) (string, error) {
	fileUrl, err := urlx.NormalizeString(fmt.Sprintf("%s/%s", a.Options.BaseUrl, v.Path))
	if err != nil {
		return "", err
	}

	if a.Options.Logger != nil {
		a.Options.Logger.Debugf("[1/4] downloading %s..", fileUrl)
	}

	resp, err := http.Get(fileUrl)
	if err != nil {
		return "", err
	}
	defer func(Body io.ReadCloser) { _ = Body.Close() }(resp.Body)

	file, err := os.CreateTemp(os.TempDir(), filepath.Base(v.Path))
	if err != nil {
		return "", err
	}

	dstFilename := file.Name()

	if a.Options.Logger != nil {
		a.Options.Logger.Debugf("downloading to %s..", dstFilename)
	}

	defer func(file *os.File) { _ = file.Close() }(file)

	var writer io.Writer

	if bar != nil {
		bar.ChangeMax64(v.Size)
		writer = io.MultiWriter(file, bar)
	} else {
		writer = file
	}

	total, err := io.Copy(writer, resp.Body)
	if err != nil {
		return "", err
	}

	if total != v.Size {
		return "", errors.New("invalid file size")
	}

	if a.Options.Logger != nil {
		a.Options.Logger.Debugf("filesize: %d", total)
	}
	return dstFilename, nil
}

func (a AutoUpdate) extractUpdate(v VersionJson, tarFilename string) (string, error) {
	if a.Options.Logger != nil {
		a.Options.Logger.Info("[3/4] extracting update")
	}
	// creating reader..
	reader, err := os.Open(tarFilename)
	if err != nil {
		return "", err
	}
	defer func(reader *os.File) { _ = reader.Close() }(reader)

	// creating writer..
	dstDir := filepath.Join(os.TempDir(), v.Checksum)

	if a.Options.Logger != nil {
		a.Options.Logger.Debugf("extrating %s into %s", filepath.Base(tarFilename), dstDir)
	}

	result, err := tar_util.Untar(dstDir, reader)
	if err != nil {
		return "", err
	}

	if len(result) < 1 {
		return "", errors.New("no files extracted")
	}

	extractedFile := result[0]

	if a.Options.Logger != nil {
		a.Options.Logger.Debugf("extracted file: %s", extractedFile)
	}

	if !io_util.FileExists(extractedFile) {
		return "", os.ErrNotExist
	}

	return extractedFile, nil
}

func (a AutoUpdate) verifyChecksum(v VersionJson, tarFilename string) error {
	if a.Options.Logger != nil {
		a.Options.Logger.Debug("[2/4] verifying checksum")
	}

	sha, err := a.computeChecksum(tarFilename)
	if err != nil {
		return err
	}

	checksum := fmt.Sprintf("%x", sha.Sum(nil))

	if a.Options.Logger != nil {
		a.Options.Logger.Debugf("file checksum: %s", checksum)
	}

	if checksum != v.Checksum {
		return errors.New("checksum file failed")
	}
	return nil
}

func (a AutoUpdate) computeChecksum(tarFilename string) (hash.Hash, error) {
	f, err := os.Open(tarFilename)
	if err != nil {
		return nil, err
	}
	defer func(f *os.File) { _ = f.Close() }(f)

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}

	return h, nil
}

func (a AutoUpdate) writeUpdate(srcFile string, dstFile string) error {
	if a.Options.Logger != nil {
		a.Options.Logger.Infof("[4/4] writing update to %s..", dstFile)
	}

	r, err := os.Open(srcFile)
	if err != nil {
		return err
	}

	stat, err := os.Stat(dstFile)
	if err != nil {
		return err
	}

	w, err := os.OpenFile(dstFile, os.O_CREATE|os.O_RDWR, stat.Mode())
	if err != nil {
		return err
	}

	_, err = io.Copy(w, r)
	if err != nil {
		return err
	}
	return nil
}

func (a AutoUpdate) parseJson(remoteUrl string) (*VersionJson, error) {
	var jsonData VersionJson

	url, err := urlx.NormalizeString(remoteUrl)
	if err != nil {
		return nil, err
	}

	if a.Options.Logger != nil {
		a.Options.Logger.Debugf("Reading remote version url %s", url)
	}

	res, err := http.Get(url)

	if err != nil {
		return nil, err
	}

	defer func(Body io.ReadCloser) { _ = Body.Close() }(res.Body)

	if res.StatusCode > 399 {
		return nil, errors.New(res.Status)
	}

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	// error handling
	err = json.Unmarshal(data, &jsonData)

	if err != nil {
		return nil, err
	}

	semver, err := version.NewVersion(jsonData.Version)
	if err != nil {
		return nil, err
	}

	jsonData.Semver = semver
	return &jsonData, nil
}

type VersionJson struct {
	Version  string
	Checksum string
	Path     string
	Datetime string
	Size     int64
	Semver   *version.Version
}

func (v VersionJson) String() string {
	return fmt.Sprintf("VersionJson{version=%s, datetime=%s,checksum=%s, path=%s", v.Version, v.Datetime, v.Checksum, v.Path)
}

func (v VersionJson) IsValidUpdate() bool {
	return v.Version != "" && len(v.Checksum) == 64 && v.Path != "" && v.Datetime != "" && v.Size > 0 && v.Semver != nil
}

type VersionResult struct {
	Version *VersionJson
	Error   error
}

func (v VersionResult) String() string {
	return fmt.Sprintf("VersionResult{version:%v, error:%v}", v.Version, v.Error)
}
