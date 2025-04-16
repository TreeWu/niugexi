package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
	"github.com/wujunwei928/parse-video/parser"
)

//go:generate fyne package -os windows -icon xigua.png
//go:generate upx -9  xigua.exe

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
	store   *Store
	running atomic.Bool
	stats   Stats
	cancel  context.CancelFunc
}

type Stats struct {
	CurFile         string
	CurFileSize     int64
	CurFileDownSize int64
	BytesCopied     int64
	LastUpdateTime  time.Time
	LastBytes       int64
	Speed           float64
	TotalFiles      int64
	DownloadedFiles int64
}

func main() {

	conf := Conf{Store: DBConfig{
		Dns: "root:root@tcp(127.0.0.1:3306)/videos?charset=utf8mb4&parseTime=True&loc=Local",
	},
		MaxRepeat: 5,
		Replace: map[string]string{
			"山歌":   "",
			"牛歌剧":  "",
			"广西":   "",
			"弘扬":   "",
			"地方":   "",
			"特色":   "",
			"平南":   "",
			"牛歌戏":  "",
			"非遗":   "",
			"文化":   "",
			"非物质":  "",
			"遗产":   "",
			"《":    "",
			"》":    "",
			"戏曲":   "",
			"，":    "",
			" ":    "",
			"精彩":   "",
			"传承":   "",
			"区粹":   "",
			"戏剧":   "",
			"现代版":  "",
			"民间":   "",
			"现代":   "",
			"第一":   "第1",
			"第二":   "第2",
			"第三":   "第3",
			"第四":   "第4",
			"第五":   "第5",
			"第六":   "第6",
			"第七":   "第7",
			"第八":   "第8",
			"第九":   "第9",
			"第十一":  "第11",
			"第十二":  "第12",
			"第十三":  "第13",
			"第十四":  "第14",
			"第十五":  "第15",
			"第十六":  "第16",
			"第十七":  "第17",
			"第十八":  "第18",
			"第十九":  "第19",
			"第二十一": "第21",
			"第二十二": "第22",
			"第二十三": "第23",
			"第十集":  "第10集",
			"第二十集": "第20集",
			"第十节":  "第10节",
			"第二十节": "第20节",
		},
	}

	myApp := app.NewWithID("xigua-shrimp")
	window := myApp.NewWindow("西瓜下载工具")

	form := widget.NewForm()

	home := widget.NewEntry()
	home.SetPlaceHolder("输入视频主页")
	home.SetText(conf.TargetUrl)
	form.AppendItem(widget.NewFormItem("视频主页", home))

	showBrowser := widget.NewCheck("", func(b bool) {
		conf.ShowBrowser = b
	})
	showBrowser.SetChecked(conf.ShowBrowser)
	form.AppendItem(widget.NewFormItem("显示浏览器", showBrowser))

	getUrl := widget.NewCheck("", func(b bool) {
		conf.GetUrl = b
	})
	getUrl.SetChecked(conf.GetUrl)
	form.AppendItem(widget.NewFormItem("获取链接", getUrl))

	fillUrl := widget.NewCheck("", func(b bool) {
		conf.FillUrl = b
	})
	fillUrl.SetChecked(conf.FillUrl)
	form.AppendItem(widget.NewFormItem("填充地址", fillUrl))

	download := widget.NewCheck("", func(b bool) {
		conf.Download = b
	})
	download.SetChecked(conf.Download)
	form.AppendItem(widget.NewFormItem("下载文件", download))

	savePath := widget.NewEntry()
	savePath.SetPlaceHolder("文件保存地址")
	savePath.SetText(conf.DownloadPath)
	savePath.Disable()
	selectButton := widget.NewButton("浏览...", func() {
		folderDialog := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil {
				dialog.ShowError(err, window)
				return
			}
			if uri == nil {
				log.Println("用户取消了选择")
				return
			}
			savePath.SetText(uri.Path())
		}, window)
		folderDialog.Show()
	})
	pathRow := container.NewHSplit(
		savePath,
		selectButton,
	)
	pathRow.SetOffset(0.8)
	form.AppendItem(widget.NewFormItem("文件保存地址", pathRow))

	statsLabel := widget.NewLabel("")

	var startButton *widget.Button
	s := Server{running: atomic.Bool{}}

	progressBar := widget.NewProgressBar()
	progressBar.Min = 0
	progressBar.Max = 100

	statusLabel := widget.NewLabel("准备下载...")
	currentFileLabel := widget.NewLabel("")
	speedLabel := widget.NewLabel("")

	stopButton := widget.NewButton("停止", func() {
		if s.running.Load() && s.cancel != nil {
			s.cancel()
		}
	})

	startButton = widget.NewButton("开始", func() {
		startButton.Disable()

		go func() {
			defer fyne.Do(func() { startButton.Enable() })
			if s.running.Load() {
				return
			}
			conf.TargetUrl = home.Text
			conf.DownloadPath = savePath.Text
			s.stats = Stats{}
			s.running.Store(true)

			go func() {
				for {
					fyne.Do(func() {
						if s.stats.TotalFiles > 0 {
							progress := float64(s.stats.DownloadedFiles/s.stats.TotalFiles) * 100
							progressBar.SetValue(progress)
							statusLabel.SetText(fmt.Sprintf("正在下载: %d/%d 文件", s.stats.DownloadedFiles, s.stats.TotalFiles))
							currentFileLabel.SetText("当前文件: " + s.stats.CurFile)
							speedLabel.SetText(fmt.Sprintf("速度: %.2f MB/s", s.stats.Speed))
							statsLabel.SetText(fmt.Sprintf("已下载: %d 文件", s.stats.DownloadedFiles))
						}
					})
					if !s.running.Load() {
						return
					}
					time.Sleep(time.Second)
				}
			}()

			defer s.running.Store(false)
			if s.store == nil {
				store, err := NewStore(conf.Store)
				if err != nil {
					fyne.Do(func() {
						statsLabel.SetText(err.Error())
					})
					return
				}
				s.store = store
			}

			var ctx context.Context
			ctx, s.cancel = context.WithCancel(context.Background())
			if conf.GetUrl {
				log.Println("拉取最新的播放页面保存到数据库")
				err := s.GetList(ctx, conf)
				if err != nil {
					fyne.Do(func() {
						statsLabel.SetText(err.Error())
					})
					return
				}
			}

			if conf.FillUrl {
				log.Println("处理没有获取下载连接的")
				err := s.FillDownload(ctx, conf)
				if err != nil {
					fyne.Do(func() {
						statsLabel.SetText(err.Error())
					})
					return
				}
			}

			if conf.Download {
				log.Println("本地下载列表和远程比较，补全未下载的文件")
				err := s.DownloadNotExist(ctx, conf)
				if err != nil {
					fyne.Do(func() {
						statsLabel.SetText(err.Error())
					})
					return
				}
			}
		}()

	})

	box := container.NewVBox(
		form,
		startButton,
		stopButton,
		progressBar,
		statusLabel,
		currentFileLabel,
		speedLabel,
		statsLabel,
	)
	window.SetContent(box)
	window.Resize(fyne.NewSize(600, 400))
	window.ShowAndRun()
}

