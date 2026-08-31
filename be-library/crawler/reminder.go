package crawler

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/asynccnu/ccnubox-be/common/pkg/metricsx"
	"github.com/google/uuid"
)

const (
	reminderTodayPath   = "/jsq/static/frontApi/user/lastMake"
	reminderHistoryPath = "/jsq/static/frontApi/user/history/%d/%d"
	reminderCurrentPath = "/jsq/static/frontApi/user/currentUseMake"
	reminderUserPath    = "/jsq/static/frontApi/user/getUserInfo"
	reminderBreachPath  = "/jsq/static/frontApi/user/breach/%d/%d"
	reminderDoorLogPath = "/jsq/static/frontApi/user/doorLog/%s"
	reminderSysSetPath  = "/jsq/static/public/cg/getSysSet/PC"
)

var ErrUpstreamStateUnknown = errors.New("library reminder upstream state unknown")

type ReminderCrawler interface {
	GetTodayReservations(context.Context, string) ([]ReminderReservation, error)
	GetRecentHistory(context.Context, string, HistoryWatermark) (HistoryPage, error)
	GetCurrentReservation(context.Context, string) (*ReminderReservation, error)
	GetUserState(context.Context, string) (LibraryUserState, error)
	GetBreaches(context.Context, string, int, int) (BreachPage, error)
	GetDoorLogs(context.Context, string, string) ([]DoorLog, error)
}

type HistoryWatermark struct {
	ReservationID string
}

type HistoryPage struct {
	Reservations []ReminderReservation
	Complete     bool
}

type ReminderReservation struct {
	ID          string `json:"id"`
	SeatID      string `json:"seatId"`
	SeatLabel   string `json:"seatLabel"`
	MakeDateStr string `json:"makeDateStr"`
	MakeBegin   Minute `json:"makeBegin"`
	MakeEnd     Minute `json:"makeEnd"`
	Status      string `json:"status"`
	Location    string `json:"location"`
	Message     string `json:"message"`
	AwayRange   string `json:"awayRange"`
	AwayTimeM   int    `json:"awayTimeM"`
}

func (r ReminderReservation) Times() (time.Time, time.Time, error) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	day, err := time.ParseInLocation("2006-01-02", r.MakeDateStr, loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse reservation date: %w", err)
	}
	if int(r.MakeBegin) < 0 || int(r.MakeBegin) > 24*60 || int(r.MakeEnd) < 0 || int(r.MakeEnd) > 24*60 || r.MakeEnd <= r.MakeBegin {
		return time.Time{}, time.Time{}, errors.New("invalid reservation minute range")
	}
	return day.Add(time.Duration(r.MakeBegin) * time.Minute), day.Add(time.Duration(r.MakeEnd) * time.Minute), nil
}

type Minute int

func (m *Minute) UnmarshalJSON(raw []byte) error {
	var number int
	if err := json.Unmarshal(raw, &number); err == nil {
		*m = Minute(number)
		return nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return errors.New("minute must be a number or HH:mm string")
	}
	if value, err := strconv.Atoi(text); err == nil {
		*m = Minute(value)
		return nil
	}
	parsed, err := time.Parse("15:04", text)
	if err != nil {
		return fmt.Errorf("invalid minute %q", text)
	}
	*m = Minute(parsed.Hour()*60 + parsed.Minute())
	return nil
}

type LibraryUserState struct {
	BreachNum     int    `json:"breachNum"`
	ScoreNum      int    `json:"scoreNum"`
	BlackTime     string `json:"blackTime"`
	BlackMessage  string `json:"blackMessage"`
	CycleTimeName string `json:"cycleTimeName"`
}

type BreachPage struct {
	Count int               `json:"count"`
	List  []json.RawMessage `json:"list"`
}

type DoorLog json.RawMessage

