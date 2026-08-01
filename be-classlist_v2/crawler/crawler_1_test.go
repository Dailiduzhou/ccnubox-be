package crawler

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/asynccnu/ccnubox-be/be-classlist_v2/conf"
	com_conf "github.com/asynccnu/ccnubox-be/common/bizpkg/conf"
	"github.com/asynccnu/ccnubox-be/common/bizpkg/log"
	"github.com/asynccnu/ccnubox-be/common/pkg/logger"
)

type MockProxyGetter struct{}

func (m *MockProxyGetter) GetProxy(ctx context.Context) *url.URL {
	return nil
}

func integrationCredentials(tb testing.TB, cookieEnv string) (string, string) {
	tb.Helper()
	studentID := os.Getenv("CCNU_TEST_STUDENT_ID")
	cookie := os.Getenv(cookieEnv)
	if studentID == "" || cookie == "" {
		tb.Skip("set CCNU_TEST_STUDENT_ID and " + cookieEnv + " to run integration test")
	}
	return studentID, cookie
}

func newTestLogger(t testing.TB) logger.Logger {
	t.Helper()

	logDir := "./debug_log"
	logPath := filepath.Join(logDir, "crawler-test.log")
	cfg := &conf.ServerConf{
		BaseServerConf: com_conf.BaseServerConf{
			Log: &com_conf.LogConf{
				Path:       logPath,
				MaxSize:    1,
				MaxBackups: 1,
				MaxAge:     1,
				Compress:   false,
			},
		},
	}

	return log.InitLogger(cfg.Log, 3)
}

func TestCrawler_GetClassInfosForUndergraduate(t *testing.T) {
	studentID, cookie := integrationCredentials(t, "CCNU_TEST_UNDERGRAD_COOKIE")
	crawler := NewClassCrawler(&MockProxyGetter{}, newTestLogger(t))
	start := time.Now()
	infos, scs, _, err := crawler.GetClassInfosForUndergraduate(context.Background(), studentID, "2024", "2", cookie)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(fmt.Sprintf("一共耗时%v", time.Since(start)))

	for _, v := range infos {
		t.Log(*v)
	}
	for _, v := range scs {
		t.Log(*v)
	}
	//t.Log(infos, scs)
}

func BenchmarkCrawler_GetClassInfosForUndergraduate(b *testing.B) {
	studentID, cookie := integrationCredentials(b, "CCNU_TEST_UNDERGRAD_COOKIE")
	crawler := NewClassCrawler(&MockProxyGetter{}, newTestLogger(b))

	ctx := context.Background()

	// 通常第一次调用可以预热缓存等，不纳入统计
	_, _, _, _ = crawler.GetClassInfosForUndergraduate(ctx, studentID, "2024", "2", cookie)

	b.ResetTimer() // 重置计时器，排除预热时间
	for i := 0; i < b.N; i++ {
		_, _, _, err := crawler.GetClassInfosForUndergraduate(ctx, studentID, "2024", "2", cookie)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestCrawler_GetClassInfoForGraduateStudent(t *testing.T) {
	studentID, cookie := integrationCredentials(t, "CCNU_TEST_GRAD_COOKIE")
	crawler := NewClassCrawler(&MockProxyGetter{}, newTestLogger(t))
	start := time.Now()
	infos, scs, _, err := crawler.GetClassInfoForGraduateStudent(context.Background(), studentID, "2024", "1", cookie)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(fmt.Sprintf("一共耗时%v", time.Since(start)))

	for _, v := range infos {
		t.Log(*v)
	}
	for _, v := range scs {
		t.Log(*v)
	}
}
