package main

import (
	"os"
	"testing"
)

func TestDownload(t *testing.T) {
	create, err := os.Create("d:/妹仔想当主人婆21.mp4.download")
	if err != nil {
		return
	}
	create.Close()
}