type reminderEnvelope struct {
	Status  bool            `json:"status"`
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type upstreamError struct {
	HTTPCode int
	Code     int
	Message  string
	Cause    error
}

func (e *upstreamError) Error() string {
	return fmt.Sprintf("%v: http=%d code=%d message=%s", ErrUpstreamStateUnknown, e.HTTPCode, e.Code, e.Message)
}
func (e *upstreamError) Unwrap() error { return e.Cause }
func (e *upstreamError) Is(target error) bool {
	return target == ErrUpstreamStateUnknown || errors.Is(e.Cause, target)
}

type ReminderHTTPClient struct {
	client          *http.Client
	baseURL         string
	requestTimeout  time.Duration
	historyPageSize int
	historyMaxPages int
	historyLookback int
	requestSpacing  time.Duration
	rateMu          sync.Mutex
	nextRequest     time.Time

	keyMu        sync.Mutex
	hmacKey      string
	hmacKeyUntil time.Time
	metrics      *metricsx.LibraryReminderMetrics
}

func NewReminderCrawler(client *http.Client, requestTimeout time.Duration, historyPageSize, lookbackDays, upstreamQPS int, metricSet ...*metricsx.LibraryReminderMetrics) *ReminderHTTPClient {
	if requestTimeout <= 0 {
		requestTimeout = 8 * time.Second
	}
	if historyPageSize <= 0 {
		historyPageSize = 20
	}
	if lookbackDays <= 0 {
		lookbackDays = 3
	}
	if upstreamQPS <= 0 {
		upstreamQPS = 20
	}
	result := &ReminderHTTPClient{client: client, baseURL: BaseDomain, requestTimeout: requestTimeout, historyPageSize: historyPageSize, historyMaxPages: 100, historyLookback: lookbackDays, requestSpacing: time.Second / time.Duration(upstreamQPS)}
	if len(metricSet) > 0 {
		result.metrics = metricSet[0]
	}
	return result
}

func (c *ReminderHTTPClient) GetTodayReservations(ctx context.Context, token string) ([]ReminderReservation, error) {
	raw, err := c.request(ctx, token, reminderTodayPath)
	if err != nil {
		return nil, err
	}
	var rows []ReminderReservation
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("%w: decode today reservations", ErrUpstreamStateUnknown)
	}
	if err := validateReservations(rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (c *ReminderHTTPClient) GetRecentHistory(ctx context.Context, token string, watermark HistoryWatermark) (HistoryPage, error) {
	result := HistoryPage{Complete: true}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	cutoff := time.Now().In(loc).AddDate(0, 0, -c.historyLookback).Format("2006-01-02")
	for page := 0; page < c.historyMaxPages; page++ {
		raw, err := c.request(ctx, token, fmt.Sprintf(reminderHistoryPath, page, c.historyPageSize))
		if err != nil {
			return HistoryPage{}, err
		}
		var data struct {
			Count int                   `json:"count"`
			List  []ReminderReservation `json:"list"`
		}
		if err := json.Unmarshal(raw, &data); err != nil {
			return HistoryPage{}, fmt.Errorf("%w: decode reservation history", ErrUpstreamStateUnknown)
		}
		if err := validateReservations(data.List); err != nil {
			return HistoryPage{}, err
		}
		for _, row := range data.List {
			if watermark.ReservationID != "" && row.ID == watermark.ReservationID {
				return result, nil
			}
			if row.MakeDateStr < cutoff {
				return result, nil
			}
			result.Reservations = append(result.Reservations, row)
		}
		if len(data.List) < c.historyPageSize || len(result.Reservations) >= data.Count {
			return result, nil
		}
	}
	result.Complete = false
	return result, nil
}

func (c *ReminderHTTPClient) GetCurrentReservation(ctx context.Context, token string) (*ReminderReservation, error) {
	raw, err := c.request(ctx, token, reminderCurrentPath)
	if err != nil {
		return nil, err
	}
	data := bytes.TrimSpace(raw)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) || bytes.Equal(data, []byte(`""`)) {
		return nil, nil
	}
	if len(data) > 0 && data[0] == '[' {
		return nil, fmt.Errorf("%w: current reservation data was an array", ErrUpstreamStateUnknown)
	}
	var row ReminderReservation
	if err := json.Unmarshal(data, &row); err != nil {
		return nil, fmt.Errorf("%w: decode current reservation", ErrUpstreamStateUnknown)
	}
	if err := validateReservations([]ReminderReservation{row}); err != nil {
		return nil, err
	}
	return &row, nil
}

func (c *ReminderHTTPClient) GetUserState(ctx context.Context, token string) (LibraryUserState, error) {
	raw, err := c.request(ctx, token, reminderUserPath)
	if err != nil {
		return LibraryUserState{}, err
	}
	var state LibraryUserState
	if err := json.Unmarshal(raw, &state); err != nil {
		return LibraryUserState{}, fmt.Errorf("%w: decode user state", ErrUpstreamStateUnknown)
	}
	if len(state.CycleTimeName) > 128 || len(state.BlackTime) > 255 || len(state.BlackMessage) > 60<<10 {
		return LibraryUserState{}, fmt.Errorf("%w: user state fields exceed storage limits", ErrUpstreamStateUnknown)
	}
	return state, nil
}

func (c *ReminderHTTPClient) GetBreaches(ctx context.Context, token string, page, pageSize int) (BreachPage, error) {
	raw, err := c.request(ctx, token, fmt.Sprintf(reminderBreachPath, page, pageSize))
	if err != nil {
		return BreachPage{}, err
	}
	var result BreachPage
	if err := json.Unmarshal(raw, &result); err != nil {
		return BreachPage{}, fmt.Errorf("%w: decode breach page", ErrUpstreamStateUnknown)
	}
	return result, nil
}

func (c *ReminderHTTPClient) GetDoorLogs(ctx context.Context, token, date string) ([]DoorLog, error) {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return nil, errors.New("door log date must use YYYY-MM-DD")
	}
	raw, err := c.request(ctx, token, fmt.Sprintf(reminderDoorLogPath, date))
	if err != nil {
		return nil, err
	}
	var result []DoorLog
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("%w: decode door logs", ErrUpstreamStateUnknown)
	}
	return result, nil
}

