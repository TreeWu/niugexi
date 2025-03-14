package main

import (
	"os"
	"strings"
	"testing"
)

func TestFormatName(t *testing.T) {
	dir, err := os.ReadDir(conf.DownloadPath)
	if err != nil {
		return
	}
	for i := range dir {
		entry := dir[i]
		if strings.HasSuffix(entry.Name(), ".mp4") {
			saveName := entry.Name()
			for key, value := range conf.Replace {
				saveName = strings.ReplaceAll(saveName, key, value)
			}
			err := os.Rename(conf.DownloadPath+entry.Name(), conf.DownloadPath+saveName)
			if err != nil {
				return
			}
		}
	}
}

func TestFormatDBName(t *testing.T) {
	store, err := NewStore(conf.Store)
	if err != nil {
		return
	}
	list, err := store.List()
	if err != nil {
		return
	}

	for i := range list {
		entry := list[i]
		for key, value := range conf.Replace {
			entry.SaveName = strings.ReplaceAll(entry.SaveName, key, value)
		}
		err := store.Update(entry)
		if err != nil {
			return
		}
	}
}

func TestDownload(t *testing.T) {
	download := Download{
		Url:   "https://v11-colds.douyinvod.com/b60012efae85de810512aea724c38ad8/67cf33ff/video/tos/cn/tos-cn-ve-4/oILqAGmAIsfeCXtAQrnKhBASlj8fUEblaAOnVD/?a=1128&ch=0&cr=0&dr=0&er=0&lr=unwatermarked&cd=0%7C0%7C0%7C0&cv=1&br=1183&bt=1183&cs=0&ds=3&ft=3p6wFGUUmfusdPQ6OQ01HHopidIBF_VkmA5~eTv7ThWH6P7TrW&mime_type=video_mp4&qs=0&rc=OjY7NDZmNmk0aTVkOTloNEBpanNocDQ6ZnR5cTMzNDczM0A0LjItYTUxNV4xMGJiMy9fYSNrX15pcjRvXl5gLS1kLS9zcw%3D%3D&btag=c0010e000b8001&cdn_type=2&cquery=100g&dy_q=1741588067&l=20250310142747A39AF1B5DD0847BC4E4E&pwid=196&req_cdn_type=r",
		Path:  "D:/广西非遗戏曲平南牛歌戏《刘备借荆州》第14集.mp4",
		Title: "广西非遗戏曲平南牛歌戏《刘备借荆州》第14集",
	}

	s := &Server{}
	err := s.DownloadFile(download)
	if err != nil {
		t.Error(err)
	}
}
