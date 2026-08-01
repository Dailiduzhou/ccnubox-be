package crawler

import (
	"context"
	"errors"
	"os"
	"testing"
)

func Test_extractCourseInfo(t *testing.T) {

	path := "./test.html"

	// 读取文件内容
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			t.Skip("test.html fixture is not available")
		}
		t.Fatalf("failed to read file: %v", err)
	}

	t.Logf("length: %v", len(content))

	c := NewClassCrawler2(&MockProxyGetter{}, newTestLogger(t))

	classes, err := c.extractCourses(context.Background(), "2025", "1", []byte(string(content)))
	if err != nil {
		t.Fatalf("failed to extract classes: %v", err)
	}
	for _, class := range classes {
		t.Log(class)
	}
}

func Test_Crawler2(t *testing.T) {
	studentID, cookie := integrationCredentials(t, "CCNU_TEST_UNDERGRAD_COOKIE")
	c := NewClassCrawler2(&MockProxyGetter{}, newTestLogger(t))
	a, b, _, err := c.GetClassInfosForUndergraduate(context.Background(), studentID, "2025", "1", cookie)
	if err != nil {
		t.Fatalf("failed to crawl: %v", err)
	}
	for _, info := range a {
		t.Log(info)
	}
	for _, info := range b {
		t.Log(info)
	}

}
