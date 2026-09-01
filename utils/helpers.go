// utils/helpers.go
package utils

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
)

const (
	alpineURL = "https://dl-cdn.alpinelinux.org/alpine/v3.19/releases/x86_64/alpine-minirootfs-3.19.1-x86_64.tar.gz"
)





func CheckAndSetupRootFS(lowerDir string) {
	if _, err := os.Stat(lowerDir); os.IsNotExist(err) {
		fmt.Println("Base rootfs not found. Triggering automated setup...")
		if err := setupRootFS(lowerDir); err != nil {
			fmt.Printf("Failed to setup rootfs: %v\n", err)
			os.Exit(1)
		}
	}
}


func setupRootFS(targetDir string) error {
	os.MkdirAll(targetDir, 0755)

	fmt.Println("Downloading Alpine Linux minirootfs...")
	resp, err := http.Get(alpineURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	gzipReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	fmt.Println("Extracting rootfs layers...")
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(targetDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, header.FileInfo().Mode())
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		case tar.TypeSymlink:
			os.Symlink(header.Linkname, target)
		}
	}

	fmt.Println("RootFS successfully automated and configured!")
	return nil
}


func MountOverlayFS(lowerDir, upperDir, workDir, mergedDir string) {
	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerDir, upperDir, workDir)
	err := syscall.Mount("overlay", mergedDir, "overlay", 0, opts)
	if err != nil {
		fmt.Printf("Overlay Mount Error: %v\n", err)
		os.Exit(1)
	}
}




func IsolateAndPivotRoot(mergedDir string) {
	err := syscall.Chroot(mergedDir)
	if err != nil {
		fmt.Printf("Chroot Error: %v\n", err)
		os.Exit(1)
	}

	os.Chdir("/")
	syscall.Mount("proc", "proc", "proc", 0, "")
	syscall.Sethostname([]byte("my-isolated-container"))
}

// CreateContainerDirs 
func CreateContainerDirs(upper, work, merged string) {
	os.MkdirAll(upper, 0755)
	os.MkdirAll(work, 0755)
	os.MkdirAll(merged, 0755)
}

func CleanupContainer(containerID string) {
	basePath, err := os.Getwd()
	if err != nil {
		fmt.Printf("⚠️ Error getting current working directory: %v\n", err)
		return
	}

	// filepath.Clean ব্যবহার করে পাথের বাড়তি / বা ডট ডিলিট করা
	mergedDir := filepath.Clean(filepath.Join(basePath, "containers", containerID, "merged"))
	containerDir := filepath.Clean(filepath.Join(basePath, "containers", containerID))

	fmt.Printf("\n🧹 Starting cleanup for container: %s\n", containerID)

	// ১. কার্নেল থেকে সঠিক পাথে আনমাউন্ট করা
	err = syscall.Unmount(mergedDir, syscall.MNT_DETACH)
	if err != nil {
		fmt.Printf("⚠️ Warning: Failed to unmount %s: %v\n", mergedDir, err)
	} else {
		fmt.Println("✅ Successfully unmounted container filesystem.")
	}

	// ২. ডিলিট করা
	err = os.RemoveAll(containerDir)
	if err != nil {
		fmt.Printf("⚠️ Warning: Failed to delete container directory: %v\n", err)
	} else {
		fmt.Println("✅ Successfully deleted temporary container files.")
	}
}
