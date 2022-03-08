package baidunetdisk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/avast/retry-go"
	"github.com/spf13/cast"
)

type BaiduPanClient struct {
	Context     context.Context
	Index       int
	AppKey      string
	SecretKey   string
	SignKey     string
	AccessToken string
}

const BaiduPanAPIHost = "https://pan.baidu.com"
const BaiduPanCategoryVideo = 1 // 文件类型，1 视频、2 音频、3 图片、4 文档、5 应用、6 其他、7 种子

var BaiduPanSpiderErrors = map[int]string{
	2:  "参数错误",
	-7: "文件或目录无权限",
	-9: "文件或目录不存在",
}

func NewBaiduPanSpiderClient() *BaiduPanClient {
	return &BaiduPanClient{
		Context:     context.Background(),
		AppKey:      "<APP KEY>",
		SecretKey:   "<Secret Key>",
		SignKey:     "<Sign Key>",
		AccessToken: "<Access Token>",
	}
}

type BaiduPanAuth2 struct {
	ExpiresIn     int    `json:"expires_in"`
	RefreshToken  string `json:"refresh_token"`
	AccessToken   string `json:"access_token"`
	SessionSecret string `json:"session_secret"`
	SessionKey    string `json:"session_key"`
	Scope         string `json:"scope"`
}

type BaiduPanListFile struct {
	TkbindID       int               `json:"tkbind_id"`
	Category       int               `json:"category"`
	RealCategory   string            `json:"real_category"`
	Isdir          int               `json:"isdir"`
	ServerFilename string            `json:"server_filename"`
	Path           string            `json:"path"`
	Wpfile         int               `json:"wpfile"`
	ServerAtime    int               `json:"server_atime"`
	ServerCtime    int               `json:"server_ctime"`
	ExtentTinyint7 int               `json:"extent_tinyint7"`
	OwnerID        int64             `json:"owner_id"`
	LocalMtime     int               `json:"local_mtime"`
	Size           int               `json:"size"`
	Unlist         int               `json:"unlist"`
	Share          int               `json:"share"`
	ServerMtime    int               `json:"server_mtime"`
	Pl             int               `json:"pl"`
	LocalCtime     int               `json:"local_ctime"`
	OwnerType      int               `json:"owner_type"`
	OperID         int               `json:"oper_id"`
	FsID           int64             `json:"fs_id"`
	Thumbs         map[string]string `json:"thumbs"`
}

type BaiduPanFileMeta struct {
	Category    int    `json:"category"`
	Dlink       string `json:"dlink"`
	Duration    int    `json:"duration"`
	Filename    string `json:"filename"`
	FsID        int64  `json:"fs_id"`
	Isdir       int    `json:"isdir"`
	Md5         string `json:"md5"`
	OperID      int    `json:"oper_id"`
	Path        string `json:"path"`
	ServerCtime int    `json:"server_ctime"`
	ServerMtime int    `json:"server_mtime"`
	Size        int    `json:"size"`

	WalkIndex int `json:"-"`
}

type BaiduPanSpiderJSON map[string]interface{}

func (b *BaiduPanClient) GetAccessTokenByCode(code string) (accessToken string, err error) {
	res, err := b.APIGet("https://openapi.baidu.com/oauth/2.0/token", BaiduPanSpiderJSON{
		"code":          code,
		"grant_type":    "authorization_code",
		"client_id":     b.AppKey,
		"client_secret": b.SecretKey,
		"redirect_uri":  "oob",
	})
	if err != nil {
		return "", err
	}
	return res["access_token"].(string), nil
}

func (b *BaiduPanClient) Walk(path string, filterIDFunc func(f *BaiduPanListFile) bool, runFunc func(f []*BaiduPanFileMeta) error) error {
	fmt.Printf("🚜 %s \n", path) // nolint
	time.Sleep(200 * time.Millisecond)
	start := 0
	limit := 1000 // max 10000
	for {
		files, err := b.Dir(path, start, limit)
		start += limit
		if err != nil {
			panic(err)
		}
		var fsids []int64
		for _, v := range files {
			if v.Isdir > 0 {
				err = b.Walk(v.Path, filterIDFunc, runFunc)
				if err != nil {
					return err
				}
				continue
			}
			if filterIDFunc(v) {
				fsids = append(fsids, v.FsID)
			}
		}
		fileMetas, err := b.FileMeta(fsids)
		if err != nil {
			panic(err)
		}
		var sortedMetas []*BaiduPanFileMeta
		mappedMetas := make(map[int64]*BaiduPanFileMeta)
		for _, v := range fileMetas {
			mappedMetas[v.FsID] = v
		}
		for _, v := range fsids {
			if mappedMetas[v] == nil {
				continue
			}
			mappedMetas[v].WalkIndex = b.Index
			sortedMetas = append(sortedMetas, mappedMetas[v])
			b.Index++
		}
		err = runFunc(sortedMetas)
		if err != nil {
			return err
		}
		if len(files) < limit {
			break
		}
	}
	return nil
}

