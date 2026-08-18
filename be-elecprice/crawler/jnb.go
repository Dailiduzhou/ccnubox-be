package crawler

import (
	"context"
	"crypto/ecdsa"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/asynccnu/ccnubox-be/be-elecprice/conf"
	"github.com/asynccnu/ccnubox-be/common/bizpkg/proxy"
	"github.com/asynccnu/ccnubox-be/common/pkg/errorx"
	"github.com/asynccnu/ccnubox-be/common/pkg/httpx"
	"github.com/asynccnu/ccnubox-be/common/pkg/logger"
	"github.com/emmansun/gmsm/sm2"
)

// tokenExpiredCodes JNB 返回的登录过期错误码
var tokenExpiredCodes = map[int]struct{}{
	2001:  {},
	50008: {},
	50012: {},
	50014: {},
}

type JnbClient interface {
	GetArchitectureInfo(ctx context.Context, areaID string) ([]ArchitectureInfo, error)
	GetRoomInfo(ctx context.Context, archiID string, floor string) ([]RoomInfo, error)
	GetRoomMeterInfo(ctx context.Context, roomID string) (*RoomMeterInfo, error)
	GetReserve(ctx context.Context, meterID string) (*ReserveInfo, error)
	GetMeterDayValue(ctx context.Context, meterID string, date string) (*DayValueInfo, error)
}

type ArchitectureInfo struct {
	ArchitectureID     string `json:"ArchitectureID"`
	ArchitectureName   string `json:"ArchitectureName"`
	ArchitectureStorys int    `json:"ArchitectureStorys"`
	ArchitectureBegin  int    `json:"ArchitectureBegin"`
	ArchitectureUnit   string `json:"ArchitectureUnit"`
}

type RoomInfo struct {
	RoomNo   string `json:"RoomNo"`
	RoomName string `json:"RoomName"`
}

type RoomMeterInfo struct {
	MeterList []MeterInfo `json:"meterList"`
}

type MeterInfo struct {
	MeterId   string `json:"meterId"`
	MeterType string `json:"meterType"`
	Style     int    `json:"style"`
}

type ReserveInfo struct {
	RemainPower string `json:"remainPower"`
}

type DayValueInfo struct {
	DayValue    string `json:"dayValue"`
	DayUseMeony string `json:"dayUseMeony"`
}

type jnbClient struct {
	cfg *conf.JnbConf
	pc  proxy.Client
	l   logger.Logger

	mu     sync.Mutex
	access string
	expAt  time.Time
}

func NewJnbClient(pc proxy.Client, l logger.Logger, cfg *conf.JnbConf) JnbClient {
	return &jnbClient{cfg: cfg.Default(), pc: pc, l: l}
}

// token 获取可用 access_Jwt, 过期前十分钟提前刷新
func (c *jnbClient) token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.access != "" && time.Now().Before(c.expAt.Add(-10*time.Minute)) {
		return c.access, nil
	}

	access, exp, err := c.login(ctx)
	if err != nil {
		return "", err
	}
	c.access = access
	c.expAt = exp
	return access, nil
}

func (c *jnbClient) invalidateToken() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.access = ""
}

// login SM2 加密查询串登录, 返回 access_Jwt 及过期时间
func (c *jnbClient) login(ctx context.Context) (string, time.Time, error) {
	var (
		p2 = md5Hex(c.cfg.AccountPass)
		t  = strconv.FormatInt(time.Now().Unix(), 10)
		e  = fmt.Sprintf("p1=%s&p2=%s&p3=%s&t=%s", c.cfg.AccountId, p2, c.cfg.SysId, t)
	)

	s, err := sm2EncryptHex(e, c.cfg.Sm2PublicKey)
	if err != nil {
		return "", time.Time{}, errorx.Errorf("crawler: sm2 encrypt login query failed, err: %w", err)
	}

	params := url.Values{}
	params.Set("p1", c.cfg.AccountId)
	params.Set("p2", p2)
	params.Set("p3", c.cfg.SysId)
	params.Set("t", t)
	params.Set("s", s)

	code, body, err := c.do(ctx, c.cfg.BaseUrl+"/v3/XINTFLg/SpecialSignIn?"+params.Encode(), "")
	if err != nil {
		return "", time.Time{}, err
	}
	if code != 0 {
		return "", time.Time{}, errorx.Errorf("crawler: jnb login failed, code: %d, body: %s", code, body)
	}

	var resp struct {
		Data struct {
			AccessJwt  string `json:"access_Jwt"`
			RefreshJwt string `json:"refresh_Jwt"`
		} `json:"Data"`
	}
	if err = json.Unmarshal([]byte(body), &resp); err != nil {
		return "", time.Time{}, errorx.Errorf("crawler: unmarshal jnb login response failed, body: %s, err: %w", body, err)
	}
	if resp.Data.AccessJwt == "" {
		return "", time.Time{}, errorx.Errorf("crawler: jnb login empty access jwt, body: %s", body)
	}

	return resp.Data.AccessJwt, parseJwtExp(resp.Data.AccessJwt), nil
}