func validateReservations(rows []ReminderReservation) error {
	for _, row := range rows {
		if strings.TrimSpace(row.ID) == "" || strings.TrimSpace(row.MakeDateStr) == "" {
			return fmt.Errorf("%w: reservation is missing identity or date", ErrUpstreamStateUnknown)
		}
		if len(row.ID) > 128 || len(row.SeatID) > 128 || len(row.SeatLabel) > 128 || len(row.Location) > 512 || len(row.Status) > 128 || len(row.MakeDateStr) != len("2006-01-02") {
			return fmt.Errorf("%w: reservation fields exceed storage limits", ErrUpstreamStateUnknown)
		}
		if _, _, err := row.Times(); err != nil {
			return fmt.Errorf("%w: invalid reservation %s: %v", ErrUpstreamStateUnknown, row.ID, err)
		}
	}
	return nil
}

func (c *ReminderHTTPClient) request(parent context.Context, token, path string) (json.RawMessage, error) {
	if token == "" {
		return nil, fmt.Errorf("%w: empty token", ErrUpstreamStateUnknown)
	}
	key, err := c.signingKey(parent, token, false)
	if err != nil {
		return nil, err
	}
	data, err := c.signedRequest(parent, token, key, path)
	if err == nil {
		return data, nil
	}
	// HMAC 密钥过期或轮换与认证拒绝无法区分，因此刷新一次；后续失败仍视为“未知”。
	key, refreshErr := c.signingKey(parent, token, true)
	if refreshErr != nil {
		return nil, err
	}
	return c.signedRequest(parent, token, key, path)
}

func (c *ReminderHTTPClient) signedRequest(parent context.Context, token, key, path string) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(parent, c.requestTimeout)
	defer cancel()
	now := time.Now().UnixMilli()
	requestID := uuid.NewString()
	message := fmt.Sprintf("seat::%s::%d::POST", requestID, now)
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(message))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, fmt.Errorf("%w: create request", ErrUpstreamStateUnknown)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("token", token)
	req.Header.Set("loginType", "PC")
	req.Header.Set("X-request-id", requestID)
	req.Header.Set("X-request-date", strconv.FormatInt(now, 10))
	req.Header.Set("X-hmac-request-key", hex.EncodeToString(mac.Sum(nil)))
	return c.do(req)
}