// Dir 文件列表 Ref: https://pan.baidu.com/union/doc/nksg0sat9
func (b *BaiduPanClient) Dir(dirName string, start, limit int) (list []*BaiduPanListFile, err error) {
	if limit == 0 {
		limit = 20
	}

	res, err := b.APIGet("/rest/2.0/xpan/file?method=list", BaiduPanSpiderJSON{
		"dir":   dirName,
		"start": start,
		"limit": limit,
		"order": "name",
	})
	if err != nil {
		return nil, err
	}
	data := res["list"]
	if data == nil {
		return nil, nil
	}
	l := data.([]interface{})
	for _, v := range l {
		f := &BaiduPanListFile{}
		d, _ := json.Marshal(v)
		_ = json.Unmarshal(d, &f)
		list = append(list, f)
	}
	return list, err
}

// MediaDir 文件列表 Ref: https://pan.baidu.com/union/doc/nksg0sat9
func (b *BaiduPanClient) MediaDir(dirName string, page, limit int, method string) (list []*BaiduPanListFile, err error) {
	if limit == 0 {
		limit = 20
	}
	if method == "" {
		method = "list"
	}
	res, err := b.APIGet("/rest/2.0/xpan/file?method="+method, BaiduPanSpiderJSON{
		"parent_path": dirName,
		"page":        page,
		"num":         limit,
	})
	if err != nil {
		return nil, err
	}
	data := res["info"]
	if data == nil {
		return nil, nil
	}
	l := data.([]interface{})
	for _, v := range l {
		f := &BaiduPanListFile{}
		d, _ := json.Marshal(v)
		_ = json.Unmarshal(d, &f)
		list = append(list, f)
	}
	return list, err
}

// FileMeta 文件信息 Ref: https://pan.baidu.com/union/doc/Fksg0sbcm
func (b *BaiduPanClient) FileMeta(fsids []int64) (list []*BaiduPanFileMeta, err error) {
	fsidStrs := make([]string, 0, len(fsids))
	for _, v := range fsids {
		fsidStrs = append(fsidStrs, cast.ToString(v))
	}
	res, err := b.APIGet("/rest/2.0/xpan/multimedia?method=filemetas", BaiduPanSpiderJSON{
		"fsids":     "[" + strings.Join(fsidStrs, ",") + "]",
		"dlink":     1,
		"needmedia": 1,
	})
	if err != nil {
		return nil, err
	}
	data := res["list"]
	if data == nil {
		return nil, nil
	}
	l := data.([]interface{})
	for _, v := range l {
		f := &BaiduPanFileMeta{}
		d, _ := json.Marshal(v)
		err = b.JSONUnmarshal(d, &f)
		if err != nil {
			return nil, err
		}
		list = append(list, f)
	}
	return list, err
}

func (b *BaiduPanClient) APIGet(path string, query BaiduPanSpiderJSON) (res BaiduPanSpiderJSON, err error) {
	if b.AccessToken == "" {
		panic("access token 不存在")
	}
	queryStrs := make([]string, 0, len(query))
	if query == nil {
		query = BaiduPanSpiderJSON{}
	}
	query["access_token"] = b.AccessToken
	for k, v := range query {
		queryStrs = append(queryStrs, k+"="+url.QueryEscape(fmt.Sprintf("%v", v)))
	}
	queryStr := strings.Join(queryStrs, "&")
	if strings.Contains(path, "?") {
		path = path + "&" + queryStr
	} else {
		path = path + "?" + queryStr
	}
	httpURL := BaiduPanAPIHost + path

	var response *http.Response
	err = retry.Do(func() error {
		defer func() {
			if e := recover(); e != nil {
				err = fmt.Errorf("%s", e)
			}
		}()
		response, err = http.Get(httpURL) // nolint
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			return fmt.Errorf("bad request, status code: %d", response.StatusCode)
		}
		body, _ := ioutil.ReadAll(response.Body)
		err = b.JSONUnmarshal(body, &res)
		if err != nil {
			return err
		}
		errno, _ := res["errno"].(json.Number).Int64()
		if errno != 0 {
			msg := BaiduPanSpiderErrors[int(errno)]
			if res["errmsg"] != nil {
				msg = res["errmsg"].(string)
			}
			return fmt.Errorf("errno: %v, request_id: %v， msg: %s", res["errno"], res["request_id"], msg)
		}
		return nil
	}, retry.Attempts(3), retry.Delay(time.Second*2), retry.DelayType(retry.BackOffDelay))

	return res, err
}

func (b *BaiduPanClient) JSONUnmarshal(data []byte, v interface{}) error {
	buffer := bytes.NewBuffer(data)
	decoder := json.NewDecoder(buffer)
	decoder.UseNumber()
	return decoder.Decode(&v)
}
