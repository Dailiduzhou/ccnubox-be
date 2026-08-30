package service

import (
	"context"
	"fmt"
	"strings"

	contentv1 "github.com/asynccnu/ccnubox-be/common/api/gen/proto/content/v1"
)

// ContentClient 从 be-content 获取当前学年学期
type ContentClient interface {
	GetCurrentSemester(ctx context.Context) (year, semester string, err error)
}

type contentClient struct {
	content contentv1.ContentServiceClient
}

func NewContentClient(content contentv1.ContentServiceClient) ContentClient {
	return &contentClient{content: content}
}

func (c *contentClient) GetCurrentSemester(ctx context.Context) (string, string, error) {
	resp, err := c.content.GetSemester(ctx, &contentv1.GetSemesterRequest{})
	if err != nil || resp == nil || resp.Semester == nil {
		if err == nil {
			err = fmt.Errorf("empty response")
		}
		return "", "", fmt.Errorf("get current semester from content service failed: %w", err)
	}
	// Semester 形如 "2026-1"
	parts := strings.Split(resp.Semester.Semester, "-")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid semester format from content service: %s", resp.Semester.Semester)
	}
	return parts[0], parts[1], nil
}