func (c *ReminderHTTPClient) signingKey(parent context.Context, token string, force bool) (string, error) {
	c.keyMu.Lock()
	defer c.keyMu.Unlock()
	if !force && c.hmacKey != "" && time.Now().Before(c.hmacKeyUntil) {
		return c.hmacKey, nil
	}
	ctx, cancel := context.WithTimeout(parent, c.requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+reminderSysSetPath, bytes.NewReader([]byte("{}")))
	if err != nil {
		return "", fmt.Errorf("%w: create system config request", ErrUpstreamStateUnknown)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("token", token)
	req.Header.Set("loginType", "PC")
	raw, err := c.do(req)
	if err != nil {
		return "", err
	}
	var data struct {
		HMACKey string `json:"hmacKey"`
	}
	if err := json.Unmarshal(raw, &data); err != nil || data.HMACKey == "" {
		return "", fmt.Errorf("%w: invalid system hmac config", ErrUpstreamStateUnknown)
	}
	key, err := decryptHMACKey(data.HMACKey)
	if err != nil || key == "" {
		return "", fmt.Errorf("%w: decrypt system hmac config", ErrUpstreamStateUnknown)
	}
	c.hmacKey, c.hmacKeyUntil = key, time.Now().Add(10*time.Minute)
	return key, nil
}

func (c *ReminderHTTPClient) do(req *http.Request) (data json.RawMessage, err error) {
	started := time.Now()
	if c.metrics != nil {
		endpoint := reminderMetricEndpoint(req.URL.Path)
		defer func() {
			result := "success"
			if err != nil {
				result = classifyReminderUpstreamError(err)
			}
			c.metrics.UpstreamRequestsTotal.WithLabelValues(endpoint, result).Inc()
			c.metrics.UpstreamDurationSeconds.WithLabelValues(endpoint).Observe(time.Since(started).Seconds())
		}()
	}
	if err := c.waitRate(req.Context()); err != nil {
		return nil, &upstreamError{Cause: err}
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, &upstreamError{Cause: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, &upstreamError{HTTPCode: resp.StatusCode, Cause: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &upstreamError{HTTPCode: resp.StatusCode}
	}
	var env reminderEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, &upstreamError{HTTPCode: resp.StatusCode, Cause: err}
	}
	if !env.Status || env.Code != http.StatusOK {
		return nil, &upstreamError{HTTPCode: resp.StatusCode, Code: env.Code, Message: env.Message}
	}
	if env.Data == nil {
		return nil, &upstreamError{HTTPCode: resp.StatusCode, Code: env.Code, Message: "missing data"}
	}
	return env.Data, nil
}

func reminderMetricEndpoint(path string) string {
	switch {
	case path == reminderTodayPath:
		return "last_make"
	case strings.HasPrefix(path, "/jsq/static/frontApi/user/history/"):
		return "history"
	case path == reminderCurrentPath:
		return "current_use_make"
	case path == reminderUserPath:
		return "user_info"
	case strings.HasPrefix(path, "/jsq/static/frontApi/user/breach/"):
		return "breach"
	case strings.HasPrefix(path, "/jsq/static/frontApi/user/doorLog/"):
		return "door_log"
	case path == reminderSysSetPath:
		return "system_config"
	default:
		return "unknown"
	}
}

func classifyReminderUpstreamError(err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var upstream *upstreamError
	if errors.As(err, &upstream) {
		if upstream.HTTPCode == http.StatusUnauthorized || upstream.HTTPCode == http.StatusForbidden || upstream.Code == http.StatusUnauthorized || upstream.Code == http.StatusForbidden {
			return "auth_error"
		}
		if upstream.HTTPCode != 0 && (upstream.HTTPCode < 200 || upstream.HTTPCode >= 300) {
			return "http_error"
		}
		if upstream.Code != 0 && upstream.Code != http.StatusOK {
			return "business_error"
		}
		if upstream.Cause != nil {
			return "network_or_decode_error"
		}
	}
	return "invalid_response"
}

func (c *ReminderHTTPClient) waitRate(ctx context.Context) error {
	c.rateMu.Lock()
	now := time.Now()
	ready := c.nextRequest
	if ready.Before(now) {
		ready = now
	}
	c.nextRequest = ready.Add(c.requestSpacing)
	c.rateMu.Unlock()
	if delay := time.Until(ready); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}

func decryptHMACKey(ciphertextBase64 string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextBase64)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher([]byte("server_date_time"))
	if err != nil {
		return "", err
	}
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return "", errors.New("invalid AES-CBC ciphertext length")
	}
	plain := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, []byte("client_date_time")).CryptBlocks(plain, ciphertext)
	padding := int(plain[len(plain)-1])
	if padding < 1 || padding > aes.BlockSize || padding > len(plain) {
		return "", errors.New("invalid PKCS7 padding")
	}
	for _, value := range plain[len(plain)-padding:] {
		if int(value) != padding {
			return "", errors.New("invalid PKCS7 padding")
		}
	}
	return string(plain[:len(plain)-padding]), nil
}
