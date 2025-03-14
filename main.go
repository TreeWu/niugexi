package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/cheggaaa/pb"
	"github.com/chromedp/chromedp"
	"github.com/wujunwei928/parse-video/parser"
)

//https://www.ixigua.com/7338043310168572427?logTag=6840d6236b6cc9908465

type Media struct {
	Href  string `json:"href"`
	Title string `json:"title"`
}

type Download struct {
	Url   string
	Path  string
	Title string
}

type Server struct {
	store *Store
}

var conf Conf

func main() {
	getwd, err := os.Getwd()
	if err != nil {
		return
	}
	fmt.Println(getwd)
	dir, err := os.ReadFile(path.Join(getwd, "conf.json"))
	if err != nil {
		log.Fatal(err)
		return
	}
	err = json.Unmarshal(dir, &conf)
	if err != nil {
		log.Fatal(err)
		return
	}
	store, err := NewStore(conf.Store)
	if err != nil {
		log.Fatal(err)
	}
	s := Server{store: store}
	ctx := context.Background()
	if ok := conf.Mode["geturl"]; ok {
		log.Println("拉取最新的播放页面保存到数据库")
		err = s.GetList(ctx, conf.TargetUrl)
		if err != nil {
			log.Fatal(err)
		}
	}

	if ok := conf.Mode["fillurl"]; ok {
		log.Println("处理没有获取下载连接的")
		err = s.FillDownload(ctx)
		if err != nil {
			log.Fatal(err)
		}
	}

	if ok := conf.Mode["download"]; ok {
		log.Println("本地下载列表和远程比较，补全未下载的文件")
		err = s.DownloadNotExist(ctx)
		if err != nil {
			log.Fatal(err)
		}
	}

	log.Println("完成")
}

// GetList 获取所有的下载链接
func (s *Server) GetList(ctx context.Context, url string) error {
	options := []chromedp.ExecAllocatorOption{
		chromedp.Flag("headless", !conf.ShowBrowser), // debug使用
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/113.0.0.0 Safari/537.36"),
	}
	//初始化参数，先传一个空的数据
	options = append(chromedp.DefaultExecAllocatorOptions[:], options...)

	c, c1 := chromedp.NewExecAllocator(ctx, options...)
	defer c1()

	// create context
	chromeCtx, c2 := chromedp.NewContext(c, chromedp.WithLogf(log.Printf))
	defer c2()

	err := chromedp.Run(chromeCtx, chromedp.Navigate(url),
		chromedp.WaitVisible("div.userDetailV3__main__list"),
	)
	defer chromedp.Run(chromeCtx, chromedp.ActionFunc(func(ctx context.Context) error { return chromedp.Cancel(ctx) }))
	defer chromedp.Run(chromeCtx, chromedp.Stop())

	if err != nil {
		return err
	}
	var hasMore string
	for hasMore != "<div class=\"Feed-footer\">已经到底部，没有更多内容了</div>" {
		err = chromedp.Run(chromeCtx, chromedp.Evaluate(`window.scrollTo(0, document.documentElement.scrollHeight)`, nil),
			chromedp.Sleep(time.Duration(rand.Intn(2)+2)*time.Second),
			chromedp.OuterHTML(".Feed-footer", &hasMore, chromedp.ByQuery),
		)
		if err != nil {
			return err
		}
	}

	var as string
	err = chromedp.Run(chromeCtx, chromedp.OuterHTML(".userDetailV3__main__list", &as, chromedp.ByQueryAll))
	if err != nil {
		return err
	}

	dom, err := goquery.NewDocumentFromReader(strings.NewReader(as))
	if err != nil {
		return err
	}
	nodes := dom.Find("div.HorizontalFeedCard__contentWrapper > div > a").Nodes
	log.Println("获取到", len(nodes), "个视频")
	//因为名字会有重复，所以需要额外计算 集数
	list, err := s.store.List()
	if err != nil {
		return err
	}
	log.Println("数据库中已经存在", len(list), "个视频")
	repeat := make(map[string]int, len(list))
	repeatWebUrl := make(map[string]int, len(list))
	for i := range list {
		repeatWebUrl[list[i].WebUrl] = 0
		originName := list[i].OriginName
		if value, ok := repeat[originName]; ok {
			ii := value + 1
			repeat[originName] = ii
		} else {
			repeat[originName] = 1
		}
	}

	//	repeatCount := 0
	var newInsert int
	defer func() {
		log.Println("新插入", newInsert)
	}()
	for i := range nodes {
		var originName string
		var saveName string
		var webUrl string
		node := nodes[i]
		for index := range node.Attr {

			attribute := node.Attr[index]
			if attribute.Key == "href" {
				webUrl = "https://www.ixigua.com" + attribute.Val
			}
			if attribute.Key == "title" {
				originName = attribute.Val
				saveName = originName
			}
		}

		var ok bool
		_, ok = repeatWebUrl[webUrl]

		// 数据已经存在，并且重复数据已经大于
		if ok {
			//if repeatCount > conf.MaxRepeat {
			//	return nil
			//}
			//repeatCount++
			continue
		}

		//	repeatCount = 0
		for key, value := range conf.Replace {
			saveName = strings.ReplaceAll(saveName, key, value)
		}

		if value, ok := repeat[originName]; ok {
			ii := value + 1
			saveName = saveName + strconv.Itoa(ii)
			repeat[originName] = ii
		} else {
			repeat[originName] = 1
		}

		log.Println("新增数据【", originName, "】的链接：", webUrl)
		video := Video{
			WebUrl:         webUrl,
			MUrl:           strings.ReplaceAll(webUrl, "https://www.ixigua.com/", "https://m.ixigua.com/video/"),
			OriginName:     originName,
			SaveName:       saveName,
			WebDownloadUrl: "",
			MDownloadUrl:   "",
			NeedDownload:   true,
			ErrorMsg:       "",
		}
		if err = s.store.Save([]Video{video}); err != nil {
			log.Println("新增数据错误", err)
			continue
		}
		newInsert++

		repeatWebUrl[webUrl] = 0
	}
	return nil
}

