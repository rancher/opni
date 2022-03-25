package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
	"github.com/magefile/mage/sh"
	"github.com/schollz/progressbar/v3"
	"github.com/ttacon/chalk"
)

type webAssetFile struct {
	Path string
	Data []byte
}

const (
	uiBuildImage = "kralicky/opni-monitoring-ui-build"
)

var (
	uiRepo       = "kralicky/opni-ui"
	uiRepoBranch = "updates"
)

func init() {
	if repo, ok := os.LookupEnv("OPNI_UI_REPO"); ok {
		uiRepo = repo
	}
	if branch, ok := os.LookupEnv("OPNI_UI_BRANCH"); ok {
		uiRepoBranch = branch
	}
}

func Dist() error {
	version, err := getOpniUiVersion()
	if err != nil {
		return err
	}
	exists, err := uiImageExists(version)
	if err != nil {
		fmt.Printf(chalk.Red.Color("=>")+" %v\n", err)
		return err
	}
	if !exists {
		if err := buildOrPullUiImage(version); err != nil {
			fmt.Printf(chalk.Red.Color("=>")+" %v\n", err)
			return err
		}
	}
	if err := copyAssetsFromUiImage(version); err != nil {
		fmt.Printf(chalk.Red.Color("=>")+" %v\n", err)
		return err
	}
	if err := compressAssets(); err != nil {
		fmt.Printf(chalk.Red.Color("=>")+" %v\n", err)
		return err
	}
	fmt.Println(chalk.Green.Color("=>") + " Done.")
	return nil
}

func Clean() error {
	if _, err := os.Stat("web/dist/_nuxt"); err == nil {
		fmt.Println("Removing web/dist/_nuxt")
		if err := os.RemoveAll("web/dist/_nuxt"); err != nil {
			return err
		}
	}
	// remove all files in web/dist except .gitignore
	return filepath.Walk("web/dist", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if info.Name() == ".gitignore" {
			return nil
		}
		fmt.Println("Removing", path)
		return os.Remove(path)
	})
}

func numFilesRecursive(dir string) int64 {
	count := int64(0)
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".br") {
			return nil
		}
		count++
		return nil
	})
	return count
}

func getOpniUiVersion() (string, error) {
	fmt.Print(chalk.Blue.Color("=>") + " Fetching latest UI Version... ")
	url := fmt.Sprintf("https://api.github.com/repos/%s/git/refs/heads/%s", uiRepo, uiRepoBranch)
	response, err := http.Get(url)
	if err != nil {
		fmt.Println(chalk.Red.Color("error"))
		return "", err
	}
	defer response.Body.Close()
	var apiResponse struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := json.NewDecoder(response.Body).Decode(&apiResponse); err != nil {
		fmt.Println(chalk.Red.Color("error"))
		return "", err
	}
	version := apiResponse.Object.SHA
	fmt.Println(chalk.Green.Color(version))
	return version, nil
}

func uiImageExists(version string) (bool, error) {
	fmt.Print(chalk.Blue.Color("=>") + " Checking if UI image exists locally... ")
	stderrBuffer := new(bytes.Buffer)
	output := exec.Command("docker", "image", "inspect", fmt.Sprintf("%s:%s", uiBuildImage, version))
	output.Stdout = io.Discard
	output.Stderr = stderrBuffer
	if err := output.Run(); err != nil {
		if strings.Contains(stderrBuffer.String(), "No such image") {
			fmt.Println(chalk.Yellow.Color("no"))
			return false, nil
		}
		fmt.Println(chalk.Red.Color("error"))
		return false, err
	}
	fmt.Println(chalk.Green.Color("yes"))
	return true, nil
}

