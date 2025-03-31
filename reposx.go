/*
* reposx
*
* aceinetx (2022-2025)
* SPDX-License-Identifier: GPL-2.0
 */

package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// XML structures
type URL struct {
	URL string `xml:",chardata"`
}

type Package struct {
	Name string `xml:"name,attr"`
	AMD  struct {
		URL URL `xml:"url"`
	} `xml:"amd"`
	ARM struct {
		URL URL `xml:"url"`
	} `xml:"arm"`
}

type Packages struct {
	XMLName  xml.Name  `xml:"packages"`
	Packages []Package `xml:"package"`
}

const base_url = "http://93.100.25.80:8080/reposx/"

func ensureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

func getLocalDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".local", "reposx"), nil
}

func downloadFile(url, filepath string) error {
	fmt.Printf("[...] downloading %v\n", url)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func extractTarGz(gzipStream io.Reader, destPath string) error {
	uncompressedStream, err := gzip.NewReader(gzipStream)
	if err != nil {
		return err
	}
	defer uncompressedStream.Close()

	tarReader := tar.NewReader(uncompressedStream)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		path := filepath.Join(destPath, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			outFile, err := os.Create(path)
			if err != nil {
				return err
			}
			defer outFile.Close()
			if _, err := io.Copy(outFile, tarReader); err != nil {
				return err
			}
			if err := os.Chmod(path, 0755); err != nil {
				return err
			}
		}
	}
	return nil
}

func downloadAndParseXML(force bool) (*Packages, error) {
	localDir, err := getLocalDir()
	if err != nil {
		return nil, err
	}

	if err := ensureDir(localDir); err != nil {
		return nil, err
	}

	xmlPath := filepath.Join(localDir, "index.xml")

	// Check if XML exists and force is false
	if !force {
		if _, err := os.Stat(xmlPath); err == nil {
			xmlFile, err := os.Open(xmlPath)
			if err != nil {
				return nil, err
			}
			defer xmlFile.Close()

			var packages Packages
			if err := xml.NewDecoder(xmlFile).Decode(&packages); err != nil {
				return nil, err
			}
			return &packages, nil
		}
	}

	// Download XML
	if err := downloadFile(base_url+"index.xml", xmlPath); err != nil {
		return nil, err
	}

	xmlFile, err := os.Open(xmlPath)
	if err != nil {
		return nil, err
	}
	defer xmlFile.Close()

	var packages Packages
	if err := xml.NewDecoder(xmlFile).Decode(&packages); err != nil {
		return nil, err
	}

	return &packages, nil
}

func installPackage(packageName string, packages *Packages) error {
	// Find package
	var targetPackage *Package
	for _, pkg := range packages.Packages {
		if pkg.Name == packageName {
			targetPackage = &pkg
			break
		}
	}

	if targetPackage == nil {
		return fmt.Errorf("package %s not found", packageName)
	}

	// Get URL based on architecture
	var downloadURL string
	arch := runtime.GOARCH
	switch arch {
	case "386":
		downloadURL = targetPackage.AMD.URL.URL
	case "arm":
		downloadURL = targetPackage.ARM.URL.URL
	case "amd64":
		downloadURL = targetPackage.AMD.URL.URL
	case "arm64":
		downloadURL = targetPackage.ARM.URL.URL
	default:
		return fmt.Errorf("[ ! ] unsupported architecture: %s", arch)
	}

	localDir, err := getLocalDir()
	if err != nil {
		return err
	}

	// Download package
	packagePath := filepath.Join(localDir, fmt.Sprintf("%s.tar.gz", packageName))
	if err := downloadFile(downloadURL, packagePath); err != nil {
		return err
	}

	// Extract package
	fmt.Printf("[...] extracting %v\n", packagePath)
	file, err := os.Open(packagePath)
	if err != nil {
		return err
	}
	defer file.Close()

	extractPath := filepath.Join(localDir, packageName)
	if err := ensureDir(extractPath); err != nil {
		return err
	}

	if err := extractTarGz(file, extractPath); err != nil {
		return err
	}

	// Clean up downloaded archive
	return os.Remove(packagePath)
}

func getPackagePaths() (string, error) {
	localDir, err := getLocalDir()
	if err != nil {
		return "", err
	}

	entries, err := os.ReadDir(localDir)
	if err != nil {
		return "", err
	}

	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			paths = append(paths, filepath.Join(localDir, entry.Name()))
		}
	}

	return strings.Join(paths, ":"), nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("reposx by aceinet (2022-2025)")
		fmt.Println("usage:")
		fmt.Println("  update              Update package list")
		fmt.Println("  install <package>   Install package")
		fmt.Println("  paths               Get package paths for shells")
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "update":
		_, err := downloadAndParseXML(true)
		if err != nil {
			fmt.Printf("[ ! ] Error updating package list: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("[...] Package list updated successfully")

	case "install":
		if len(os.Args) != 3 {
			fmt.Println("usage: install <package>")
			os.Exit(1)
		}

		packages, err := downloadAndParseXML(false)
		if err != nil {
			fmt.Printf("[ ! ] Error reading package list: %v\n", err)
			os.Exit(1)
		}

		packageName := os.Args[2]
		if err := installPackage(packageName, packages); err != nil {
			fmt.Printf("[ ! ] Error installing package %s: %v\n", packageName, err)
			os.Exit(1)
		}
		fmt.Printf("[...] Package %s installed successfully\n", packageName)

	case "paths":
		paths, err := getPackagePaths()
		if err != nil {
			fmt.Printf("[ ! ] Error getting package paths: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(paths)

	default:
		fmt.Printf("unknown command: %s\n", command)
		os.Exit(1)
	}
}
