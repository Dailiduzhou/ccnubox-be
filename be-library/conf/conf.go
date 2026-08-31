package conf

import (
	"time"

	"github.com/asynccnu/ccnubox-be/common/bizpkg/conf"
)

const (
	ServerEnv = "CCNUBOX_LIBRARY_NACOS_DSN"
)

// InfraConf 通用配置
type InfraConf struct {
	*conf.InfraConf `mapstructure:",squash"` // 为了能够正常解析需要对其进行拍平
}

// ServerConf 服务配置
type ServerConf struct {
	conf.BaseServerConf `mapstructure:",squash"`
	Crypto              *CryptoConf          `yaml:"crypto"`
	LibraryReminder     *LibraryReminderConf `yaml:"libraryReminder"`
}

type CryptoConf struct {
	Secret string `yaml:"secret"`
}

type LibraryReminderConf struct {
	Enabled                bool                   `yaml:"enabled"`
	PreferenceSyncInterval time.Duration          `yaml:"preferenceSyncInterval"`
	PreferenceFullSyncCron string                 `yaml:"preferenceFullSyncCron"`
	FullRefreshCron        string                 `yaml:"fullRefreshCron"`
	FullRefreshMinInterval time.Duration          `yaml:"fullRefreshMinInterval"`
	ActiveScanCron         string                 `yaml:"activeScanCron"`
	ActiveScanMinInterval  time.Duration          `yaml:"activeScanMinInterval"`
	JobDispatchCron        string                 `yaml:"jobDispatchCron"`
	OutboxInterval         time.Duration          `yaml:"outboxInterval"`
	UserConcurrency        int                    `yaml:"userConcurrency"`
	UserJitter             time.Duration          `yaml:"userJitter"`
	UpstreamRetryAttempts  int                    `yaml:"upstreamRetryAttempts"`
	UpstreamQPS            int                    `yaml:"upstreamQPS"`
	RequestTimeout         time.Duration          `yaml:"requestTimeout"`
	HistoryPageSize        int                    `yaml:"historyPageSize"`
	HistoryLookbackDays    int                    `yaml:"historyLookbackDays"`
	RetryMaxAttempts       int                    `yaml:"retryMaxAttempts"`
	BaselineOnEnable       *bool                  `yaml:"baselineOnEnable"`
	DryRun                 *bool                  `yaml:"dryRun"`
	NotificationTypes      *NotificationTypesConf `yaml:"notificationTypes"`
}

type NotificationTypesConf struct {
	ReservationDiscovered bool `yaml:"reservationDiscovered"`
	Start30               bool `yaml:"start30"`
	End10                 bool `yaml:"end10"`
	Away60                bool `yaml:"away60"`
	Away80                bool `yaml:"away80"`
	Breach                bool `yaml:"breach"`
	Blacklisted           bool `yaml:"blacklisted"`
}

// Reminder 返回完整且保守的配置。配置段缺失时等同于 enabled:false，
// 使旧部署配置无需增加 Feed 依赖或学校侧流量也能继续启动。
func (c *ServerConf) Reminder() LibraryReminderConf {
	result := LibraryReminderConf{
		PreferenceSyncInterval: 15 * time.Second,
		PreferenceFullSyncCron: "15 3 * * *",
		FullRefreshCron:        "*/30 * * * *",
		FullRefreshMinInterval: 25 * time.Minute,
		ActiveScanCron:         "*/5 * * * *",
		ActiveScanMinInterval:  4 * time.Minute,
		JobDispatchCron:        "* * * * *",
		OutboxInterval:         2 * time.Second,
		UserConcurrency:        20,
		UserJitter:             250 * time.Millisecond,
		UpstreamRetryAttempts:  3,
		UpstreamQPS:            20,
		RequestTimeout:         8 * time.Second,
		HistoryPageSize:        20,
		HistoryLookbackDays:    3,
		RetryMaxAttempts:       10,
		NotificationTypes: &NotificationTypesConf{
			ReservationDiscovered: true,
			Start30:               true,
			End10:                 true,
			Away60:                true,
			Away80:                true,
			Breach:                true,
			Blacklisted:           true,
		},
	}
	if c == nil || c.LibraryReminder == nil {
		return result
	}
	configured := *c.LibraryReminder
	result.Enabled = configured.Enabled
	result.DryRun = configured.DryRun
	result.BaselineOnEnable = configured.BaselineOnEnable
	if configured.NotificationTypes != nil {
		copyTypes := *configured.NotificationTypes
		result.NotificationTypes = &copyTypes
	}
	if configured.PreferenceSyncInterval > 0 {
		result.PreferenceSyncInterval = configured.PreferenceSyncInterval
	}
	if configured.PreferenceFullSyncCron != "" {
		result.PreferenceFullSyncCron = configured.PreferenceFullSyncCron
	}
	if configured.FullRefreshCron != "" {
		result.FullRefreshCron = configured.FullRefreshCron
	}
	if configured.FullRefreshMinInterval > 0 {
		result.FullRefreshMinInterval = configured.FullRefreshMinInterval
	}
	if configured.ActiveScanCron != "" {
		result.ActiveScanCron = configured.ActiveScanCron
	}
	if configured.ActiveScanMinInterval > 0 {
		result.ActiveScanMinInterval = configured.ActiveScanMinInterval
	}
	if configured.JobDispatchCron != "" {
		result.JobDispatchCron = configured.JobDispatchCron
	}
	if configured.OutboxInterval > 0 {
		result.OutboxInterval = configured.OutboxInterval
	}
	if configured.UserConcurrency > 0 {
		result.UserConcurrency = configured.UserConcurrency
	}
	if configured.UserJitter > 0 {
		result.UserJitter = configured.UserJitter
	}
	if configured.UpstreamRetryAttempts > 0 {
		result.UpstreamRetryAttempts = configured.UpstreamRetryAttempts
	}
	if configured.UpstreamQPS > 0 {
		result.UpstreamQPS = configured.UpstreamQPS
	}
	if configured.RequestTimeout > 0 {
		result.RequestTimeout = configured.RequestTimeout
	}
	if configured.HistoryPageSize > 0 {
		result.HistoryPageSize = configured.HistoryPageSize
	}
	if configured.HistoryLookbackDays > 0 {
		result.HistoryLookbackDays = configured.HistoryLookbackDays
	}
	if configured.RetryMaxAttempts > 0 {
		result.RetryMaxAttempts = configured.RetryMaxAttempts
	}
	return result
}

func (c LibraryReminderConf) IsDryRun() bool { return c.DryRun == nil || *c.DryRun }

func (c LibraryReminderConf) ShouldBaselineOnEnable() bool {
	return c.BaselineOnEnable == nil || *c.BaselineOnEnable
}

func InitServerConf() *ServerConf {
	return conf.InitConfig[ServerConf](ServerEnv)
}

func InitInfraConfig() *InfraConf {
	return &InfraConf{conf.InitInfraConfig()}
}