func buildOrPullUiImage(version string) error {
	fmt.Print(chalk.Blue.Color("=>") + " Checking if UI image exists on remote... ")
	resp, err := http.Get(fmt.Sprintf("https://index.docker.io/v1/repositories/%s/tags/%s", uiBuildImage, version))
	if err != nil {
		fmt.Println(chalk.Red.Color("error"))
		return err
	}
	taggedImage := fmt.Sprintf("%s:%s", uiBuildImage, version)
	if resp.StatusCode == 200 {
		fmt.Println(chalk.Green.Color("yes"))
		stderrBuffer := new(bytes.Buffer)
		output := exec.Command("docker", "pull", taggedImage)
		output.Stdout = io.Discard
		output.Stderr = stderrBuffer
		if err := output.Run(); err != nil {
			if !strings.Contains(stderrBuffer.String(), "manifest unknown") {
				fmt.Println(chalk.Red.Color("error"))
				return err
			}
		} else {
			fmt.Println(chalk.Green.Color("=>") + " Successfully pulled UI image")
			return nil
		}
	} else {
		fmt.Println(chalk.Yellow.Color("no"))
		fmt.Println(chalk.Blue.Color("=>") + " Building UI image...")
		err := sh.RunWith(map[string]string{
			"DOCKER_BUILDKIT": "1",
		}, "docker", "build",
			"-t", taggedImage,
			"-f", "Dockerfile.ui",
			"--build-arg", "REPO="+uiRepo,
			"--build-arg", "BRANCH="+uiRepoBranch,
			".")
		if err != nil {
			return err
		}
		if os.Getenv("GITHUB_ACTIONS") == "true" {
			fmt.Println("::set-output name=push_ui_image::" + taggedImage)
		}
	}
	return nil
}

func copyAssetsFromUiImage(version string) error {
	fmt.Println(chalk.Blue.Color("=>") + " Copying UI assets from image")
	pwd, err := os.Getwd()
	if err != nil {
		return err
	}
	curUser, err := user.Current()
	if err != nil {
		return err
	}
	taggedImage := fmt.Sprintf("%s:%s", uiBuildImage, version)
	err = sh.Run("docker", "run", "-t", "--rm", "-v",
		filepath.Join(pwd, "web/dist")+":/dist",
		taggedImage, fmt.Sprintf("%s:%s", curUser.Uid, curUser.Gid))
	if err != nil {
		return err
	}
	return nil
}

func compressAssets() error {
	count := 0
	uncompressedSize := int64(0)
	compressedSize := int64(0)
	workerCount := runtime.NumCPU()
	uncompressedFiles := make(chan *webAssetFile, workerCount)
	compressedFiles := make(chan *webAssetFile, workerCount)
	bar := progressbar.Default(numFilesRecursive("web/dist"), chalk.Blue.Color("=>")+" Compressing assets")
	writeWorkers := &sync.WaitGroup{}
	for i := 0; i < workerCount; i++ {
		writeWorkers.Add(1)
		go func() {
			defer writeWorkers.Done()
			for {
				cf, ok := <-compressedFiles
				if !ok {
					return
				}
				f, err := os.OpenFile(cf.Path, os.O_WRONLY|os.O_TRUNC, 0644)
				if err != nil {
					panic(err)
				}
				if _, err := f.Write(cf.Data); err != nil {
					panic(err)
				}
				os.Rename(cf.Path, cf.Path+".br")
				info, err := os.Stat(cf.Path + ".br")
				if err != nil {
					panic(err)
				}
				compressedSize += info.Size()
				count++
			}
		}()
	}
	compressWorkers := &sync.WaitGroup{}
	for i := 0; i < workerCount; i++ {
		compressWorkers.Add(1)
		go func() {
			defer compressWorkers.Done()
			for {
				ucf, ok := <-uncompressedFiles
				if !ok {
					return
				}

				buf := new(bytes.Buffer)
				w := brotli.NewWriterLevel(buf, 10)
				w.Write(ucf.Data)
				w.Close()
				bar.Add(1)
				compressedFiles <- &webAssetFile{
					Path: ucf.Path,
					Data: buf.Bytes(),
				}
			}
		}()
	}
	if err := filepath.WalkDir("web/dist/_nuxt", func(path string, d fs.DirEntry, err error) error {
		// skip dirs
		if d.IsDir() {
			return nil
		}
		// compress files with brotli
		if strings.HasSuffix(path, ".br") {
			// already compressed
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		uncompressedSize += info.Size()
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		buf := bytes.NewBuffer(make([]byte, 0, info.Size()))
		_, err = io.Copy(buf, f)
		if err != nil {
			return err
		}
		f.Close()
		uncompressedFiles <- &webAssetFile{
			Path: path,
			Data: buf.Bytes(),
		}
		return nil
	}); err != nil {
		return err
	}
	close(uncompressedFiles)
	compressWorkers.Wait()
	close(compressedFiles)
	writeWorkers.Wait()
	bar.Close()

	fmt.Printf(chalk.Green.Color("=>")+" Compressed %d files (%d MiB -> %d MiB)\n", count, uncompressedSize/1024/1024, compressedSize/1024/1024)
	return nil
}
