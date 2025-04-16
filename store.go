package main

import (
	"context"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type Store struct {
	db *gorm.DB
}

type Video struct {
	gorm.Model
	WebUrl         string `gorm:"column:web_url;type:varchar(1024)" json:"webUrl"`
	MUrl           string `gorm:"column:m_url;type:varchar(1024);comment:手机端url" json:"MUrl"`
	OriginName     string `gorm:"column:origin_name;type:varchar(255);comment:原始名称" json:"originName"`
	SaveName       string `gorm:"column:save_name;type:varchar(255);comment:保存名称" json:"title"`
	WebDownloadUrl string `gorm:"column:web_download_url;type:varchar(1024);comment:浏览器下载地址" json:"webDownloadUrl"`
	MDownloadUrl   string `gorm:"column:m_download_url;type:varchar(1024);comment:手机端下载地址" json:"MDownloadUrl"`
	NeedDownload   bool   `gorm:"column:need_download" json:"needDownload"`
	ErrorMsg       string `gorm:"column:error_msg;type:varchar(512)" json:"errorMsg"`
	DownloadErr    string `gorm:"column:download_err;type:varchar(512)" json:"downloadErr"`
}

func (m *Video) TableName() string {
	return "biz_videos"
}

func NewStore(conf DBConfig) (*Store, error) {
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       conf.Dns, // DSN data source name
		DefaultStringSize:         256,      // string 类型字段的默认长度
		DisableDatetimePrecision:  true,     // 禁用 datetime 精度，MySQL 5.6 之前的数据库不支持
		DontSupportRenameIndex:    true,     // 重命名索引时采用删除并新建的方式，MySQL 5.7 之前的数据库和 MariaDB 不支持重命名索引
		DontSupportRenameColumn:   true,     // 用 `change` 重命名列，MySQL 8 之前的数据库和 MariaDB 不支持重命名列
		SkipInitializeWithVersion: false,    // 根据当前 MySQL 版本自动配置
	}), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	err = db.AutoMigrate(&Video{})
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Save(media []Video) error {
	return s.db.CreateInBatches(media, 100).Error
}

func (s *Store) List() ([]Video, error) {
	var medias []Video
	err := s.db.Model(&Video{}).Where("").Scan(&medias).Error
	return medias, err
}

func (s *Store) Update(v Video) error {
	return s.db.Model(&Video{}).Where("id =?", v.ID).Updates(&v).Error
}

func (s *Store) findByWebUrl(ctx context.Context, weburl string) (n Video, e error) {
	e = s.db.WithContext(ctx).Model(&Video{}).Where(Video{WebUrl: weburl}).First(&n).Error
	return
}

func (s *Store) GetEmptyDownload(ctx context.Context) ([]Video, error) {
	var medias []Video
	err := s.db.WithContext(ctx).Debug().Model(&Video{}).Select("id ,web_url,save_name").Where("length(m_download_url) = 0  or length(web_download_url) = 0").Scan(&medias).Error

	return medias, err

}
