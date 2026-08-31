package domain

// FeedEvent的模型
type FeedEvent struct {
	ID           int64             `json:"id"` // ID
	StudentId    string            `json:"student_id"`
	Type         string            `json:"type"`          // 类型
	Title        string            `json:"title"`         // 提示用的字段
	Content      string            `json:"content"`       // 正式文本
	Url          string            `json:"url"`           // 消息详情跳转路由
	ExtendFields map[string]string `json:"extend_fields"` // 拓展字段
	CreatedAt    int64             `json:"created_at"`    // 创建时间，Unix 时间戳（int格式）
	DedupeKey    string            `json:"dedupe_key,omitempty"`
	Source       string            `json:"source,omitempty"`
	OccurredAt   int64             `json:"occurred_at,omitempty"`
}

// AllowList 表示更改推送消息数量的请求
type AllowList struct {
	StudentId string `json:"student_id"`
	Grade     bool   `json:"grade"`
	Muxi      bool   `json:"muxi"`
	Holiday   bool   `json:"holiday"`
	Energy    bool   `json:"energy"`
	FeedBack  bool   `json:"feed_back"`
	// 可空字段，兼容
	Library *bool `json:"library,omitempty"`
}

type LibraryPreferenceChange struct {
	Revision  int64
	StudentID string
	Enabled   bool
	ChangedAt int64
}

type LibraryReminderUser struct {
	ID        int64
	StudentID string
	Revision  int64
}

type MuxiOfficialMSG struct {
	Title        string
	Content      string
	ExtendFields       // 拓展字段如果要发额外的东西的话
	PublicTime   int64 // 正式发布的时间
	Id           string
}