// FillDownload 填充下载地址
func (s *Server) FillDownload(ctx context.Context) error {
	videos, err := s.store.GetEmptyDownload(ctx)
	if err != nil {
		return err
	}

	for _, item := range videos {
		log.Println("获取下载链接", item.SaveName)
		var errs error
		if item.WebDownloadUrl == "" {
			item.WebDownloadUrl, err = s.GetDownloadUrlParse(ctx, item.WebUrl)
			errs = errors.Join(errs, err)
		}
		if item.MUrl == "" || item.MDownloadUrl == "" {
			item.MDownloadUrl, err = s.GetDownloadUrlChrome(ctx, item.MDownloadUrl)
			errs = errors.Join(errs, err)
		}
		if errs != nil {
			item.ErrorMsg = err.Error()
		}
		if err = s.store.Update(item); err != nil {
			log.Println("更新数据错误", err)
			continue
		}
	}
	return nil
}

func (s *Server) GetDownloadUrlChrome(ctx context.Context, webUrl string) (string, error) {
	options := []chromedp.ExecAllocatorOption{
		chromedp.Flag("headless", !conf.ShowBrowser), // debug使用
		chromedp.UserAgent(`Mozilla/5.0 (Linux; Android 6.0; Nexus 5 Build/MRA58N) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/113.0.0.0 Mobile Safari/537.36`),
	}
	//初始化参数，先传一个空的数据
	options = append(chromedp.DefaultExecAllocatorOptions[:], options...)

	c, c1 := chromedp.NewExecAllocator(ctx, options...)
	defer c1()
	chromeCtx, c2 := chromedp.NewContext(c, chromedp.WithLogf(log.Printf))
	defer c2()

	var downloadUrl string
	timeout, _ := context.WithTimeout(chromeCtx, time.Second*20)
	err := chromedp.Run(timeout, chromedp.Navigate(webUrl),
		chromedp.Sleep(time.Second*time.Duration(rand.Intn(5)+2)),
		//chromedp.WaitVisible("video", chromedp.ByQueryAll),
		chromedp.OuterHTML("video", &downloadUrl, chromedp.ByQuery))
	if err != nil {
		return downloadUrl, err
	}
	if downloadUrl != "" {
		dom, err := goquery.NewDocumentFromReader(strings.NewReader(downloadUrl))
		if err != nil {
			return downloadUrl, err
		}
		dom.Find("video[mediatype]").Each(func(i int, selection *goquery.Selection) {
			for i2 := range selection.Nodes[0].Attr {
				if selection.Nodes[0].Attr[i2].Key == "src" {
					downloadUrl = "https:" + selection.Nodes[0].Attr[i2].Val
					break
				}
			}
		})
	} else {
		return "", errors.New("downloadUrl not found")
	}
	return downloadUrl, nil
}