// get 携带 Bearer token 请求接口, 登录过期自动重登重试一次
func (c *jnbClient) get(ctx context.Context, path string, params url.Values, target any) error {
	reqURL := c.cfg.BaseUrl + path
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	for attempt := 0; attempt < 2; attempt++ {
		token, err := c.token(ctx)
		if err != nil {
			return err
		}

		code, body, err := c.do(ctx, reqURL, token)
		if err != nil {
			return err
		}

		if _, expired := tokenExpiredCodes[code]; expired {
			if attempt == 0 {
				c.invalidateToken()
				continue
			}
			return errorx.Errorf("crawler: jnb token still expired after relogin, path: %s, code: %d", path, code)
		}

		if code != 0 {
			return errorx.Errorf("crawler: jnb api code error, path: %s, code: %d, body: %s", path, code, body)
		}

		return parseEnvelopeData(body, target)
	}

	return errorx.Errorf("crawler: jnb api unreachable, path: %s", path)
}

// do 发送请求, 返回业务码和响应体
func (c *jnbClient) do(ctx context.Context, reqURL string, token string) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return 0, "", fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("Referer", "https://jnb.ccnu.edu.cn/MobilePayWeb_Vue/")
	req.Header.Set("Origin", "https://jnb.ccnu.edu.cn")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.pc.NewProxyClient(proxy.WithProxyTransport()).Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return 0, "", fmt.Errorf("服务器返回错误状态码: %d", resp.StatusCode)
	}

	body, err := httpx.ReadResponse(resp, httpx.WithMaxBodyBytes(1<<20))
	if err != nil {
		return 0, "", fmt.Errorf("读取响应体失败: %w", err)
	}

	var envelope struct {
		Code int `json:"Code"`
	}
	if err = json.Unmarshal(body, &envelope); err != nil {
		return 0, "", fmt.Errorf("解析响应信封失败: %w", err)
	}

	return envelope.Code, string(body), nil
}

func (c *jnbClient) GetArchitectureInfo(ctx context.Context, areaID string) ([]ArchitectureInfo, error) {
	var resp struct {
		ArchitectureInfoList []ArchitectureInfo `json:"architectureInfoList"`
	}
	err := c.get(ctx, "/v3/XINTF/GetArchitectureInfo", url.Values{"AreaID": {areaID}}, &resp)
	return resp.ArchitectureInfoList, err
}

func (c *jnbClient) GetRoomInfo(ctx context.Context, archiID string, floor string) ([]RoomInfo, error) {
	var resp struct {
		RoomInfoList []RoomInfo `json:"roomInfoList"`
	}
	err := c.get(ctx, "/v3/XINTF/GetRoomInfo", url.Values{"ArchitectureID": {archiID}, "Floor": {floor}}, &resp)
	return resp.RoomInfoList, err
}

func (c *jnbClient) GetRoomMeterInfo(ctx context.Context, roomID string) (*RoomMeterInfo, error) {
	var resp RoomMeterInfo
	err := c.get(ctx, "/v3/XINTF/GetRoomMeterInfo", url.Values{"RoomID": {roomID}}, &resp)
	return &resp, err
}

func (c *jnbClient) GetReserve(ctx context.Context, meterID string) (*ReserveInfo, error) {
	var resp ReserveInfo
	err := c.get(ctx, "/v3/XINTF/GetReserve", url.Values{"MeterID": {meterID}}, &resp)
	return &resp, err
}

func (c *jnbClient) GetMeterDayValue(ctx context.Context, meterID string, date string) (*DayValueInfo, error) {
	var resp struct {
		DayValues []DayValueInfo `json:"DayValues"`
	}
	err := c.get(ctx, "/v3/XINTF/GetMeterDayValue", url.Values{"MeterID": {meterID}, "startDate": {date}, "endDate": {date}}, &resp)
	if err != nil {
		return nil, err
	}
	if len(resp.DayValues) == 0 {
		return nil, errorx.Errorf("crawler: jnb meter day value empty, mid: %s, date: %s", meterID, date)
	}
	return &resp.DayValues[0], nil
}

func parseEnvelopeData(body string, target any) error {
	var envelope struct {
		Code int             `json:"Code"`
		Data json.RawMessage `json:"Data"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		return fmt.Errorf("解析响应信封失败: %w", err)
	}
	if err := json.Unmarshal(envelope.Data, target); err != nil {
		return fmt.Errorf("解析响应数据失败, body: %s, err: %w", body, err)
	}
	return nil
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// sm2EncryptHex 与前端 sm-crypto doEncrypt(msg, pub, 1) 一致: C1C3C2, 无 04 前缀
func sm2EncryptHex(msg string, pubHex string) (string, error) {
	pubBytes, err := hex.DecodeString(pubHex)
	if err != nil || len(pubBytes) != 65 {
		return "", fmt.Errorf("sm2 公钥格式错误")
	}
	pub := &ecdsa.PublicKey{
		Curve: sm2.P256(),
		X:     new(big.Int).SetBytes(pubBytes[1:33]),
		Y:     new(big.Int).SetBytes(pubBytes[33:65]),
	}

	ct, err := sm2.Encrypt(rand.Reader, pub, []byte(msg), sm2.NewPlainEncrypterOpts(sm2.MarshalUncompressed, sm2.C1C3C2))
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(ct[1:]), nil
}

// parseJwtExp 解析 JWT payload 的 exp, 解析失败时返回零值
func parseJwtExp(token string) time.Time {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err = json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
		return time.Time{}
	}
	return time.Unix(claims.Exp, 0)
}
