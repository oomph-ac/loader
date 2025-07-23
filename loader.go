package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"time"

	"github.com/oomph-ac/loader/api"
)

func main() {
	_ = os.Mkdir(".oomph-cache", 0755)

	branch := flag.String("branch", "stable", "The branch to download the Oomph binary from.")
	useCached := flag.Bool("use-cache", false, "If true, only use the local cache and do not attempt to download the latest Oomph binary.")
	flag.Parse()

	binaryAssetId := fmt.Sprintf("production_binary_%s_%s_%s", *branch, runtime.GOOS, runtime.GOARCH)
	binaryPath := ".oomph-cache/" + binaryAssetId
	fmt.Println("Searching for", binaryAssetId)

	var (
		binaryHash       string
		binaryDownloaded bool
	)
	if dat, err := os.ReadFile(binaryPath); err == nil {
		binaryHash = fmt.Sprintf("%X", sha256.Sum256(dat))
	}

	if !(*useCached) {
		api.CallEndpoint(
			"https://api.oomph.ac/assets",
			api.AssetRequest{
				AssetId:        binaryAssetId,
				LocalAssetHash: binaryHash,
			},
			func(resp api.AssetResponse) {
				if resp.CacheHit {
					fmt.Println("Latest version of Oomph is already installed.")
					binaryDownloaded = true
					return
				}
				fmt.Printf("Received asset response (%.2fMB)\n", float64(len(resp.AssetPayload))/1024/1024)

				dec, err := base64.StdEncoding.DecodeString(resp.AssetPayload)
				if err != nil {
					fmt.Printf("Failed to decode asset payload: %v\nResorting to cache.\n", err)
					return
				}

				compressedBytes := bytes.NewBuffer(dec)
				r, err := zlib.NewReader(compressedBytes)
				if err != nil {
					fmt.Printf("Failed to create zlib reader: %v\nResorting to cache.\n", err)
					return
				}
				defer r.Close()

				binaryData, err := io.ReadAll(r)
				if err != nil {
					fmt.Printf("Failed to decompress asset payload: %v\nResorting to cache.\n", err)
					return
				}

				if err := os.WriteFile(binaryPath, binaryData, 0755); err != nil {
					fmt.Printf("Failed to write Oomph binary to cache: %v\n", err)
					return
				}
				fmt.Printf("Latest version of Oomph downloaded successfully (%.2fMB)\n", float64(len(binaryData))/1024/1024)
				binaryDownloaded = true
			},
			func(message string) {
				fmt.Printf("Unable to retrieve latest Oomph binary: %s\n", message)
			},
			func(err error) {
				fmt.Printf("Error occurred while retrieving latest Oomph binary: %v\n", err)
			},
		)
	}

	if !binaryDownloaded {
		if len(binaryHash) == 0 {
			fmt.Println("No Oomph proxy binary found in local cache, please try again later.")
			return
		}
		if !(*useCached) {
			fmt.Println("Download failed, resorting to local cache.")
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		wd = "." // ?????
	}
	cmd, err := os.StartProcess(binaryPath, []string{binaryPath}, &os.ProcAttr{
		Env:   os.Environ(),
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		Dir:   wd,
	})
	if err != nil {
		fmt.Printf("Failed to start Oomph binary: %v\n", err)
		return
	}

	var (
		finishChan    = make(chan struct{}, 1)
		interruptChan = make(chan os.Signal, 1)
	)
	signal.Notify(interruptChan, os.Interrupt, os.Kill)
	go func() {
		if status, err := cmd.Wait(); err != nil {
			fmt.Printf("Oomph binary exited with status: %v\n", status.ExitCode())
		} else {
			fmt.Println("Oomph binary stopped successfully.")
		}
		finishChan <- struct{}{}
	}()

	select {
	case <-finishChan:
		break
	case <-interruptChan:
		completionChan := make(chan struct{}, 1)
		go func() {
			if err := cmd.Signal(os.Interrupt); err != nil {
				fmt.Printf("Failed to send interrupt signal to Oomph binary: %v\n", err)
				return
			}
			completionChan <- struct{}{}
		}()

		select {
		case <-completionChan:
			fmt.Println("Oomph stopped successfully.")
		case <-time.After(5 * time.Second):
			fmt.Println("Oomph did not stop in time, forcefully terminating.")
			_ = cmd.Kill()
			fmt.Println("Oomph forcefully terminated.")
		}
	}
}
