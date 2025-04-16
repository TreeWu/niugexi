package main

type Conf struct {
	GetUrl       bool              `json:"getUrl"`
	FillUrl      bool              `json:"fillUrl"`
	Download     bool              `json:"download"`
	Store        DBConfig          `json:"store"`
	Replace      map[string]string `json:"replace"`
	ShowBrowser  bool              `json:"showBrowser"`
	MaxRepeat    int               `json:"maxRepeat"`
	DownloadPath string            `json:"downloadPath"`
	TargetUrl    string            `json:"targetUrl"`
}

type DBConfig struct {
	Type string `json:"type"`
	Dns  string `json:"dns"`
}
