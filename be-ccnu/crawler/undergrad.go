package crawler

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/asynccnu/ccnubox-be/common/pkg/errorx"
	"github.com/asynccnu/ccnubox-be/common/tool"
)

const (
	loginCCNUPassPortURL = "https://account.ccnu.edu.cn/cas/login"
	// 新的教务系统 CAS 登录地址
	CASURL = loginCCNUPassPortURL + "?service=https://bkzhjw.ccnu.edu.cn/jsxsd/framework/xsMainV.htmlx"
	pgUrl  = "https://bkzhjw.ccnu.edu.cn/jsxsd/"
)

// 存放本科生院相关的爬虫
type UnderGrad struct {
	Client *http.Client
}

func NewUnderGrad(client *http.Client) *UnderGrad {
	return &UnderGrad{
		Client: client,
	}
}

// 1. LoginUnderGradSystem 教务系统 CAS 模拟登录
func (c *UnderGrad) LoginUnderGradSystem(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, CASURL, nil)
	if err != nil {
		return errorx.Errorf("undergrad: create CAS login request failed: %w", err)
	}

	request.Header.Set(
		"User-Agent",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/72.0.3626.109 Safari/537.36",
	)

	resp, err := c.Client.Do(request)
	if err != nil {
		return errorx.Errorf("undergrad: send CAS login request failed: %w", err)
	}
	defer resp.Body.Close()

	// CAS 登录主要依赖重定向和 Cookie，这里不强校验响应体
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusFound {
		return errorx.Errorf("undergrad: unexpected CAS response status %d", resp.StatusCode)
	}
	if err := validateUndergraduateLanding(resp); err != nil {
		return err
	}

	return nil
}

func validateUndergraduateLanding(resp *http.Response) error {
	if resp == nil || resp.Request == nil || resp.Request.URL == nil {
		return errorx.New("undergrad: CAS response is missing its final URL")
	}

	host := resp.Request.URL.Hostname()
	path := strings.TrimRight(resp.Request.URL.Path, "/")
	if strings.EqualFold(host, "selfservice.ccnu.edu.cn") &&
		strings.EqualFold(path, "/account/init.jsf") {
		return errorx.Errorf("undergrad: account initialization required: %w", tool.ErrCCNUAccountInitializationRequired)
	}
	if !strings.EqualFold(host, "bkzhjw.ccnu.edu.cn") {
		return errorx.Errorf("undergrad: CAS authentication did not reach undergraduate system, final host=%s", host)
	}
	return nil
}

// 2. GetCookieFromUnderGradSystem 从教务系统 CookieJar 中提取 Cookie
func (c *UnderGrad) GetCookieFromUnderGradSystem() (string, error) {
	parsedURL, err := url.Parse(pgUrl)
	if err != nil {
		return "", errorx.Errorf("undergrad: parse pgUrl failed: %w", err)
	}

	cookies := c.Client.Jar.Cookies(parsedURL)
	if len(cookies) == 0 {
		return "", errorx.Errorf("undergrad: no cookies found in jar")
	}

	var cookieStr strings.Builder
	for i, cookie := range cookies {
		cookieStr.WriteString(cookie.Name)
		cookieStr.WriteString("=")
		cookieStr.WriteString(cookie.Value)
		if i != len(cookies)-1 {
			cookieStr.WriteString("; ")
		}
	}

	return cookieStr.String(), nil
}
