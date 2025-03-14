package main

type Conf struct {
	Mode         map[string]bool   `json:"mode"`
	Store        DBConfig          `json:"store"`
	Replace      map[string]string `json:"replace"`
	ShowBrowser  bool              `json:"showBrowser"`
	MaxRepeat    int               `json:"maxRepeat"`
	DownloadPath string            `json:"downloadPath"`
	TargetUrl    string            `json:"targetUrl"`
}

type DBConfig struct {
	Dns string `json:"dns"`
}