// GetList 获取所有的下载链接
func (s *Server) GetList(ctx context.Context, conf Conf) error {
	if conf.TargetUrl == "" {
		return errors.New("请输入视频主页")
	}
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

	err := chromedp.Run(chromeCtx, chromedp.Navigate(conf.TargetUrl),
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
func (s *Server) FillDownload(ctx context.Context, conf Conf) error {
	videos, err := s.store.GetEmptyDownload(ctx)
	if err != nil {
		return err
	}

	for _, item := range videos {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		log.Println("获取下载链接", item.SaveName)
		var errs error
		if item.WebDownloadUrl == "" {
			item.WebDownloadUrl, err = s.GetDownloadUrlParse(ctx, item.WebUrl)
			errs = errors.Join(errs, err)
		}
		if item.MUrl == "" || item.MDownloadUrl == "" {
			item.MDownloadUrl, err = s.GetDownloadUrlChrome(ctx, conf, item.MDownloadUrl)
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

func (s *Server) GetDownloadUrlChrome(ctx context.Context, conf Conf, webUrl string) (string, error) {
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

func (s *Server) DownloadNotExist(ctx context.Context, conf Conf) error {
	if conf.DownloadPath == "" {
		return errors.New("请填写保存地址")
	}
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

	s.stats.TotalFiles = int64(len(allMedias))
	for s2 := range allMedias {

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		s.stats.DownloadedFiles++
		niugexi := allMedias[s2]
		if !niugexi.NeedDownload {
			continue
		}
		var errs error

		niugexi.WebDownloadUrl, _ = s.GetDownloadUrlParse(ctx, niugexi.WebUrl)
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
	s.stats.CurFile = d.Title
	s.stats.CurFileSize = 0
	s.stats.CurFileDownSize = 0
	s.stats.BytesCopied = 0
	s.stats.LastUpdateTime = time.Time{}
	s.stats.LastBytes = 0
	s.stats.Speed = 0

	client := &http.Client{}

	// 创建一个 GET 请求
	req, err := http.NewRequest("GET", d.Url, nil)
	if err != nil {
		return err
	}

	//// 设置请求头
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
	s.stats.CurFileSize = size
	s2 := d.Path + ".download"
	out, err := os.Create(s2)
	if err != nil {
		return err
	}
	defer os.Rename(s2, d.Path)
	defer out.Close()
	copier := &statsWriter{
		writer: out,
		stats:  &s.stats,
	}
	_, err = io.Copy(copier, resp.Body)
	return err
}

type statsWriter struct {
	writer io.Writer
	stats  *Stats
}

func (sw *statsWriter) Write(p []byte) (int, error) {
	n, err := sw.writer.Write(p)
	if n > 0 {
		sw.stats.BytesCopied += int64(n)

		now := time.Now()
		if !sw.stats.LastUpdateTime.IsZero() {
			elapsed := now.Sub(sw.stats.LastUpdateTime).Seconds()
			if elapsed > 0 {
				bytesDiff := sw.stats.BytesCopied - sw.stats.LastBytes
				sw.stats.Speed = (float64(bytesDiff) / 1024 / 1024) / elapsed
			}
		}
		sw.stats.LastUpdateTime = now
		sw.stats.LastBytes = sw.stats.BytesCopied
	}
	return n, err
}