func (s *Server) GetDownloadUrlParse(ctx context.Context, webUrl string) (string, error) {

	u, err := url.Parse(webUrl)
	videoId := strings.Split(u.Path, "/")[1]
	id, err := parser.ParseVideoId(parser.SourceXiGua, videoId)
	if err != nil {
		return "", err
	}
	return id.VideoUrl, nil
}

func (s *Server) DownloadNotExist(ctx context.Context) error {
	// 获取数据库所有数据
	allMedias := make(map[string]Video)
	list, err := s.store.List()
	if err != nil {
		return err
	}
	for i := range list {
		allMedias[list[i].SaveName] = list[i]
	}

	dir, err := os.ReadDir(conf.DownloadPath)
	if err != nil {
		return err
	}
	for i := range dir {
		name := dir[i].Name()
		if strings.HasSuffix(name, ".mp4") {
			s2 := strings.ReplaceAll(name, ".mp4", "")
			delete(allMedias, s2)
		}
	}

	for s2 := range allMedias {
		niugexi := allMedias[s2]
		if !niugexi.NeedDownload {
			continue
		}
		var errs error
		if niugexi.WebDownloadUrl != "" {
			log.Println("下载", niugexi.SaveName)
			err = s.DownloadFile(Download{
				Url:   niugexi.WebDownloadUrl,
				Path:  conf.DownloadPath + niugexi.SaveName + ".mp4",
				Title: niugexi.SaveName + ".mp4",
			})
			errs = errors.Join(errs, err)
		}

		if niugexi.MDownloadUrl != "" && errs != nil {
			log.Println("下载", niugexi.SaveName)
			err = s.DownloadFile(Download{
				Url:   niugexi.MDownloadUrl,
				Path:  conf.DownloadPath + niugexi.SaveName + ".mp4",
				Title: niugexi.SaveName + ".mp4",
			})
			errs = errors.Join(errs, err)
		}
		if errs != nil {
			niugexi.DownloadErr = errs.Error()
		}
		_ = s.store.Update(niugexi)
	}
	return nil

}

// DownloadFile will download a url to a local file. It's efficient because it will
// write as it downloads and not load the whole file into memory.
func (s *Server) DownloadFile(d Download) error {

	client := &http.Client{}

	// 创建一个 GET 请求
	req, err := http.NewRequest("GET", d.Url, nil)
	if err != nil {
		log.Fatal(err)
	}

	// 设置请求头
	req.Header.Set("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("accept-language", "zh-CN,zh;q=0.9")
	req.Header.Set("cache-control", "max-age=0")
	req.Header.Set("if-range", "d897f936db2114cf331695bf62c1f79c")
	req.Header.Set("sec-ch-ua", "Chromium;v=\"122\", \"Not(A:Brand\";v=\"24\", \"Google Chrome\";v=\"122\"")
	req.Header.Set("sec-ch-ua-platform", "Android")
	req.Header.Set("sec-fetch-dest", "document")
	req.Header.Set("sec-fetch-mode", "navigate")
	req.Header.Set("sec-fetch-site", "none")
	req.Header.Set("sec-fetch-user", "?1")
	req.Header.Set("upgrade-insecure-requests", "1")
	req.Header.Set("cookie", "msToken=_3UyugtA1ObN9cC9Ln30k28_rddPaayO4URQfuG7S4bBY3SnWJ8k6uIFidHJ51bEIJ7Brn6sjW9qeWjd6W05V3wQfHAuBS94LunKlibV; ttwid=1%7CljryZJfSOCXbioEHd37n64DLY03lRq4TJ8BL9HUs3Tc%7C1714399395%7C081165af3232224d1d7db3f587d1099d9dc7729bfab0a2f48fea3b6cac53b3c7")

	// 发送请求并获取响应
	resp, err := client.Do(req)

	// Get the data
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 300 {
		return fmt.Errorf("httpcode: %d, status: %s", resp.StatusCode, resp.Status)
	}

	length := resp.Header.Get("Content-Length")
	size, _ := strconv.ParseInt(length, 10, 64)
	body := resp.Body //获取文件内容
	bar := pb.New(int(size)).Prefix(d.Title)
	bar.SetWidth(120)               //设置进度条宽度
	bar.SetRefreshRate(time.Second) //设置刷新速率
	bar.ShowSpeed = true
	bar.SetUnits(pb.U_BYTES)
	bar.Start()
	defer bar.Finish()
	// create proxy reader
	barReader := bar.NewProxyReader(body)
	buffer := bytes.NewBuffer(nil)
	writer := io.Writer(buffer)
	_, err = io.Copy(writer, barReader)

	out, err := os.Create(d.Path)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, buffer)

	return err
}
