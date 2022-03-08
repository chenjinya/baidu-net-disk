# baidu-net-disk
百度网盘 OpenAPI 简单小工具

## Usage 

```go
c := baidunetdisk.NewBaiduPanSpiderClient()
var sections []interface
_ = c.Walk(baiduPanPath, func(f *baidunetdisk.BaiduPanListFile) bool {
    // 过滤视频类型
    if f.Category != baidunetdisk.BaiduPanCategoryVideo {
        return false
    }
    return true
}, func(metas []*baidunetdisk.BaiduPanFileMeta) error {
    for _, v := range metas {
        println(v)
    }
    return nil
})
```

## 获取授权信息

- 首先打开网页登陆百度账号
- 通过网页手动授权OAuth2.0
https://openapi.baidu.com/oauth/2.0/authorize?response_type=code&client_id=xxxxx&redirect_uri=oob&scope=basic,netdisk&display=popup

client_id:是 APP key

redirect_uri: 填写oob即可

scope 值指定权限,basic 就是获取的基本用户信息之类的,netdisk 就是获取网盘的信息。

第一次授权之后，页面会展示授权 code 。 每一个Authorization Code的有效期为10分钟，并且只能使用一次，再次使用将无效。

- 获取AccessToken
https://openapi.baidu.com/oauth/2.0/token?grant_type=authorization_code&code=xxxx&client_id=xxxx&client_secret=xxx&redirect_uri=oob

code 就是上面获取的 code。
返回值为 OAuth 的结构，保存里面的 Access Token.

- 使用缓存，每次获取都会缓存一份缓存，access_token 30天过期，refresh_token 10年过期
https://openapi.baidu.com/oauth/2.0/login_success#expires_in=2592000&access_token=xxxx&session_secret=&session_key=&scope=basic+netdisk

- 获取网盘用户信息

https://pan.baidu.com/rest/2.0/xpan/nas?access_token=xxxxxxx&method=uinfo

## License
MIT